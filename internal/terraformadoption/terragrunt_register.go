package terraformadoption

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

const (
	unitCommand        = "command"
	unitScheme         = "terragrunt://"
	unitPackage        = "oci://ghcr.io/cloudposse/atmos/plugins/terragrunt"
	unitPackageVersion = "^0.1.0"
	executableMode     = "executable"
	unitSettings       = "components.terraform.terragrunt."
)

type adoptionFile struct {
	path          string
	before, after []byte
	mode          os.FileMode
}

func openAdoptionFile(path string) (*adoptionFile, error) {
	b, mode, err := readRegular(path)
	return &adoptionFile{path: path, before: b, after: b, mode: mode}, err
}

func (f *adoptionFile) set(key, value, kind string) error {
	b, err := y.SetWithType(f.after, key, value, kind)
	if err == nil {
		f.after = b
	}
	return err
}

func (f *adoptionFile) write() error {
	if bytes.Equal(f.after, f.before) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.path), configDirMode); err != nil {
		return err
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(f.path, f.after, f.mode)
}

type unitRegistration struct {
	uri, name         string
	config            *schema.AtmosConfiguration
	project, manifest *adoptionFile
}

func (r *Registration) saveUnit(ctx context.Context, unit string) error {
	u, err := r.unitDestination(unit)
	if err != nil {
		return err
	}
	if err := u.configureProject(r); err != nil {
		return err
	}
	if err := u.addImport(r.Stack); err != nil {
		return err
	}
	mode := cmp.Or(u.config.Components.Terraform.ConfigurationProviders[tg].Mode, "native")
	if err := u.componentSettings(ctx, r, unit, mode); err != nil {
		return err
	}
	if err := u.finish(ctx, r); err != nil {
		return err
	}
	fmt.Fprintf(r.Output, "Atmos stack: %s\nComponent: %s\nUnit: %s\nTerragrunt mode: %s\nNext: atmos terraform plan %s -s %s\n", r.Stack, u.name, unit, mode, u.name, r.Stack)
	return nil
}

func (u *unitRegistration) componentSettings(ctx context.Context, r *Registration, unit, mode string) error {
	if mode == executableMode {
		saved, _ := y.Get(u.manifest.before, stack.BuildComponentYqPath(terraform, u.name, "dependencies.tools.terragrunt"))
		tool, err := unitTool(ctx, unit, r.Project, saved, u.config)
		if err != nil {
			return err
		}
		if err := u.componentValue("dependencies.tools.terragrunt", tool.version); err != nil {
			return err
		}
	}
	env := viper.New()
	env.AutomaticEnv()
	if workspace := env.GetString("TF_WORKSPACE"); workspace != "" {
		if err := u.componentValue("env.TF_WORKSPACE", workspace); err != nil {
			return err
		}
	}
	if cmd := unitOverride(r.Explicit); cmd != "" {
		return u.setEngine(ctx, filepath.Dir(unit), r.Project, cmd)
	}
	return nil
}

func (u *unitRegistration) setEngine(ctx context.Context, dir, project, command string) error {
	if err := u.componentValue(unitCommand, command); err != nil {
		return err
	}
	return u.pinEngine(ctx, dir, project, command)
}

func (u *unitRegistration) pinEngine(ctx context.Context, dir, project, command string) error {
	if command != terraform && command != tofu {
		return nil
	}
	if _, err := y.Get(u.manifest.before, stack.BuildComponentYqPath(terraform, u.name, "dependencies.tools."+command)); err == nil {
		return nil
	}
	pins, err := projectPins(evidenceDirectories(dir, project))
	if err != nil {
		return err
	}
	// Terragrunt locates its wrapped executable on PATH. A different cached
	// version must not silently turn the saved plan into a different trial.
	engine, err := resolve(ctx, dir, command, "Terragrunt engine", "")
	if err != nil {
		return err
	}
	_, version, _ := strings.Cut(versionLine(ctx, dir, engine.Path), " v")
	if !releasePin.MatchString(version) {
		return fmt.Errorf("%w: could not determine %s version", errUtils.ErrExecutableVersion, command)
	}
	if pin := strings.TrimPrefix(pins[command], "v"); pin != "" && pin != "system" && pin != version {
		return fmt.Errorf("%w: project pins %s %s but the native unit uses %s; select the pinned binary on PATH", errUtils.ErrExecutableVersion, command, pin, version)
	}
	return u.componentValue("dependencies.tools."+command, version)
}

func (u *unitRegistration) finish(ctx context.Context, r *Registration) error {
	if r.DryRun {
		for _, f := range []*adoptionFile{u.project, u.manifest} {
			rel, err := filepath.Rel(r.Project, f.path)
			if err != nil {
				return err
			}
			printChange(r.Output, filepath.ToSlash(rel), f.before, f.after)
		}
		if u.inferEngine(r) {
			fmt.Fprintln(r.Output, "On save, Atmos reads the unit's terraform_binary and pins a standard Terraform or OpenTofu executable.")
		}
		return nil
	}
	engine, err := u.validate(ctx)
	if err != nil {
		return err
	}
	if u.inferEngine(r) {
		if err := u.saveResolvedEngine(ctx, r, engine); err != nil {
			return err
		}
	}
	if err := u.project.write(); err != nil {
		return err
	}
	return u.manifest.write()
}

func (u *unitRegistration) inferEngine(r *Registration) bool {
	return u.config.Components.Terraform.ConfigurationProviders[tg].Mode == executableMode && unitOverride(r.Explicit) == ""
}

func (u *unitRegistration) saveResolvedEngine(ctx context.Context, r *Registration, declared string) error {
	saved, _ := y.Get(u.manifest.after, stack.BuildComponentYqPath(terraform, u.name, unitCommand))
	command := cmp.Or(saved, declared, defaultWrappedCommand())
	if declared == "" {
		if err := u.componentValue(unitCommand, command); err != nil {
			return err
		}
	}
	unit := filepath.Join(r.Project, filepath.FromSlash(strings.TrimPrefix(u.uri, unitScheme)))
	return u.pinEngine(ctx, filepath.Dir(unit), r.Project, command)
}

func (r *Registration) unitDestination(unit string) (*unitRegistration, error) {
	rel, err := r.unitPath(unit)
	if err != nil {
		return nil, err
	}
	config := &schema.AtmosConfiguration{BasePathAbsolute: r.Project}
	if r.Defaults != nil {
		c := *r.Defaults
		c.BasePathAbsolute = r.Project
		config = &c
	}
	stacksDir := filepath.Join(r.Project, "stacks")
	if r.Config != nil {
		c := *r.Config
		config = &c
		if !standaloneStacks(&config.Stacks) {
			return nil, fmt.Errorf("%w: stack add requires standalone filename-named stacks with included_paths [\"*.yaml\"]", errUtils.ErrInvalidStackConfig)
		}
		stacksDir = config.StacksBaseAbsolutePath
	}
	path, err := standaloneDestination(stacksDir, r.Stack)
	if err != nil {
		return nil, err
	}
	project, err := openAdoptionFile(filepath.Join(r.Project, "atmos.yaml"))
	if err != nil {
		return nil, err
	}
	manifest, err := openAdoptionFile(path)
	if err != nil {
		return nil, err
	}
	return &unitRegistration{uri: unitScheme + filepath.ToSlash(rel), name: unitName(rel), project: project, manifest: manifest, config: config}, nil
}

func (r *Registration) unitPath(unit string) (string, error) {
	dir, err := filepath.EvalSymlinks(filepath.Dir(unit))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(r.Project, filepath.Join(dir, unitFile))
	if err != nil || !filepath.IsLocal(rel) || strings.ContainsAny(filepath.ToSlash(rel), `\?#*[{`) {
		return "", fmt.Errorf("%w: unit must be a literal path inside %s", errUtils.ErrInvalidComponent, r.Project)
	}
	return rel, nil
}

func unitName(path string) string {
	if path == unitFile {
		return "root"
	}
	return filepath.Base(filepath.Dir(path))
}

// Terragrunt prefers OpenTofu on PATH when the unit declares no terraform_binary.
func defaultWrappedCommand() string {
	if _, err := exec.LookPath(tofu); err == nil {
		return tofu
	}
	return terraform
}

func (u *unitRegistration) configureProject(r *Registration) error {
	if r.Config == nil {
		if u.project.before != nil {
			return fmt.Errorf("%w: configuration appeared at %s; reload and retry", errUtils.ErrInvalidStackConfig, u.project.path)
		}
		u.project.after = []byte(starter)
		cmd := defaultWrappedCommand()
		if err := u.project.set("components.terraform.command", cmd, y.TypeString); err != nil {
			return err
		}
		u.config.Components.Terraform.Command = cmd
	}
	if err := u.enableProvider(); err != nil {
		return err
	}
	// Avoid guessing which layer owns a merged existing configuration.
	if r.Config != nil && !bytes.Equal(u.project.before, u.project.after) && u.project.before == nil {
		return fmt.Errorf("%w: enable the Terragrunt provider in the project's Atmos configuration first", errUtils.ErrInvalidStackConfig)
	}
	return nil
}

func (u *unitRegistration) enableProvider() error {
	c := u.config
	providers := maps.Clone(c.Components.Terraform.ConfigurationProviders)
	settings, exists := providers[tg]
	if exists {
		if err := checkUnitProvider(&settings); err != nil {
			return err
		}
		return u.enablePackage()
	}
	if providers == nil {
		providers = make(map[string]schema.ComponentConfiguration)
	}
	providers[tg] = schema.ComponentConfiguration{Enabled: true, Mode: executableMode}
	c.Components.Terraform.ConfigurationProviders = providers
	if err := u.project.set(unitSettings+"enabled", "true", y.TypeBool); err != nil {
		return err
	}
	if err := u.project.set(unitSettings+"mode", executableMode, y.TypeString); err != nil {
		return err
	}
	return u.enablePackage()
}

func checkUnitProvider(settings *schema.ComponentConfiguration) error {
	if !settings.Enabled || (settings.Provider != "" && settings.Provider != tg) {
		return fmt.Errorf("%w: components.terraform.terragrunt is disabled or selects another provider", errUtils.ErrPluginInvalidConfig)
	}
	return nil
}

func (u *unitRegistration) enablePackage() error {
	c := u.config
	if pkg, ok := c.Plugins.Packages[tg]; ok {
		if pkg.Enabled != nil && !*pkg.Enabled {
			return fmt.Errorf("%w: the Terragrunt plugin package is disabled", errUtils.ErrPluginInvalidConfig)
		}
		return nil
	}
	c.Plugins.Packages = maps.Clone(c.Plugins.Packages)
	if c.Plugins.Packages == nil {
		c.Plugins.Packages = make(map[string]schema.PluginPackage)
	}
	c.Plugins.Packages[tg] = schema.PluginPackage{Source: unitPackage, Version: unitPackageVersion}
	if err := u.project.set("plugins.packages.terragrunt.source", unitPackage, y.TypeString); err != nil {
		return err
	}
	return u.project.set("plugins.packages.terragrunt.version", unitPackageVersion, y.TypeString)
}

func (u *unitRegistration) addImport(stackName string) error {
	var manifest struct {
		Name       string
		Import     []string
		Components struct{ Terraform map[string]any }
		Shared     map[string]any `yaml:",inline"`
	}
	if err := yaml.Unmarshal(u.manifest.before, &manifest); err != nil {
		return err
	}
	if manifest.Name != "" && manifest.Name != stackName {
		return fmt.Errorf("%w: %s declares %s", errUtils.ErrInvalidStackConfig, u.manifest.path, manifest.Name)
	}
	if len(manifest.Shared) > 0 {
		return fmt.Errorf("%w: %s contains shared stack settings", errUtils.ErrInvalidStackConfig, u.manifest.path)
	}
	exists, err := u.hasImport(manifest.Import)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, ok := manifest.Components.Terraform[u.name]; ok {
		return fmt.Errorf("%w: component %s already exists in %s", errUtils.ErrInvalidComponent, u.name, u.manifest.path)
	}
	return u.manifest.set(fmt.Sprintf("import[%d]", len(manifest.Import)), u.uri, y.TypeString)
}

func (u *unitRegistration) hasImport(imports []string) (bool, error) {
	exists := false
	for _, uri := range imports {
		path, ok := strings.CutPrefix(uri, unitScheme)
		if !ok || filepath.Base(path) != unitFile {
			return false, fmt.Errorf("%w: cannot establish the components inherited from %s in %s", errUtils.ErrInvalidStackConfig, uri, u.manifest.path)
		}
		if uri == u.uri {
			exists = true
			continue
		}
		if unitName(path) == u.name {
			return false, fmt.Errorf("%w: %s and %s both register %s", errUtils.ErrInvalidComponent, uri, u.uri, u.name)
		}
	}
	return exists, nil
}

func (u *unitRegistration) componentValue(field, value string) error {
	path := stack.BuildComponentYqPath(terraform, u.name, field)
	if current, ok := y.GetType(u.manifest.after, path); ok {
		if current == y.TypeString {
			existing, err := y.Get(u.manifest.after, path)
			if err == nil && existing == value {
				return nil
			}
		}
		return fmt.Errorf("%w: existing %s differs from %s", errUtils.ErrInvalidStackConfig, path, value)
	}
	return u.manifest.set(path, value, y.TypeString)
}

func (u *unitRegistration) validate(ctx context.Context) (string, error) {
	if err := configuration.Attach(ctx, u.config, "stack add"); err != nil {
		return "", err
	}
	runtime, ok := u.config.ComponentConfigurations.(schema.ComponentConfigurationImportRuntime)
	if !ok {
		return "", errUtils.ErrPluginUnsupportedExtension
	}
	units, handled, err := runtime.ImportComponents(ctx, u.uri)
	if err != nil {
		return "", fmt.Errorf("import %s: %w", u.uri, err)
	}
	if _, ok := units[u.name]; !handled || !ok || len(units) != 1 {
		return "", fmt.Errorf("%w: %s did not resolve to the single unit %s", errUtils.ErrInvalidComponent, u.uri, u.name)
	}
	entry, _ := units[u.name].(map[string]any)
	command, _ := entry[unitCommand].(string)
	return command, nil
}
