package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// DescribeLocalsArgs holds the arguments for the describe locals command.
type DescribeLocalsArgs struct {
	Component     string
	Query         string
	FilterByStack string
	Format        string
	File          string
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_describe_locals.go -package=$GOPACKAGE

// DescribeLocalsExec defines the interface for executing describe locals.
type DescribeLocalsExec interface {
	Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeLocalsArgs) error
}

type describeLocalsExec struct {
	pageCreator              pager.PageCreator
	isTTYSupportForStdout    func() bool
	printOrWriteToFile       func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	executeDescribeLocals    func(atmosConfig *schema.AtmosConfiguration, filterByStack string) (map[string]any, error)
	executeDescribeComponent func(params *ExecuteDescribeComponentParams) (map[string]any, error)
}

// NewDescribeLocalsExec creates a new DescribeLocalsExec instance.
func NewDescribeLocalsExec() DescribeLocalsExec {
	defer perf.Track(nil, "exec.NewDescribeLocalsExec")()

	return &describeLocalsExec{
		pageCreator:              pager.New(),
		isTTYSupportForStdout:    term.IsTTYSupportForStdout,
		printOrWriteToFile:       printOrWriteToFile,
		executeDescribeLocals:    ExecuteDescribeLocals,
		executeDescribeComponent: ExecuteDescribeComponent,
	}
}

// Execute executes the describe locals command.
func (d *describeLocalsExec) Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeLocalsArgs) error {
	defer perf.Track(atmosConfig, "exec.DescribeLocalsExec.Execute")()

	// Stack is required.
	if args.FilterByStack == "" {
		return errUtils.ErrStackRequired
	}

	var res any
	var err error

	// If component is specified, get locals for that specific component.
	if args.Component != "" {
		res, err = d.executeForComponent(atmosConfig, args)
		if err != nil {
			return err
		}
	} else {
		// Get locals for the specified stack.
		finalLocalsMap, err := d.executeDescribeLocals(atmosConfig, args.FilterByStack)
		if err != nil {
			return err
		}
		res = finalLocalsMap
	}

	// Apply query if specified.
	if args.Query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, res, args.Query)
		if err != nil {
			return err
		}
	}

	return viewWithScroll(&viewWithScrollProps{
		pageCreator:           d.pageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		printOrWriteToFile:    d.printOrWriteToFile,
		atmosConfig:           atmosConfig,
		displayName:           "Locals",
		format:                args.Format,
		file:                  args.File,
		res:                   res,
	})
}

// executeForComponent gets the locals for a specific component in a stack.
// Component-level locals are merged with stack-level locals (component locals take precedence).
func (d *describeLocalsExec) executeForComponent(
	atmosConfig *schema.AtmosConfiguration,
	args *DescribeLocalsArgs,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.DescribeLocalsExec.executeForComponent")()

	// Stack is validated in Execute(), but double-check for safety.
	if args.FilterByStack == "" {
		return nil, errUtils.ErrStackRequired
	}

	componentSection, err := d.executeDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            args.Component,
		Stack:                args.FilterByStack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe component %s in stack %s: %w", args.Component, args.FilterByStack, err)
	}

	componentType := getComponentType(componentSection)
	componentLocals := extractComponentLocals(componentSection)

	stackLocals, err := d.executeDescribeLocals(atmosConfig, args.FilterByStack)
	if err != nil {
		return nil, err
	}

	return buildComponentLocalsResult(args, stackLocals, componentType, componentLocals)
}

// getComponentType extracts the component type from a component section, defaulting to terraform.
func getComponentType(componentSection map[string]any) string {
	if ct, ok := componentSection["component_type"].(string); ok && ct != "" {
		return ct
	}
	return "terraform"
}

// extractComponentLocals extracts the locals section from a component section.
func extractComponentLocals(componentSection map[string]any) map[string]any {
	if cl, ok := componentSection[cfg.LocalsSectionName].(map[string]any); ok {
		return cl
	}
	return nil
}

// buildComponentLocalsResult builds the result map for component locals query.
// Output format matches Atmos stack manifest schema:
//
//	components:
//	  terraform:
//	    vpc:
//	      locals:
//	        foo: 123
func buildComponentLocalsResult(
	args *DescribeLocalsArgs,
	stackLocals map[string]any,
	componentType string,
	componentLocals map[string]any,
) (map[string]any, error) {
	// stackLocals is now in direct format (locals:, terraform:, etc.) without stack name wrapper.
	// Merge stack-level locals with component-level locals.
	if len(stackLocals) > 0 {
		stackTypeLocals := getLocalsForComponentType(stackLocals, componentType)
		mergedLocals := mergeLocals(stackTypeLocals, componentLocals)
		return buildComponentSchemaOutput(args.Component, componentType, mergedLocals), nil
	}

	if len(componentLocals) > 0 {
		return buildComponentSchemaOutput(args.Component, componentType, componentLocals), nil
	}

	return nil, fmt.Errorf("%w: %s", errUtils.ErrStackHasNoLocals, args.FilterByStack)
}

// buildComponentSchemaOutput creates the Atmos schema-compliant output for component locals.
func buildComponentSchemaOutput(component, componentType string, locals map[string]any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			componentType: map[string]any{
				component: map[string]any{
					cfg.LocalsSectionName: locals,
				},
			},
		},
	}
}

// mergeLocals deep-merges base and override locals maps.
// This uses the same deep-merge semantics as vars, settings, and other Atmos sections,
// so nested maps (e.g., tags: {env: dev, team: platform}) are recursively merged
// rather than entirely replaced.
func mergeLocals(base, override map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	if override == nil {
		return base
	}

	// Use pkg/merge for consistent deep-merge behavior with the rest of Atmos.
	// MergeWithOptions handles deep copying internally to avoid pointer mutation.
	result, err := m.MergeWithOptions(nil, []map[string]any{base, override}, false, false)
	if err != nil {
		// On merge error, fall back to shallow merge for robustness.
		log.Warn("Deep-merge failed, falling back to shallow merge", "error", err)
		result = make(map[string]any, len(base)+len(override))
		for k, v := range base {
			result[k] = v
		}
		for k, v := range override {
			result[k] = v
		}
	}

	return result
}

// getLocalsForComponentType extracts the appropriate merged locals for a component type.
// Input format is Atmos schema: locals: {...}, terraform: {locals: {...}}, etc.
// Uses mergeLocals for consistent deep-merge semantics with nested maps.
func getLocalsForComponentType(stackLocals map[string]any, componentType string) map[string]any {
	result := make(map[string]any)

	// Start with global locals (root-level "locals:" key).
	if globalLocals, ok := stackLocals[cfg.LocalsSectionName].(map[string]any); ok {
		result = mergeLocals(result, globalLocals)
	}

	// Merge section-specific locals (e.g., "terraform: locals:").
	if sectionMap, ok := stackLocals[componentType].(map[string]any); ok {
		if sectionLocals, ok := sectionMap[cfg.LocalsSectionName].(map[string]any); ok {
			result = mergeLocals(result, sectionLocals)
		}
	}

	return result
}

// stackFileLocalsResult holds the result of processing a stack file for locals.
type stackFileLocalsResult struct {
	StackName   string         // Derived stack name (empty if filtered out or unparseable).
	StackLocals map[string]any // Locals extracted from the stack file.
	Found       bool           // Whether the stack matched the filter (even if no locals).
}

// ExecuteDescribeLocals processes stack manifests and returns the locals for the specified stack.
// It reads the raw YAML files directly since locals are stripped during normal stack processing.
// The output format matches the stack manifest schema (locals at root level).
func ExecuteDescribeLocals(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeLocals")()

	// Normalize path separators for Windows compatibility.
	// deriveStackFileName returns forward-slash paths, so we need to match that format.
	filterByStack = filepath.ToSlash(filterByStack)

	stackFound := false
	var stackLocals map[string]any

	// Process each stack config file directly.
	for _, filePath := range atmosConfig.StackConfigFilesAbsolutePaths {
		result, err := processStackFileForLocals(atmosConfig, filePath, filterByStack)
		if err != nil {
			return nil, err
		}

		// Track if we found a matching stack (even with no locals).
		if result.Found {
			stackFound = true
			stackLocals = result.StackLocals
			break // Found the matching stack, no need to continue.
		}
	}

	// Validate the result.
	if !stackFound {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, filterByStack)
	}
	if len(stackLocals) == 0 {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackHasNoLocals, filterByStack)
	}

	return stackLocals, nil
}

// parseStackFileYAML reads and parses a stack file's YAML content.
// Returns the raw config, or nil if the file should be skipped.
// If filterMatchesFileName is true, parse errors return an error; otherwise they are logged and skipped.
func parseStackFileYAML(filePath string, filterMatchesFileName bool) (map[string]any, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Join(errUtils.ErrInvalidStackManifest, fmt.Errorf("failed to read stack file %s: %w", filePath, err))
	}

	var rawConfig map[string]any
	if err := yaml.Unmarshal(content, &rawConfig); err != nil {
		if filterMatchesFileName {
			return nil, errors.Join(errUtils.ErrInvalidStackManifest, fmt.Errorf("failed to parse YAML in %s: %w", filePath, err))
		}
		log.Warn("Skipping file with YAML parse error", "file", filePath, "error", err)
		return nil, nil //nolint:nilnil // nil config signals skip without error
	}

	return rawConfig, nil
}

// stackMatchesFilter checks if a stack matches the filter criteria.
// Returns true if no filter is specified or if the filter matches either the filename or derived name.
func stackMatchesFilter(filterByStack, stackFileName, stackName string) bool {
	if filterByStack == "" {
		return true
	}
	return filterByStack == stackFileName || filterByStack == stackName
}

// processStackFileForLocals reads a stack file and extracts its locals.
// Returns a result struct with stack name, locals, and whether the stack matched the filter.
func processStackFileForLocals(
	atmosConfig *schema.AtmosConfiguration,
	filePath string,
	filterByStack string,
) (*stackFileLocalsResult, error) {
	stackFileName := deriveStackFileName(atmosConfig, filePath)
	filterMatchesFileName := filterByStack != "" && filterByStack == stackFileName

	rawConfig, err := parseStackFileYAML(filePath, filterMatchesFileName)
	if err != nil {
		return nil, err
	}
	if rawConfig == nil {
		return &stackFileLocalsResult{}, nil
	}

	// Extract vars for stack name derivation.
	var varsSection map[string]any
	if vs, ok := rawConfig[cfg.VarsSectionName].(map[string]any); ok {
		varsSection = vs
	}

	stackName := deriveStackName(atmosConfig, stackFileName, varsSection, rawConfig)

	// Apply filter if specified.
	if !stackMatchesFilter(filterByStack, stackFileName, stackName) {
		return &stackFileLocalsResult{}, nil
	}

	localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath, stackName)
	if err != nil {
		return &stackFileLocalsResult{Found: true}, fmt.Errorf("failed to process locals for stack %s: %w", stackFileName, err)
	}

	return &stackFileLocalsResult{
		StackName:   stackName,
		StackLocals: buildStackLocalsFromContext(localsCtx),
		Found:       true,
	}, nil
}

// buildStackLocalsFromContext converts a LocalsContext to a map using Atmos schema format.
// Returns an empty map if localsCtx is nil or has no locals.
// Output format matches Atmos stack manifest schema:
//
//	locals:
//	  foo: 123
//	terraform:
//	  locals:
//	    xyz: 123
func buildStackLocalsFromContext(localsCtx *LocalsContext) map[string]any {
	stackLocals := make(map[string]any)

	if localsCtx == nil {
		return stackLocals
	}

	// Global locals go under root "locals:" key.
	if len(localsCtx.Global) > 0 {
		stackLocals[cfg.LocalsSectionName] = localsCtx.Global
	}

	// Section-specific locals go under "terraform: locals:", "helmfile: locals:", etc.
	if localsCtx.HasTerraformLocals && len(localsCtx.Terraform) > 0 {
		stackLocals[cfg.TerraformSectionName] = map[string]any{
			cfg.LocalsSectionName: getSectionOnlyLocals(localsCtx.Terraform, localsCtx.Global),
		}
	}

	if localsCtx.HasHelmfileLocals && len(localsCtx.Helmfile) > 0 {
		stackLocals[cfg.HelmfileSectionName] = map[string]any{
			cfg.LocalsSectionName: getSectionOnlyLocals(localsCtx.Helmfile, localsCtx.Global),
		}
	}

	if localsCtx.HasPackerLocals && len(localsCtx.Packer) > 0 {
		stackLocals[cfg.PackerSectionName] = map[string]any{
			cfg.LocalsSectionName: getSectionOnlyLocals(localsCtx.Packer, localsCtx.Global),
		}
	}

	return stackLocals
}

// getSectionOnlyLocals extracts locals that are unique to a section (not inherited from global).
// Since section locals are already merged with global, we need to extract only the section-specific ones.
func getSectionOnlyLocals(sectionLocals, globalLocals map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range sectionLocals {
		// Include if key doesn't exist in global, or if value differs from global.
		if globalVal, exists := globalLocals[k]; !exists || !valuesEqual(v, globalVal) {
			result[k] = v
		}
	}
	return result
}

// valuesEqual compares two values for deep equality.
func valuesEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

// deriveStackFileName extracts the stack file name from the absolute path.
// It removes the stacks base path and file extension to get the relative stack name.
// The returned path always uses forward slashes for consistency across platforms.
func deriveStackFileName(atmosConfig *schema.AtmosConfiguration, filePath string) string {
	defer perf.Track(atmosConfig, "exec.deriveStackFileName")()

	// Get the relative path from the stacks base path.
	stacksBasePath := atmosConfig.StacksBaseAbsolutePath
	if stacksBasePath == "" {
		// Fallback: just use the file name without extension.
		base := filepath.Base(filePath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Get relative path.
	relPath, err := filepath.Rel(stacksBasePath, filePath)
	if err != nil {
		// Fallback: just use the file name without extension.
		base := filepath.Base(filePath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Remove the extension and normalize path separators to forward slashes.
	result := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	return filepath.ToSlash(result)
}

// deriveStackName derives the stack name using the same logic as describe stacks.
//
// Stack-name derivation must see the merged view of vars/settings/env across
// imports because `name_template` (and `name_pattern`) commonly reference
// values that live in parent `_defaults.yaml` files. We do a lite, YAML-only
// import walk here -- no template processing, no YAML function resolution --
// so the derivation is fast and side-effect-free even when called repeatedly
// (the locals pre-pass and the main pipeline both reach this).
//
// Regression: GitHub issues #2343 (vars from imports) and #2374 (settings
// from imports). Before the fix, only `varsSection` from the leaf file was
// available, producing malformed names like "-prod" or "<no value>-prod".
func deriveStackName(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	varsSection map[string]any,
	stackSectionMap map[string]any,
) string {
	defer perf.Track(atmosConfig, "exec.deriveStackName")()

	// Try explicit name from manifest first.
	if name := getExplicitStackName(stackSectionMap); name != "" {
		return name
	}

	// Lite-merge vars/settings/env from imports so name_template (which can
	// reference any of them) sees the full picture rather than just the leaf
	// file. The current file's sections take precedence over imports.
	mergedVars, mergedSettings, mergedEnv := deriveStackNameSections(atmosConfig, stackSectionMap, stackFileName)
	// Always include the caller's varsSection on top -- handles the case
	// where the caller already merged or pre-processed vars.
	for k, v := range varsSection {
		if mergedVars == nil {
			mergedVars = map[string]any{}
		}
		mergedVars[k] = v
	}

	// Try name template using merged sections.
	if name := deriveStackNameFromTemplate(atmosConfig, stackFileName, mergedVars, mergedSettings, mergedEnv); name != "" {
		return name
	}

	// Try name pattern using merged vars.
	if name := deriveStackNameFromPattern(atmosConfig, stackFileName, mergedVars); name != "" {
		return name
	}

	// Default: use stack filename.
	return stackFileName
}

// getExplicitStackName extracts an explicit name from the stack manifest if defined.
func getExplicitStackName(stackSectionMap map[string]any) string {
	nameValue, ok := stackSectionMap[cfg.NameSectionName]
	if !ok {
		return ""
	}
	name, ok := nameValue.(string)
	if !ok || name == "" {
		return ""
	}
	return name
}

// deriveStackNameFromTemplate derives a stack name using the configured name template.
// Returns empty string if template is not configured or evaluation fails.
//
// TemplateData includes vars, settings, and env so that name_template can
// reference any of them. Pre-fix this only included vars, breaking projects
// that use `name_template: "{{ .settings.* }}"` (GitHub #2374).
//
// Renders with `ignoreMissingTemplateValues=true` so missing keys produce
// `<no value>` rather than erroring out -- the pre-pass shouldn't fail just
// because the leaf file is missing identifying values; it should fall back
// to the filename. We then explicitly reject any rendered name that contains
// `<no value>` or has empty segments around the configured delimiter so
// downstream code never sees a malformed identifier (GitHub #2343).
func deriveStackNameFromTemplate(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	varsSection, settingsSection, envSection map[string]any,
) string {
	if atmosConfig.Stacks.NameTemplate == "" {
		return ""
	}

	// Provide vars + settings + env so name_template can reference any
	// of them. Empty maps are passed (not nil) so missingkey=default still
	// produces "<no value>" rather than panicking on nil.
	templateData := map[string]any{
		cfg.VarsSectionName:     varsSection,
		cfg.SettingsSectionName: settingsSection,
		cfg.EnvSectionName:      envSection,
	}
	if templateData[cfg.VarsSectionName] == nil {
		templateData[cfg.VarsSectionName] = map[string]any{}
	}
	if templateData[cfg.SettingsSectionName] == nil {
		templateData[cfg.SettingsSectionName] = map[string]any{}
	}
	if templateData[cfg.EnvSectionName] == nil {
		templateData[cfg.EnvSectionName] = map[string]any{}
	}

	stackName, err := ProcessTmpl(atmosConfig, "describe-locals-name-template", atmosConfig.Stacks.NameTemplate, templateData, true)
	if err != nil {
		log.Debug("Failed to evaluate name template for stack", "file", stackFileName, "error", err)
		return ""
	}

	if stackName == "" {
		return ""
	}

	// If vars contain unresolved templates (e.g., "{{ .locals.* }}"), the result
	// will contain raw template markers. Fall back to empty (use filename).
	if strings.Contains(stackName, "{{") || strings.Contains(stackName, "}}") {
		log.Debug("Name template result contains unresolved templates, using filename", "file", stackFileName, "result", stackName)
		return ""
	}

	// Reject any rendering that came from missing keys -- these are not
	// usable as identifiers. Falls back to the filename.
	if strings.Contains(stackName, "<no value>") {
		log.Debug("Name template result contains <no value>, using filename",
			"file", stackFileName, "result", stackName,
			"hint", "ensure required vars/settings are defined or imported")
		return ""
	}

	return stackName
}

// deriveStackNameSections walks the import graph starting from the given
// raw config and returns vars/settings/env merged across this file and all
// transitively-imported files. Imports become the base; the current file
// overrides them (matching the main pipeline's merge semantics).
//
// This is a lite, YAML-only overlay used SOLELY to feed `name_template`
// during stack-name derivation. It deliberately does NOT process Go
// templates or YAML functions -- the main pipeline does that later. Any
// import path that can't be resolved is silently skipped: a best-effort
// merge that errs on the side of producing a usable stack name (or
// falling back to the filename) rather than failing.
//
// Cycle detection via the visited set; deterministic top-level (last-wins)
// merge sufficient because name_template typically references scalar
// fields like `vars.namespace`, `settings.tenant`.
//
// Regression: GitHub issues #2343 and #2374. Without this walk, the
// locals pre-pass renders `name_template` against the leaf file alone,
// missing identifying values defined in parent `_defaults.yaml`.
func deriveStackNameSections(
	atmosConfig *schema.AtmosConfiguration,
	rawConfig map[string]any,
	stackFileName string,
) (vars, settings, env map[string]any) {
	defer perf.Track(atmosConfig, "exec.deriveStackNameSections")()

	if atmosConfig == nil || rawConfig == nil {
		return nil, nil, nil
	}

	visited := make(map[string]bool)
	return deriveStackNameSectionsInto(atmosConfig, rawConfig, stackFileName, visited)
}

// deriveStackNameSectionsInto is the recursive helper for deriveStackNameSections.
// It mutates the visited set to break cycles and is not safe for concurrent use.
func deriveStackNameSectionsInto(
	atmosConfig *schema.AtmosConfiguration,
	rawConfig map[string]any,
	originatingFilePath string,
	visited map[string]bool,
) (vars, settings, env map[string]any) {
	if rawConfig == nil {
		return nil, nil, nil
	}

	vars = make(map[string]any)
	settings = make(map[string]any)
	env = make(map[string]any)

	// Walk imports DFS, merging their sections as the base.
	dest := stackNameSectionMaps{vars: vars, settings: settings, env: env}
	mergeImportedStackNameSections(atmosConfig, rawConfig, originatingFilePath, visited, dest)

	// Apply current file's sections last so they override imports.
	if v, ok := rawConfig[cfg.VarsSectionName].(map[string]any); ok {
		mergeMapShallow(vars, v)
	}
	if s, ok := rawConfig[cfg.SettingsSectionName].(map[string]any); ok {
		mergeMapShallow(settings, s)
	}
	if e, ok := rawConfig[cfg.EnvSectionName].(map[string]any); ok {
		mergeMapShallow(env, e)
	}
	return vars, settings, env
}

// stackNameSectionMaps bundles the three destination maps that
// mergeImportedStackNameSections / loadAndMergeStackNameImport accumulate
// into, keeping the helpers' arg lists below the linter's per-function
// limit.
type stackNameSectionMaps struct {
	vars     map[string]any
	settings map[string]any
	env      map[string]any
}

// mergeImportedStackNameSections walks imports of the given config and merges
// their vars/settings/env into dest. Each import is processed at most once
// per top-level call (cycle break via visited). Failures (unresolvable path,
// bad YAML) are silently skipped; this is a best-effort overlay used solely
// for stack-name derivation.
func mergeImportedStackNameSections(
	atmosConfig *schema.AtmosConfiguration,
	rawConfig map[string]any,
	originatingFilePath string,
	visited map[string]bool,
	dest stackNameSectionMaps,
) {
	importStructs, err := ProcessImportSection(rawConfig, originatingFilePath)
	if err != nil {
		log.Debug("deriveStackNameSections: failed to parse imports; continuing with leaf file only",
			"file", originatingFilePath, "error", err)
		return
	}

	stacksBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
	for _, imp := range importStructs {
		for _, p := range resolveImportFilePathsForStackName(stacksBasePath, imp.Path) {
			loadAndMergeStackNameImport(atmosConfig, p, visited, dest)
		}
	}
}

// loadAndMergeStackNameImport loads a single resolved import path, recurses
// into it, and merges its vars/settings/env into dest. A no-op when the path
// was already visited or YAML can't be parsed.
func loadAndMergeStackNameImport(
	atmosConfig *schema.AtmosConfiguration,
	importedFilePath string,
	visited map[string]bool,
	dest stackNameSectionMaps,
) {
	abs, err := filepath.Abs(importedFilePath)
	if err != nil {
		abs = importedFilePath
	}
	if visited[abs] {
		return
	}
	visited[abs] = true

	content, err := os.ReadFile(importedFilePath)
	if err != nil {
		return
	}
	var importedConfig map[string]any
	if err := yaml.Unmarshal(content, &importedConfig); err != nil {
		// Likely a .yaml.tmpl file or other non-pure-YAML; skip.
		return
	}
	iv, is, ie := deriveStackNameSectionsInto(atmosConfig, importedConfig, importedFilePath, visited)
	mergeMapShallow(dest.vars, iv)
	mergeMapShallow(dest.settings, is)
	mergeMapShallow(dest.env, ie)
}

// stackNameImportYAMLExts is the ordered list of file extensions tried when
// resolving an import path during stack-name derivation. Order matters:
// .yaml wins over .yml when both exist.
var stackNameImportYAMLExts = []string{
	u.YamlFileExtension,
	u.YmlFileExtension,
}

// resolveImportFilePathsForStackName resolves an import path to existing file
// paths under the stacks base. The path may be either absolute (when the
// original was `./xxx` and was already resolved by ResolveRelativePath in
// ProcessImportSection) or relative-to-stacks-base. Returns existing matches;
// silently returns nil for unresolvable paths since this is best-effort.
func resolveImportFilePathsForStackName(stacksBasePath, importPath string) []string {
	if importPath == "" {
		return nil
	}

	// Determine the search base.
	searchPath := importPath
	if !filepath.IsAbs(importPath) {
		searchPath = filepath.Join(stacksBasePath, importPath)
	}

	// If the path already has a recognized YAML extension and the file
	// exists, use it directly.
	if hasYAMLExt(searchPath) {
		if _, err := os.Stat(searchPath); err == nil {
			return []string{searchPath}
		}
	}

	// Otherwise try common extensions; first match wins.
	if found := findFirstExistingWithExt(searchPath, stackNameImportYAMLExts); found != "" {
		return []string{found}
	}

	// Glob fallback (handles `mixins/region/*` style imports).
	return globMatchesWithExt(searchPath, stackNameImportYAMLExts)
}

// hasYAMLExt reports whether path ends in `.yaml` or `.yml`.
func hasYAMLExt(path string) bool {
	ext := filepath.Ext(path)
	return ext == u.YamlFileExtension || ext == u.YmlFileExtension
}

// findFirstExistingWithExt returns the first `searchPath+ext` that exists on
// disk, or an empty string if none of them do.
func findFirstExistingWithExt(searchPath string, exts []string) string {
	for _, e := range exts {
		candidate := searchPath + e
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// globMatchesWithExt returns the first non-empty glob match for
// `searchPath+ext` across the supplied extensions.
func globMatchesWithExt(searchPath string, exts []string) []string {
	for _, e := range exts {
		matches, err := u.GetGlobMatches(searchPath + e)
		if err == nil && len(matches) > 0 {
			return matches
		}
	}
	return nil
}

// mergeMapShallow merges src into dst at the top level (last wins). Nil src
// is a no-op.
func mergeMapShallow(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

// deriveStackNameFromPattern derives a stack name using the configured name pattern.
// Returns empty string if pattern is not configured or evaluation fails.
func deriveStackNameFromPattern(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	varsSection map[string]any,
) string {
	pattern := GetStackNamePattern(atmosConfig)
	if pattern == "" {
		return ""
	}

	context := cfg.GetContextFromVars(varsSection)
	stackName, err := cfg.GetContextPrefix(stackFileName, context, pattern, stackFileName)
	if err != nil {
		log.Debug("Failed to evaluate name pattern for stack", "file", stackFileName, "error", err)
		return ""
	}

	return stackName
}
