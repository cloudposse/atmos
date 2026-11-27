package adoption

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

func (r *request) layout() (string, string, error) {
	componentBase := r.Project
	stacksDir := filepath.Join(r.Project, "stacks")

	if c := r.Config; c != nil {
		if !stackadoption.StandaloneStacks(&c.Stacks) {
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

type deployment struct {
	Command      string
	Dependencies struct{ Tools map[string]string }
	Metadata     struct {
		Component string
		Workspace string `yaml:"terraform_workspace"`
	}
}

func (r *request) saveTerraform(ctx context.Context) error {
	componentBase, stacksDir, err := r.layout()
	if err != nil {
		return err
	}
	relative, err := r.relativeRoot(componentBase)
	if err != nil {
		return err
	}
	file, err := stackadoption.StandaloneDestination(stacksDir, r.Stack)
	if err != nil {
		return err
	}
	original, mode, err := stackadoption.ReadRegular(file)
	if err != nil {
		return err
	}
	name := cmp.Or(r.Name, filepath.Base(r.Dir))
	dest := y.ComponentPath(terraform, name, "")
	entry, exists, err := r.manifestEntry(original, file, name)
	if err != nil {
		return err
	}
	target, err := r.resolveTarget(ctx, entry, relative)
	if err != nil {
		return err
	}
	data, err := target.updateEntry(original, dest, entry, exists)
	if err != nil {
		return fmt.Errorf("component %s in %s: %w", name, file, err)
	}
	data, err = r.AddVarFiles(data, stackadoption.VarFileTarget{Dest: dest, File: file, Dir: r.Dir, Extensions: []string{".tfvars", ".tfvars.json"}}, exists)
	if err != nil {
		return err
	}
	data, err = r.addAtlantisSource(data, dest, exists)
	if err != nil {
		return err
	}
	fmt.Fprintf(r.Output, "Atmos stack: %s\nComponent: %s\nEngine: %s (%s)\nWorkspace: %s\nManifest: %s\n", r.Stack, name, target.engine.Command, target.engine.Reason, target.workspace, file)
	if err := r.WriteManifest(file, original, data, mode); err != nil {
		return err
	}
	if r.DryRun {
		r.Name, r.Explicit = name, target.engine.Command
		if r.atlantisSource == "" {
			r.Workspace = target.workspace
		} else {
			r.VarFiles = nil // The selected Atlantis project supplies the ordered inputs.
		}
		r.PreviewSave()
	} else {
		fmt.Fprintln(r.Output, "Next:", shellescape.QuoteCommand([]string{"atmos", "terraform", "plan", name, "-s", r.Stack}))
	}
	return nil
}

func (r *request) resolveTarget(ctx context.Context, entry deployment, relative string) (deploymentTarget, error) {
	engine, err := Select(ctx, r.Dir, r.Project, Selection{Explicit: r.Explicit, Configured: r.configuredCommand(entry), Interactive: r.Interactive})
	if err != nil {
		return deploymentTarget{}, err
	}
	workspace, err := r.registrationWorkspace(ctx, engine, entry, relative)
	if err != nil {
		return deploymentTarget{}, err
	}
	if err := r.checkAtlantisTarget(ctx, &engine, workspace); err != nil {
		return deploymentTarget{}, err
	}
	return deploymentTarget{relative, workspace, engine}, nil
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

func verifiedWorkspace(dir string, layout backend.LocalState, selected string) (string, error) {
	if selected == "" {
		var err error
		selected, err = nativeWorkspace(dir)
		if err != nil {
			return "", err
		}
	}
	if selected == "default" {
		return selected, nil
	}
	if !filepath.IsLocal(selected) || selected == "." || filepath.Base(selected) != selected {
		return "", fmt.Errorf("%w: invalid workspace %q", errUtils.ErrInvalidStackConfig, selected)
	}
	info, err := os.Stat(filepath.Join(layout.Workspaces, selected))
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: workspace %q does not exist in %s", errUtils.ErrInvalidStackConfig, selected, layout.Workspaces)
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

func (r *request) relativeRoot(componentBase string) (string, error) {
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

type adoptionManifest struct {
	Name       string
	Import     []any
	Shared     map[string]any `yaml:",inline"`
	Components struct {
		Terraform map[string]deployment
	}
}

func (r *request) manifestEntry(original []byte, file, name string) (deployment, bool, error) {
	var manifest adoptionManifest
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

func (r *request) configuredCommand(entry deployment) string {
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
