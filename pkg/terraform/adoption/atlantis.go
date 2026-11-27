package adoption

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

const (
	atlantisDefault     = "default"
	atlantisPlan        = "plan"
	atlantisInit        = "init"
	atlantisVarFlag     = "-var-file"
	atlantisProjectFlag = "--project"
)

type atlantisConfig struct {
	Version   int
	Projects  []atlantisProject
	Workflows map[string]atlantisWorkflow
	Other     map[string]yaml.Node `yaml:",inline"`
}

type atlantisProject struct {
	Name, Dir, Workspace, Workflow string
	Distribution                   string               `yaml:"terraform_distribution"`
	Version                        string               `yaml:"terraform_version"`
	Other                          map[string]yaml.Node `yaml:",inline"`
}

type atlantisWorkflow struct {
	Plan *struct {
		Steps []yaml.Node
		Other map[string]yaml.Node `yaml:",inline"`
	}
	Other map[string]yaml.Node `yaml:",inline"`
}

func (r *request) saveAtlantis(ctx context.Context) error {
	cfg, err := readAtlantis(r.Dir)
	if err != nil {
		return err
	}
	fmt.Fprintln(r.Output, "Atlantis retains PR approvals, locking, apply and server-side workflows.")
	if len(cfg.Projects) == 0 {
		fmt.Fprintln(r.Output, "No explicit v3 projects. Automatic discovery requires Atlantis.")
	}
	names := make([]string, 0, len(cfg.Projects))
	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		name := p.selector()
		if slices.Contains(names, name) {
			return fmt.Errorf("%w: duplicate Atlantis project selector %q", errUtils.ErrInvalidComponent, name)
		}
		names = append(names, name)
		cfg.describe(r, r.Output, filepath.Dir(r.Dir), p)
	}
	if r.DryRun && r.AtlantisProject == "" {
		return nil
	}
	selected, err := r.selectAtlantis(names)
	if err != nil {
		return err
	}
	p := &cfg.Projects[selected]
	files, err := cfg.planFiles(p)
	if err != nil {
		return fmt.Errorf("atlantis project %s: %w", p.selector(), err)
	}
	return r.registerAtlantis(ctx, p, files)
}

func readAtlantis(path string) (atlantisConfig, error) {
	var config atlantisConfig
	data, _, err := stackadoption.ReadRegular(path)
	if err != nil {
		return config, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("%s: %w", path, err)
	}
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return config, fmt.Errorf("%w: expected one YAML document in %s", errUtils.ErrInvalidStackConfig, path)
	}
	if config.Version != 3 {
		return config, fmt.Errorf("%w: %s must declare Atlantis version 3", errUtils.ErrInvalidStackConfig, path)
	}
	err = atlantisFields(config.Other, "automerge parallel_plan parallel_apply delete_source_branch_on_merge allowed_regexp_prefixes autodiscover policies repo_locks emoji_reaction abort_on_execution_order_fail silence_pr_comments")
	return config, err
}

func atlantisFields(fields map[string]yaml.Node, allowed string) error {
	names := strings.Fields(allowed)
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(names, key) {
			return fmt.Errorf("%w: unsupported Atlantis field %q", errUtils.ErrInvalidStackConfig, key)
		}
	}
	return nil
}

func (p *atlantisProject) selector() string {
	return cmp.Or(p.Name, p.Dir+"@"+cmp.Or(p.Workspace, atlantisDefault))
}

func (c *atlantisConfig) describe(r *request, out io.Writer, repo string, p *atlantisProject) {
	fmt.Fprintf(out, "Project %s (dir %q, workspace %q, workflow %q)\n", p.selector(), p.Dir, cmp.Or(p.Workspace, atlantisDefault), cmp.Or(p.Workflow, "built-in default assumed"))
	for _, k := range []string{"plan_requirements", "apply_requirements", "import_requirements"} {
		if n, ok := p.Other[k]; ok {
			var gates []string
			if err := n.Decode(&gates); err == nil {
				fmt.Fprintf(out, "  Atlantis %s: %s\n", k, strings.Join(gates, ", "))
			}
		}
	}
	_, err := c.planFiles(p)
	if err == nil {
		_, err = r.atlantisRoot(repo, p.Dir)
	}
	if err != nil {
		fmt.Fprintf(out, "  Inventory only: %v\n", err)
	}
}

func (r *request) selectAtlantis(names []string) (int, error) {
	switch {
	case len(names) == 0:
		return 0, fmt.Errorf("%w: no explicit Atlantis projects; automatic discovery requires Atlantis", errUtils.ErrInvalidComponent)
	case r.AtlantisProject != "":
		if i := slices.Index(names, r.AtlantisProject); i >= 0 {
			return i, nil
		}
		return 0, fmt.Errorf("%w: unknown --project %q; choose from %s", errUtils.ErrInvalidComponent, r.AtlantisProject, strings.Join(names, ", "))
	case len(names) == 1:
		return 0, nil
	case r.Interactive:
		s, err := flags.PromptForValue("project", "Which Atlantis project should be registered?", names)
		if err != nil {
			return 0, err
		}
		if i := slices.Index(names, s); i >= 0 {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: select one Atlantis project with %s (%s)", errUtils.ErrInvalidComponent, atlantisProjectFlag, strings.Join(names, ", "))
}

func (c *atlantisConfig) planFiles(p *atlantisProject) ([]string, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if p.Workflow == "" {
		return nil, nil
	}
	w, ok := c.Workflows[p.Workflow]
	if !ok {
		return nil, fmt.Errorf("%w: workflow %q is unavailable in atlantis.yaml; supply the effective server workflow", errUtils.ErrInvalidComponent, p.Workflow)
	}
	files, err := w.files()
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", p.Workflow, err)
	}
	return files, nil
}

func (p *atlantisProject) validate() error {
	if !literalAtlantisPath(p.Dir) || !filepath.IsLocal(p.Dir) || slices.Contains(strings.Split(p.Dir, "/"), "..") {
		return fmt.Errorf("%w: dir %q must select a literal project directory", errUtils.ErrInvalidComponent, p.Dir)
	}
	if ws := p.Workspace; ws != "" && !validAtlantisWorkspace(ws) {
		return fmt.Errorf("%w: invalid Atlantis workspace %q", errUtils.ErrInvalidComponent, ws)
	}
	if err := p.validateEngine(); err != nil {
		return err
	}
	return p.validateFields()
}

func (p *atlantisProject) validateEngine() error {
	if p.Version != "" && !releasePin.MatchString(p.Version) {
		return fmt.Errorf("%w: terraform_version must be an exact release, got %q", errUtils.ErrInvalidToolSpec, p.Version)
	}
	if p.Distribution != "" && p.Distribution != "opentofu" && p.Distribution != terraform {
		return fmt.Errorf("%w: unsupported terraform_distribution %q", errUtils.ErrInvalidToolSpec, p.Distribution)
	}
	return nil
}

func (p *atlantisProject) validateFields() error {
	if auto, ok := p.Other["autoplan"]; ok {
		var fields map[string]yaml.Node
		if err := auto.Decode(&fields); err != nil {
			return err
		}
		if err := atlantisFields(fields, "enabled when_modified"); err != nil {
			return fmt.Errorf("autoplan: %w", err)
		}
	}
	return atlantisFields(p.Other, "branch autoplan depends_on execution_order_group repo_locks repo_locking delete_source_branch_on_merge plan_requirements apply_requirements import_requirements silence_pr_comments policy_check custom_policy_check")
}

func validAtlantisWorkspace(s string) bool {
	return filepath.IsLocal(s) && filepath.Base(s) == s && s != "." && literalAtlantisPath(s) && strings.IndexFunc(s, unicode.IsSpace) < 0 && !strings.Contains(s, ":")
}

func literalAtlantisPath(s string) bool {
	return s != "" && !strings.ContainsAny(s, "\\$\x60*?[]{}~\x00") && strings.IndexFunc(s, unicode.IsControl) < 0
}

func (w *atlantisWorkflow) files() ([]string, error) {
	if err := atlantisFields(w.Other, "apply policy_check import state_rm"); err != nil {
		return nil, err
	}
	if w.Plan == nil {
		return nil, nil
	}
	if err := atlantisFields(w.Plan.Other, ""); err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	if w.Plan.Steps == nil {
		return nil, nil
	}
	var files []string
	for i := range w.Plan.Steps {
		name, args, err := atlantisStep(&w.Plan.Steps[i])
		if err == nil {
			files, err = collectPlanFiles(i, name, args)
		}
		if err != nil {
			return nil, fmt.Errorf("plan step %d (%s): %w", i+1, name, err)
		}
	}
	if len(w.Plan.Steps) != 2 {
		return nil, fmt.Errorf("%w: plan stage must contain built-in init followed by plan", errUtils.ErrInvalidComponent)
	}
	return files, nil
}

func atlantisStep(node *yaml.Node) (string, []string, error) {
	if node.Kind == yaml.AliasNode {
		return atlantisStep(node.Alias)
	}
	if node.Kind == yaml.ScalarNode && node.ShortTag() == "!!str" {
		return node.Value, nil, nil
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return "unknown", nil, fmt.Errorf("%w: expected a built-in step", errUtils.ErrInvalidComponent)
	}
	name := node.Content[0].Value
	if name != atlantisInit && name != atlantisPlan {
		return name, nil, fmt.Errorf("%w: requires manual translation", errUtils.ErrInvalidComponent)
	}
	var options struct {
		Args  []string             `yaml:"extra_args"`
		Other map[string]yaml.Node `yaml:",inline"`
	}
	if err := node.Content[1].Decode(&options); err != nil {
		return name, nil, err
	}
	return name, options.Args, atlantisFields(options.Other, "")
}

func collectPlanFiles(index int, name string, args []string) ([]string, error) {
	switch {
	case index == 0 && name == atlantisInit && len(args) == 0:
		return nil, nil
	case index == 1 && name == atlantisPlan:
		return atlantisVarFiles(args)
	default:
		return nil, fmt.Errorf("%w: only built-in init then plan with literal -var-file arguments can be registered", errUtils.ErrInvalidComponent)
	}
}

func atlantisVarFiles(args []string) ([]string, error) {
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		path, ok := strings.CutPrefix(arg, atlantisVarFlag+"=")
		if !ok && arg == atlantisVarFlag && i+1 < len(args) {
			i++
			path = args[i]
			ok = true
		}
		if !ok || !literalAtlantisPath(path) || filepath.IsAbs(path) || strings.HasPrefix(path, "-") {
			return nil, fmt.Errorf("%w: unsupported plan argument at position %d; expected a literal relative -var-file", errUtils.ErrInvalidComponent, i+1)
		}
		files = append(files, path)
	}
	return files, nil
}

func (r *request) registerAtlantis(ctx context.Context, p *atlantisProject, files []string) error {
	imported := *r
	source, err := filepath.EvalSymlinks(r.Dir)
	if err != nil {
		return err
	}
	rel, err := stackadoption.RelativeDirectory(r.Project, source)
	if err != nil {
		return err
	}
	dir, err := r.atlantisRoot(filepath.Dir(source), p.Dir)
	if err != nil {
		return err
	}
	imported.Dir = dir
	imported.Name = cmp.Or(r.Name, p.Name, filepath.Base(dir))
	if !stackadoption.ValidName(imported.Name) {
		return fmt.Errorf("%w: choose a component name with --name", errUtils.ErrInvalidComponent)
	}
	imported.Explicit, err = p.engine(r.Explicit)
	if err != nil {
		return err
	}
	imported.atlantisWorkspace = cmp.Or(p.Workspace, atlantisDefault)
	imported.atlantisVersion = strings.TrimPrefix(p.Version, "v")
	imported.atlantisSource = rel + "#" + p.selector()
	imported.VarFiles, err = atlantisWorkspaceFiles(dir, imported.atlantisWorkspace, files)
	if err != nil {
		return err
	}
	return imported.saveTerraform(ctx)
}

// Resolve the selected root without evaluating configuration or invoking its CLI.
func (r *request) atlantisRoot(repo, projectDir string) (string, error) {
	dir, err := filepath.EvalSymlinks(filepath.Join(repo, projectDir))
	if err != nil {
		return "", err
	}
	if _, err := stackadoption.RelativeDirectory(repo, dir); err != nil {
		return "", err
	}
	adapter, _, err := r.Adapters.Detect(dir)
	if errors.Is(err, stackadoption.ErrNoMatch) {
		return "", fmt.Errorf("%w: expected a Terraform or OpenTofu root in %s", errUtils.ErrInvalidComponent, dir)
	}
	if err != nil {
		return "", err
	}
	if adapter.Kind() == tg {
		return "", fmt.Errorf("%w: this Atlantis project selects a Terragrunt unit; register its unit path directly after reviewing the Atlantis workflow", errUtils.ErrInvalidComponent)
	}
	if adapter.Kind() != terraform {
		return "", fmt.Errorf("%w: expected a Terraform or OpenTofu root in %s", errUtils.ErrInvalidComponent, dir)
	}
	return dir, nil
}

func (p *atlantisProject) engine(explicit string) (string, error) {
	cmd := terraform
	if p.Distribution == "opentofu" {
		cmd = tofu
	}
	if explicit == "" || explicit == cmd {
		return cmd, nil
	}
	// The repo file can omit the server's default distribution. Accept an explicit local choice.
	if p.Distribution == "" && (explicit == terraform || explicit == tofu) {
		return explicit, nil
	}
	return "", fmt.Errorf("%w: --command %q conflicts with Atlantis distribution %q", errUtils.ErrInvalidToolSpec, explicit, cmd)
}

func atlantisWorkspaceFiles(dir, workspace string, files []string) ([]string, error) {
	// Atlantis v0.50 appends this workspace file after explicit plan arguments.
	name := filepath.Join("env", workspace+".tfvars")
	_, err := os.Stat(filepath.Join(dir, name))
	if os.IsNotExist(err) {
		return files, nil
	}
	if err != nil {
		return nil, err
	}
	return append(slices.Clone(files), name), nil
}

func (r *request) checkAtlantisTarget(ctx context.Context, e *Engine, workspace string) error {
	if r.atlantisWorkspace != "" && workspace != r.atlantisWorkspace {
		return fmt.Errorf("%w: Atlantis selects workspace %q; select that existing workspace in %s (current %q) and retry", errUtils.ErrInvalidStackConfig, r.atlantisWorkspace, r.Dir, workspace)
	}
	if r.atlantisVersion == "" {
		return nil
	}
	want := "Terraform v" + r.atlantisVersion
	if e.Command == tofu {
		want = "OpenTofu v" + r.atlantisVersion
	}
	if got := versionLine(ctx, r.Dir, e.Path); got != want {
		return fmt.Errorf("%w: Atlantis requires %s; selected binary reports %s", errUtils.ErrInvalidToolSpec, want, got)
	}
	e.Pin = r.atlantisVersion
	return nil
}

func (r *request) addAtlantisSource(data []byte, dest string, exists bool) ([]byte, error) {
	key := dest + ".metadata.custom.atlantis_project"
	if r.atlantisSource == "" {
		return data, nil
	}
	if exists {
		value, err := y.Get(data, key)
		if err == nil && value == r.atlantisSource {
			return data, nil
		}
		return nil, fmt.Errorf("%w: existing component is not bound to %s", errUtils.ErrInvalidComponent, r.atlantisSource)
	}
	return y.SetWithType(data, key, r.atlantisSource, y.TypeString)
}
