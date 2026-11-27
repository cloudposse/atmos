package terraformadoption

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

const (
	configFileMode = 0o644
	configDirMode  = 0o755
	lockDirMode    = 0o700
)

const starter = `base_path: .
components:
  terraform:
    base_path: .
    auto_generate_backend_file: false
stacks:
  base_path: stacks
  included_paths: ["*.yaml"]
`

var stackNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Registration describes one component addition to a standalone Terraform stack.
type Registration struct {
	Project, Dir, Stack, Explicit string
	Config                        *schema.AtmosConfiguration // Nil for an unconfigured project.
	Defaults                      *schema.AtmosConfiguration // CLI defaults, including plugin policy in a fresh project.
	Interactive, DryRun           bool
	Output                        io.Writer
}

// Save previews or atomically updates the selected manifest under a project lock.
func (r *Registration) Save(ctx context.Context) error {
	defer perf.Track(r.Config, "terraformadoption.Save")()

	if !stackNamePattern.MatchString(r.Stack) {
		return fmt.Errorf("%w: invalid stack name %q", errUtils.ErrInvalidStackConfig, r.Stack)
	}
	// Use one lock identity even when the checkout is reached through a symlink.
	project, err := filepath.EvalSymlinks(r.Project)
	if err != nil {
		return err
	}
	request := *r
	request.Project = project
	if r.DryRun {
		return request.save(ctx)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	locks := filepath.Join(cache, "atmos", "locks")
	if err := os.MkdirAll(locks, lockDirMode); err != nil {
		return err
	}
	lock := filelock.New(filepath.Join(locks, fmt.Sprintf("stack-add-%x.lock", sha256.Sum256([]byte(project)))))
	return lock.WithExclusive(ctx, func() error { return request.save(ctx) })
}

func (r *Registration) layout() (string, string, error) {
	componentBase := r.Project
	stacksDir := filepath.Join(r.Project, "stacks")

	if c := r.Config; c != nil {
		if !standaloneStacks(&c.Stacks) {
			return "", "", fmt.Errorf("%w: stack add requires filename-named stacks with included_paths [\"*.yaml\"]", errUtils.ErrInvalidStackConfig)
		}
		if c.Components.Terraform.AutoGenerateBackendFile {
			return "", "", fmt.Errorf("%w: stack add requires components.terraform.auto_generate_backend_file: false to preserve the adopted backend", errUtils.ErrInvalidStackConfig)
		}
		componentBase = c.TerraformDirAbsolutePath
		stacksDir = c.StacksBaseAbsolutePath
	}
	return componentBase, stacksDir, nil
}

func standaloneStacks(stacks *schema.Stacks) bool {
	return stacks.NameTemplate == "" && stacks.NamePattern == "" && slices.Equal(stacks.IncludedPaths, []string{"*.yaml"}) && len(stacks.ExcludedPaths) == 0
}

type deployment struct {
	Command      string
	Dependencies struct{ Tools map[string]string }
	Metadata     struct {
		Component string
		Workspace string `yaml:"terraform_workspace"`
	}
}

func (r *Registration) save(ctx context.Context) error {
	unit, err := TerragruntUnit(r.Dir)
	if err != nil {
		return err
	}
	if unit != "" {
		return r.saveUnit(ctx, unit)
	}
	return r.saveTerraform(ctx)
}

func (r *Registration) saveTerraform(ctx context.Context) error {
	componentBase, stacksDir, err := r.layout()
	if err != nil {
		return err
	}
	relative, err := r.relativeRoot(componentBase)
	if err != nil {
		return err
	}
	file, err := standaloneDestination(stacksDir, r.Stack)
	if err != nil {
		return err
	}
	original, mode, err := readRegular(file)
	if err != nil {
		return err
	}
	name := filepath.Base(r.Dir)
	dest := stack.BuildComponentYqPath(terraform, name, "")
	entry, exists, err := r.manifestEntry(original, file, name)
	if err != nil {
		return err
	}
	engine, err := Select(ctx, r.Dir, r.Project, Selection{Explicit: r.Explicit, Configured: r.configuredCommand(entry), Interactive: r.Interactive})
	if err != nil {
		return err
	}
	workspace, err := verifiedWorkspace(ctx, r.Dir, engine)
	if err != nil {
		return err
	}
	target := deploymentTarget{relative, workspace, engine}
	data, err := target.updateEntry(original, dest, entry, exists)
	if err != nil {
		return fmt.Errorf("component %s in %s: %w", name, file, err)
	}
	fmt.Fprintf(r.Output, "Atmos stack: %s\nComponent: %s\nEngine: %s (%s)\nWorkspace: %s\nManifest: %s\n", r.Stack, name, engine.Command, engine.Reason, workspace, file)
	fmt.Fprintf(r.Output, "Next: atmos terraform plan %s -s %s\n", name, r.Stack)
	return r.writeManifest(file, original, data, mode)
}

type deploymentTarget struct {
	relative, workspace string
	engine              Engine
}

func (d *deploymentTarget) updateEntry(original []byte, dest string, entry deployment, exists bool) ([]byte, error) {
	relative, workspace, engine := d.relative, d.workspace, d.engine
	data := original
	var err error
	if exists {
		if d.conflicts(entry) {
			return nil, fmt.Errorf("%w: already exists with different configuration", errUtils.ErrInvalidComponent)
		}
		return original, nil
	}
	for _, field := range []struct{ key, value string }{{"metadata.component", relative}, {"metadata.terraform_workspace", workspace}, {"command", engine.Command}} {
		data, err = y.SetWithType(data, dest+"."+field.key, field.value, "string")
		if err != nil {
			return nil, err
		}
	}
	if engine.Pin != "" {
		data, err = y.SetWithType(data, dest+".dependencies.tools."+engine.Command, engine.Pin, "string")
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func printChange(out io.Writer, file string, before, after []byte) {
	edits := myers.ComputeEdits(span.URIFromPath(file), string(before), string(after))
	if len(edits) > 0 {
		fmt.Fprint(out, gotextdiff.ToUnified("a/"+file, "b/"+file, string(before), edits))
	}
}

func writeStarter(project string) error {
	fs := filesystem.NewOSFileSystem()

	configFile := filepath.Join(project, "atmos.yaml")
	current, _, err := readRegular(configFile)
	if err != nil {
		return err
	}
	if current != nil && string(current) != starter {
		return fmt.Errorf("%w: configuration already exists at %s; reload and retry", errUtils.ErrInvalidStackConfig, configFile)
	}
	if current == nil {
		if err := fs.WriteFileAtomic(configFile, []byte(starter), configFileMode); err != nil {
			return err
		}
	}
	return nil
}

// A standalone manifest may explicitly name a different stack from its filename.
func standaloneDestination(dir, stackName string) (string, error) {
	target := filepath.Join(dir, stackName+".yaml")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return target, nil
	}
	if err != nil {
		return "", err
	}
	var match string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		name, err := manifestName(path, entry.Name())
		if err != nil {
			return "", err
		}
		if name == stackName {
			if match != "" {
				return "", fmt.Errorf("%w: stack %s is declared in both %s and %s", errUtils.ErrInvalidStackConfig, stackName, match, path)
			}
			match = path
		}
	}
	if match != "" {
		return match, nil
	}
	return target, nil
}

func readRegular(path string) ([]byte, os.FileMode, error) {
	stat, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, configFileMode, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !stat.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%w: expected a regular file: %s", errUtils.ErrInvalidStackConfig, path)
	}
	data, err := os.ReadFile(path)
	return data, stat.Mode().Perm(), err
}

func verifiedWorkspace(ctx context.Context, dir string, engine Engine) (string, error) {
	openTofu, err := usesTofu(ctx, dir, engine)
	if err != nil {
		return "", err
	}
	workspaces, err := backend.LocalWorkspaceDirectory(dir, openTofu)
	if err != nil {
		return "", err
	}
	selected, err := nativeWorkspace(dir)
	if err != nil || selected == "default" {
		return selected, err
	}
	if !filepath.IsLocal(selected) || selected == "." || filepath.Base(selected) != selected {
		return "", fmt.Errorf("%w: invalid workspace %q", errUtils.ErrInvalidStackConfig, selected)
	}
	info, err := os.Stat(filepath.Join(workspaces, selected))
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: workspace %q does not exist in %s", errUtils.ErrInvalidStackConfig, selected, workspaces)
	}
	return selected, nil
}

func nativeWorkspace(dir string) (string, error) {
	// TF_WORKSPACE is inherited by the native Terraform process.
	//nolint:forbidigo // Read the native workspace override exactly as the selected executable does.
	if w := os.Getenv("TF_WORKSPACE"); w != "" {
		return w, nil
	}
	b, err := os.ReadFile(filepath.Join(autoinit.DataDir(dir, nil), "environment"))
	if os.IsNotExist(err) {
		return "default", nil
	}
	if err != nil {
		return "", err
	}
	if w := strings.TrimSpace(string(b)); w != "" {
		return w, nil
	}
	return "", fmt.Errorf("%w: empty native workspace selection", errUtils.ErrInvalidConfig)
}

func (r *Registration) relativeRoot(componentBase string) (string, error) {
	// Config and the native working directory can spell the same checkout differently
	// (for example /var and /private/var on macOS).
	componentBase, err := filepath.EvalSymlinks(componentBase)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(r.Dir)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(componentBase, root)
	if err != nil || (relative != "." && !filepath.IsLocal(relative)) {
		return "", fmt.Errorf("%w: root %s is outside the Terraform component base %s", errUtils.ErrInvalidStackConfig, r.Dir, componentBase)
	}
	return filepath.ToSlash(relative), nil
}

func (r *Registration) manifestEntry(original []byte, file, name string) (deployment, bool, error) {
	var manifest struct {
		Name       string
		Import     []any
		Shared     map[string]any `yaml:",inline"`
		Components struct {
			Terraform map[string]deployment
		}
	}
	if err := yaml.Unmarshal(original, &manifest); err != nil {
		return deployment{}, false, err
	}
	if (manifest.Name != "" && manifest.Name != r.Stack) || len(manifest.Import) > 0 {
		return deployment{}, false, fmt.Errorf("%w: %s is not a standalone manifest for stack %s", errUtils.ErrInvalidStackConfig, file, r.Stack)
	}
	if len(manifest.Shared) > 0 {
		return deployment{}, false, fmt.Errorf("%w: %s contains shared stack settings; register in a standalone stack to preserve native inputs", errUtils.ErrInvalidStackConfig, file)
	}
	entry, exists := manifest.Components.Terraform[name]
	return entry, exists, nil
}

func (r *Registration) writeManifest(file string, original, data []byte, mode os.FileMode) error {
	stacksDir := filepath.Dir(file)
	if r.DryRun {
		fmt.Fprintln(r.Output, "Preview only.")
		if r.Config == nil {
			printChange(r.Output, "atmos.yaml", nil, []byte(starter))
		}
		path, err := filepath.Rel(r.Project, file)
		if err != nil {
			return err
		}
		printChange(r.Output, filepath.ToSlash(path), original, data)
		return nil
	}
	if r.Config == nil {
		if err := writeStarter(r.Project); err != nil {
			return err
		}
	}
	if bytes.Equal(data, original) {
		fmt.Fprintln(r.Output, "Already configured.")
		return nil
	}
	if err := os.MkdirAll(stacksDir, configDirMode); err != nil {
		return err
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(file, data, mode)
}

func manifestName(path, filename string) (string, error) {
	data, _, err := readRegular(path)
	if err != nil {
		return "", err
	}
	var header struct{ Name string }
	if err := yaml.Unmarshal(data, &header); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	name := header.Name
	if name == "" {
		name = strings.TrimSuffix(filename, ".yaml")
	}
	return name, nil
}

func usesTofu(ctx context.Context, dir string, engine Engine) (bool, error) {
	if engine.Command == terraform || engine.Command == tofu {
		return engine.Command == tofu, nil
	}
	_, hasTofu, err := sourceFiles(dir)
	if err != nil || !hasTofu {
		return false, err
	}
	version := versionLine(ctx, dir, engine.Path)
	if !strings.HasPrefix(version, "Terraform v") && !strings.HasPrefix(version, "OpenTofu v") {
		return false, fmt.Errorf("%w: cannot establish which source files the wrapper reads", errUtils.ErrInvalidToolSpec)
	}
	return strings.HasPrefix(version, "OpenTofu v"), nil
}

func (r *Registration) configuredCommand(entry deployment) string {
	var configured string
	if r.Config != nil && r.Config.Components.Terraform.Command != "" {
		configured = r.Config.Components.Terraform.Command
	}
	if entry.Command != "" {
		configured = entry.Command
	}
	return configured
}

func (d *deploymentTarget) conflicts(entry deployment) bool {
	relative, workspace, engine := d.relative, d.workspace, d.engine
	return entry.Metadata.Component != relative || entry.Command != engine.Command || entry.Metadata.Workspace != workspace || (engine.Pin != "" && entry.Dependencies.Tools[engine.Command] != engine.Pin)
}
