package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// DiscoveryError carries only a filesystem identity and a fixed category across
// the RPC boundary. Cause remains local for debugging and cancellation checks.
type DiscoveryError struct {
	File     string
	Category string
	Cause    error
}

func (e *DiscoveryError) Error() string { return e.Cause.Error() }
func (e *DiscoveryError) Unwrap() error { return e.Cause }

func discoveryError(root, path, category string, cause error) error {
	var existing *DiscoveryError
	if errors.As(cause, &existing) {
		return cause
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return cause
	}
	return &DiscoveryError{File: filepath.ToSlash(relative), Category: category, Cause: cause}
}

// canonicalPath preserves physical ancestor identity for files generated only in
// the discovery overlay. Actual existence is checked through the active FS.
func canonicalPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = canonicalPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

func within(path, directory string) bool {
	rel, err := filepath.Rel(directory, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// projectDiscovery holds one inventory pass. Its overlay is invocation-local:
// generated HCL retains its original virtual path and no source files or state
// are moved or written.
type projectDiscovery struct {
	ctx       context.Context
	request   model.Request
	root      string
	overlay   afero.Fs
	result    *model.Resolution
	templates map[string]bool
	// generated records units reached through stack traversal so the standalone
	// pass does not discover the same generated unit again.
	generated map[string]bool
}

// discoverProject discovers live stacks first, then standalone units that no
// stack owns, and rejects colliding Atmos identities.
func discoverProject(ctx context.Context, request model.Request) (*model.Resolution, error) {
	root, err := canonicalPath(request.BasePath)
	if err != nil {
		return nil, err
	}
	stacks, units, err := discoveryPaths(root, request.IncludedPaths, request.ExcludedPaths)
	if err != nil {
		return nil, err
	}
	d := &projectDiscovery{
		ctx: ctx, request: request, root: root,
		overlay:   afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(afero.NewOsFs()), afero.NewMemMapFs()),
		result:    &model.Resolution{},
		templates: map[string]bool{},
		generated: map[string]bool{},
	}
	if err := d.discoverStacks(stacks); err != nil {
		return nil, err
	}
	if err := d.discoverStandaloneUnits(units); err != nil {
		return nil, err
	}
	slices.SortFunc(d.result.Inventory, func(a, b model.DiscoveredUnit) int { return strings.Compare(a.Stack+"/"+a.Name, b.Stack+"/"+b.Name) })
	if err := checkIdentityCollisions(d.result.Inventory); err != nil {
		return nil, err
	}
	return d.result, nil
}

// skipped reports whether path lies inside a catalog template directory.
func (d *projectDiscovery) skipped(path string) bool {
	for dir := range d.templates {
		if within(path, dir) {
			return true
		}
	}
	return false
}

func (d *projectDiscovery) nameFor(dir string) string {
	rel, _ := filepath.Rel(d.root, dir)
	if rel == "." {
		return "default"
	}
	return filepath.ToSlash(rel)
}

func (d *projectDiscovery) parse(path string, generate bool) (*model.Resolution, error) {
	req := d.request
	req.Path, req.Phase = path, "discover"
	if generate {
		req.Command = "generate"
	}
	resolved, err := resolve(d.ctx, req, d.overlay)
	if err != nil {
		return nil, discoveryError(d.root, path, "EVALUATION_FAILED", err)
	}
	return resolved, nil
}

// discoverStacks visits live stacks. Catalog stack templates may require values
// supplied by a live stack, so their errors wait until successful roots have
// identified the referenced templates.
func (d *projectDiscovery) discoverStacks(stacks []string) error {
	failures := map[string]error{}
	for _, path := range stacks {
		if d.skipped(path) {
			continue
		}
		if _, err := d.parse(path, true); err != nil {
			failures[path] = err
			continue
		}
		if err := d.visit(path, unitPlacement{stack: d.nameFor(filepath.Dir(path)), generationRoot: path}, 0); err != nil {
			failures[path] = err
		}
	}
	for _, path := range stacks {
		if err := failures[path]; err != nil && !d.skipped(path) {
			return fmt.Errorf("discover %s: %w", path, err)
		}
	}
	// A template that parsed independently is still a template, not another live stack.
	d.result.Inventory = slices.DeleteFunc(d.result.Inventory, func(unit model.DiscoveredUnit) bool { return d.skipped(unit.GenerationRoot) })
	return nil
}

// discoverStandaloneUnits visits units that no stack owns. A unit already
// materialized by a stack (for example with no_dot_terragrunt_stack, or left on
// disk by an earlier generate) belongs to that stack and is not added twice.
func (d *projectDiscovery) discoverStandaloneUnits(units []string) error {
	for _, path := range units {
		if d.skipped(path) || d.generated[path] {
			continue
		}
		dir := filepath.Dir(path)
		stack, name := d.nameFor(filepath.Dir(dir)), filepath.Base(dir)
		if dir == d.root {
			stack, name = "default", "root"
		}
		if err := d.visit(path, unitPlacement{stack: stack, name: name}, 0); err != nil {
			return fmt.Errorf("discover %s: %w", path, err)
		}
	}
	return nil
}

// unitPlacement is the Atmos identity and provenance inherited through nested stacks.
type unitPlacement struct {
	stack, name, generationRoot string
	files, folders              []string
}

func (p unitPlacement) child(name, source string, files []string) unitPlacement {
	return unitPlacement{
		stack: p.stack, name: filepath.Join(p.name, name), generationRoot: p.generationRoot,
		files: files, folders: append(slices.Clone(p.folders), source),
	}
}

func (d *projectDiscovery) visit(path string, placement unitPlacement, depth int) error {
	if depth > maxStackDepth {
		return fmt.Errorf("nested-stack discovery depth exceeded at %s", path)
	}
	resolved, err := d.parse(path, false)
	if err != nil {
		return err
	}
	if filepath.Base(path) == stackFile {
		return d.visitStack(resolved, placement, depth)
	}
	return d.visitUnit(path, resolved, placement)
}

func (d *projectDiscovery) visitStack(resolved *model.Resolution, placement unitPlacement, depth int) error {
	files := append(slices.Clone(placement.files), resolved.Files...)
	for _, child := range slices.Sorted(maps.Keys(resolved.StackConfigs)) {
		source := resolved.StackSources[child]
		d.templates[source] = true
		if err := d.visit(resolved.StackConfigs[child], placement.child(resolved.Stacks[child], source, files), depth+1); err != nil {
			return err
		}
	}
	for _, child := range slices.Sorted(maps.Keys(resolved.UnitConfigs)) {
		source := resolved.UnitSources[child]
		d.templates[source] = true
		if physical, err := canonicalPath(resolved.UnitConfigs[child]); err == nil {
			d.generated[physical] = true
		}
		if err := d.visit(resolved.UnitConfigs[child], placement.child(child, source, files), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (d *projectDiscovery) visitUnit(path string, resolved *model.Resolution, placement unitPlacement) error {
	if len(resolved.Folders) == 0 {
		// Shared include, not a runnable local-source unit.
		return d.checkInlineModule(path)
	}
	if !plainLocalSource(resolved.Folders[0]) {
		return discoveryError(d.root, path, "NONLOCAL_SOURCE", fmt.Errorf("discovery requires local sources"))
	}
	resolved.Files = append(resolved.Files, placement.files...)
	resolved.Folders = append(resolved.Folders, placement.folders...)
	d.result.Inventory = append(d.result.Inventory, model.DiscoveredUnit{Stack: placement.stack, Name: filepath.ToSlash(placement.name), GenerationRoot: placement.generationRoot, Config: *resolved})
	return nil
}

func (d *projectDiscovery) checkInlineModule(path string) error {
	entries, err := afero.ReadDir(d.overlay, filepath.Dir(path))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".tf") || strings.HasSuffix(entry.Name(), ".tf.json")) {
			return discoveryError(d.root, path, "MISSING_TERRAFORM_SOURCE", fmt.Errorf("inline Terraform modules require terraform.source"))
		}
	}
	return nil
}

func checkIdentityCollisions(inventory []model.DiscoveredUnit) error {
	identities := map[string]string{}
	for _, unit := range inventory {
		key := unit.Stack + "\x00" + unit.Name
		if previous, exists := identities[key]; exists {
			return fmt.Errorf("discovered instance collision %s/%s: %s and %s", unit.Stack, unit.Name, previous, unit.Config.Path)
		}
		identities[key] = unit.Config.Path
	}
	return nil
}

// resolveStack reads the units and nested stacks one stack file declares. With
// generation, only this file and the nested stacks it declares are generated;
// other stack files under the same directory are never listed or generated.
func resolveStack(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*model.Resolution, error) {
	if generating(request) {
		if err := generateStack(ctx, logger, environment, request, path, 0); err != nil {
			return nil, err
		}
	}
	stack, err := readStack(ctx, logger, environment, pctx, path)
	if err != nil {
		return nil, err
	}
	return stackResolution(pctx, stack, path)
}

func readStack(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, path string) (*config.StackConfig, error) {
	values, err := config.ReadValues(ctx, logger, environment, pctx, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	return config.ReadStackConfigFile(ctx, logger, environment, pctx, path, values)
}

// generateStack generates path and then, recursively, each enabled nested stack
// it declares. Units are generated by their declaring stack file.
func generateStack(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, request model.Request, path string, depth int) error {
	if depth > maxStackDepth {
		return fmt.Errorf("nested-stack generation depth exceeded at %s", path)
	}
	if err := atmosbridge.GenerateStack(ctx, logger, environment, path, request.Tofu); err != nil {
		return err
	}
	stack, err := readStack(ctx, logger, environment, parsingContext(request, path), path)
	if err != nil {
		return err
	}
	for _, child := range stack.Stacks {
		if !child.IsEnabled() {
			continue
		}
		if err := generateStack(ctx, logger, environment, request, filepath.Join(child.GeneratedPath(filepath.Dir(path)), stackFile), depth+1); err != nil {
			return err
		}
	}
	return nil
}

// instanceIdentity keeps expansion elements that share a block label distinct.
// An unexpanded block keeps its label, so ordinary discovery is unchanged.
func instanceIdentity(kind, name, key string, expanded bool) (string, error) {
	if !expanded {
		return name, nil
	}
	if key == "" || key == "." || key == ".." || strings.ContainsAny(key, `/\`) {
		return "", fmt.Errorf("unsupported %s expansion key %q in %s", kind, key, name)
	}
	return name + "/" + key, nil
}

func stackResolution(pctx *config.ParsingContext, stack *config.StackConfig, path string) (*model.Resolution, error) {
	dir := filepath.Dir(path)
	result := &model.Resolution{
		Path: path, Files: append(pctx.FilesRead.Paths(), path),
		Units: map[string]string{}, UnitSources: map[string]string{}, UnitConfigs: map[string]string{},
		Stacks: map[string]string{}, StackSources: map[string]string{}, StackConfigs: map[string]string{},
	}
	for _, unit := range stack.Units {
		if !unit.IsEnabled() {
			continue
		}
		key, expanded := unit.InstanceKey()
		id, err := instanceIdentity("unit", unit.Name, key, expanded)
		if err != nil {
			return nil, err
		}
		if _, exists := result.Units[id]; exists {
			return nil, fmt.Errorf("duplicate unit instance %s in %s", id, path)
		}
		result.Units[id] = unit.Path
		result.UnitSources[id] = localPath(dir, unit.Source)
		result.UnitConfigs[id] = filepath.Join(unit.GeneratedPath(dir), unitFile)
	}
	for _, child := range stack.Stacks {
		if !child.IsEnabled() {
			continue
		}
		key, expanded := child.InstanceKey()
		id, err := instanceIdentity("stack", child.Name, key, expanded)
		if err != nil {
			return nil, err
		}
		if _, exists := result.Stacks[id]; exists {
			return nil, fmt.Errorf("duplicate stack instance %s in %s", id, path)
		}
		result.Stacks[id] = child.Path
		result.StackSources[id] = localPath(dir, child.Source)
		result.StackConfigs[id] = filepath.Join(child.GeneratedPath(dir), stackFile)
	}
	return result, nil
}

// resolveUnitInventory reads only the source and dependency blocks of a unit.
func resolveUnitInventory(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*model.Resolution, error) {
	cfg, err := config.PartialParseConfigFile(ctx, logger, environment,
		pctx.WithDecodeList(config.DependenciesBlock, config.DependencyBlock, config.TerraformSource), path, nil)
	if err != nil {
		return nil, err
	}
	result := &model.Resolution{Path: path, Files: append(pctx.FilesRead.Paths(), path)}
	if workspace := request.Environment["TF_WORKSPACE"]; workspace != "" && workspace != "default" && len(cfg.TerragruntDependencies) > 0 && cfg.Dependencies != nil && len(cfg.Dependencies.Paths) > 0 {
		return nil, fmt.Errorf("native output dependencies with non-default workspaces require a dependency backend adapter: %s", path)
	}
	if cfg.Terraform != nil && cfg.Terraform.Source != nil {
		result.Folders = []string{localPath(filepath.Dir(path), *cfg.Terraform.Source)}
	}
	if result.OutputDependencies, err = outputDependencies(cfg, path); err != nil {
		return nil, err
	}
	if result.Dependencies, err = orderDependencies(environment, cfg, path); err != nil {
		return nil, err
	}
	return result, nil
}

func outputDependencies(cfg *config.TerragruntConfig, path string) ([]string, error) {
	var result []string
	for _, dep := range cfg.TerragruntDependencies {
		if (dep.Enabled != nil && !*dep.Enabled) || (dep.SkipOutputs != nil && *dep.SkipOutputs) {
			continue
		}
		if !dep.ConfigPath.IsKnown() || dep.ConfigPath.IsNull() {
			continue
		}
		dependency := localPath(filepath.Dir(path), dep.ConfigPath.AsString())
		if filepath.Ext(dependency) != ".hcl" {
			dependency = filepath.Join(dependency, unitFile)
		}
		physical, err := canonicalPath(dependency)
		if err != nil {
			return nil, err
		}
		result = append(result, physical)
	}
	return result, nil
}

func orderDependencies(environment *atmosbridge.Venv, cfg *config.TerragruntConfig, path string) ([]string, error) {
	if cfg.Dependencies == nil {
		return nil, nil
	}
	var result []string
	for _, dependency := range cfg.Dependencies.Paths {
		if !filepath.IsAbs(dependency) {
			dependency = filepath.Join(filepath.Dir(path), dependency)
		}
		info, err := environment.FS.Stat(dependency)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			dependency = filepath.Join(dependency, unitFile)
		}
		physical, err := canonicalPath(dependency)
		if err != nil {
			return nil, fmt.Errorf("native adapter dependency must target a unit: %w", err)
		}
		result = append(result, physical)
	}
	return result, nil
}
