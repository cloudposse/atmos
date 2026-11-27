package terraformadoption

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

func selectedUnit(t *testing.T) (Registration, string) {
	t.Helper()
	r := registration(t)
	r.Project, _ = filepath.EvalSymlinks(r.Project)
	r.Dir = filepath.Join(r.Project, "infra", "net")
	unit := filepath.Join(r.Dir, unitFile)
	write(t, unit, "invalid HCL is sufficient for marker detection and preview")
	binaries(t, tg, tofu)
	t.Setenv("TG_TF_PATH", "")
	t.Setenv("TERRAGRUNT_TFPATH", "")
	t.Setenv("ATMOS_TOOLCHAIN_INSTALL_PATH", t.TempDir())
	return r, unit
}

func TestTerragruntMarkerAndNativeCommand(t *testing.T) {
	r, unit := selectedUnit(t)
	for _, path := range []string{unit, r.Dir} {
		got, err := TerragruntUnit(path)
		require.NoError(t, err)
		require.Equal(t, unit, got)
	}
	for _, cache := range []string{".terragrunt-stack", ".terragrunt-cache"} {
		_, err := TerragruntUnit(filepath.Join(r.Dir, cache, unitFile))
		require.ErrorContains(t, err, cache)
	}
	got, err := TerragruntUnit(r.Project)
	require.NoError(t, err)
	require.Empty(t, got)
	write(t, filepath.Join(r.Dir, stackFile), "")
	for _, path := range []string{r.Dir, filepath.Join(r.Dir, stackFile)} {
		_, err = TerragruntUnit(path)
		require.ErrorContains(t, err, "select a single unit")
	}
	require.ErrorContains(t, r.Save(t.Context()), "select a single unit")
	require.NoError(t, os.Mkdir(filepath.Join(r.Project, unitFile), 0o700))
	_, err = TerragruntUnit(r.Project)
	require.ErrorContains(t, err, "regular unit file")

	t.Setenv("TF_WORKSPACE", "blue")
	t.Setenv("TG_TF_PATH", "/original")
	for _, explicit := range []string{"", "/override"} {
		cmd, err := TerragruntPlan(t.Context(), unit, r.Project, explicit, []string{"-out=dev plan"})
		require.NoError(t, err)
		require.Equal(t, r.Dir, cmd.WorkingDir)
		require.Equal(t, []string{"--working-dir", r.Dir, "run", "--", "plan", "-out=dev plan"}, cmd.Args)
		require.Contains(t, cmd.Env, "TF_WORKSPACE=blue")
		require.Contains(t, cmd.Env, "TG_TF_PATH="+unitOverride(explicit))
	}
}

func TestTerragruntToolPins(t *testing.T) {
	r, unit := selectedUnit(t)
	versions := filepath.Join(r.Dir, ".tool-versions")
	for _, tc := range []struct{ manifest, want, failure string }{
		{"terragrunt 1.2.1", "1.2.1", ""},
		{"gruntwork-io/terragrunt v1.2.1", "1.2.1", ""},
		{"terragrunt system", "", ""},
		{"terragrunt latest", "", "exact terragrunt pin"},
		{"terragrunt 1.0.0\ngruntwork-io/terragrunt 1.2.1", "", "conflicting"},
		{"terragrunt", "", "missing version"},
	} {
		write(t, versions, tc.manifest)
		got, err := unitPin([]string{r.Dir, r.Project})
		if tc.failure != "" {
			require.ErrorContains(t, err, tc.failure)
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		}
	}
	_, err := TerragruntPlan(t.Context(), unit, r.Project, "", nil)
	require.Error(t, err)
	write(t, versions, "terragrunt 1.2.0")
	_, err = unitTool(t.Context(), unit, r.Project, "", nil)
	require.ErrorContains(t, err, "reports")
	require.NoError(t, os.Remove(versions))
	_, err = unitVersion("terragrunt version v0.99.1", "")
	require.ErrorContains(t, err, "or newer")
	_, err = unitVersion("unknown", "")
	require.ErrorContains(t, err, "or newer")
	install := t.TempDir()
	t.Setenv("ATMOS_TOOLCHAIN_INSTALL_PATH", install)
	name := tg
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cached := filepath.Join(install, "bin", "gruntwork-io", tg, "1.2.1", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(cached), 0o700))
	require.NoError(t, os.WriteFile(cached, versionProbe(t), 0o700))
	t.Setenv("PATH", t.TempDir())
	c := &schema.AtmosConfiguration{}
	c.Toolchain.InstallPath = install
	tool, err := unitTool(t.Context(), unit, r.Project, "1.2.1", c)
	require.NoError(t, err)
	require.Equal(t, cached, tool.path)
	_, err = unitTool(t.Context(), unit, r.Project, "", nil)
	require.ErrorContains(t, err, "needs terragrunt")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = unitTool(ctx, unit, r.Project, "1.2.1", c)
	require.ErrorContains(t, err, "--version")
}

func TestTerragruntPreviewAndRetry(t *testing.T) {
	r, unit := selectedUnit(t)
	r.Defaults = &schema.AtmosConfiguration{}
	r.DryRun = true
	r.Explicit = tofu
	t.Setenv("TF_WORKSPACE", "blue")
	var out bytes.Buffer
	r.Output = &out
	require.NoError(t, r.Save(t.Context()))
	for _, text := range []string{"+++ b/atmos.yaml", "+++ b/stacks/dev.yaml", "mode: executable", unitPackage, "terragrunt://infra/net/terragrunt.hcl", "TF_WORKSPACE: blue", "terragrunt: 1.2.1", "tofu: 8.7.6", "atmos terraform plan net -s dev"} {
		require.Contains(t, out.String(), text)
	}
	require.NoFileExists(t, filepath.Join(r.Project, "atmos.yaml"))
	require.NoFileExists(t, filepath.Join(r.Project, ".atmos.plugins.lock.yaml"))
	require.NoDirExists(t, filepath.Join(r.Project, "stacks"))

	u, err := r.unitDestination(unit)
	require.NoError(t, err)
	require.NoError(t, u.configureProject(&r))
	require.NoError(t, u.addImport(r.Stack))
	require.NoError(t, u.componentSettings(t.Context(), &r, unit, executableMode))
	require.NoError(t, u.project.write())
	require.NoError(t, u.manifest.write())
	c := u.config
	c.Stacks.IncludedPaths = []string{"*.yaml"}
	c.StacksBaseAbsolutePath = filepath.Dir(u.manifest.path)
	r.Config = c
	out.Reset()
	require.NoError(t, r.Save(t.Context()))
	require.NotContains(t, out.String(), "+++")
	out.Reset()
	r.Dir = filepath.Join(r.Project, "live", "dev", "database")
	write(t, filepath.Join(r.Dir, unitFile), "")
	require.NoError(t, r.Save(t.Context()))
	require.Contains(t, out.String(), "terragrunt://live/dev/database/terragrunt.hcl")
	r.Dir = filepath.Join(r.Project, "live", "prod", "net")
	write(t, filepath.Join(r.Dir, unitFile), "")
	require.ErrorContains(t, r.Save(t.Context()), "both register net")
	r.Stack = "prod"
	require.NoError(t, r.Save(t.Context()))
	require.Contains(t, out.String(), "+++ b/stacks/prod.yaml")
	r.Dir = r.Project
	write(t, filepath.Join(r.Dir, unitFile), "")
	require.NoError(t, r.Save(t.Context()))
	require.Contains(t, out.String(), "Component: root")
}

func TestTerragruntDestinationConflicts(t *testing.T) {
	r, unit := selectedUnit(t)
	r.DryRun = true
	stackDir := filepath.Join(r.Project, "stacks")
	file := filepath.Join(stackDir, "dev.yaml")
	for _, body := range []string{"name: other", "components: [", "vars: {region: west}", "import: [catalog/common]", "components: {terraform: {net: {}}}", "import: [terragrunt://other/net/terragrunt.hcl]"} {
		write(t, file, body)
		require.Error(t, r.Save(t.Context()), body)
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		require.Equal(t, body, string(data))
	}
	write(t, file, "# retain me\ncomponents: {terraform: {other: {}}}\n")
	u, err := r.unitDestination(unit)
	require.NoError(t, err)
	require.NoError(t, u.addImport(r.Stack))
	require.Contains(t, string(u.manifest.after), "# retain me")
	_, err = r.unitDestination(filepath.Join(t.TempDir(), unitFile))
	require.ErrorContains(t, err, "inside")
	_, err = r.unitDestination(filepath.Join(t.TempDir(), "missing", unitFile))
	require.Error(t, err)
	p := filepath.Join(r.Project, "atmos.yaml")
	write(t, p, "# existing")
	require.ErrorContains(t, r.Save(t.Context()), "reload and retry")
	r.Config = &schema.AtmosConfiguration{}
	require.ErrorContains(t, r.Save(t.Context()), "filename-named")
	r.Config.Stacks.IncludedPaths = []string{"*.yaml"}
	r.Config.StacksBaseAbsolutePath = stackDir
	require.NoError(t, os.Remove(p))
	require.ErrorContains(t, r.Save(t.Context()), "enable the Terragrunt provider")
	require.NoError(t, os.Mkdir(p, 0o700))
	require.ErrorContains(t, r.Save(t.Context()), "regular file")
	require.NoError(t, os.Remove(p))
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Mkdir(file, 0o700))
	require.ErrorContains(t, r.Save(t.Context()), "regular file")
	r.Config.StacksBaseAbsolutePath = filepath.Join(r.Project, "not-directory")
	write(t, r.Config.StacksBaseAbsolutePath, "")
	require.Error(t, r.Save(t.Context()))
}

func TestTerragruntExistingProviderAndSettings(t *testing.T) {
	r, unit := selectedUnit(t)
	u, err := r.unitDestination(unit)
	require.NoError(t, err)
	settings := map[string]schema.ComponentConfiguration{tg: {Enabled: true, Mode: "native"}}
	u.config.Components.Terraform.ConfigurationProviders = settings
	enabled := true
	u.config.Plugins.Packages = map[string]schema.PluginPackage{tg: {Path: "../package", Enabled: &enabled}}
	require.NoError(t, u.enableProvider())
	require.Empty(t, u.project.after)
	require.Equal(t, "native", u.config.Components.Terraform.ConfigurationProviders[tg].Mode)
	enabled = false
	require.ErrorContains(t, u.enableProvider(), "package is disabled")
	settings[tg] = schema.ComponentConfiguration{Enabled: true, Provider: "alternate"}
	u.config.Components.Terraform.ConfigurationProviders = settings
	require.ErrorContains(t, u.enableProvider(), "another provider")
	require.NoError(t, checkUnitProvider(&schema.ComponentConfiguration{Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}))
	require.NoError(t, u.componentValue("command", "./wrapper"))
	require.NoError(t, u.setEngine(t.Context(), r.Dir, r.Project, "./wrapper"))
	require.ErrorContains(t, u.setEngine(t.Context(), r.Dir, r.Project, tofu), "differs")
	u.manifest.after = nil
	require.NoError(t, u.componentValue("env.TF_WORKSPACE", "original"))
	t.Setenv("TF_WORKSPACE", "changed")
	require.ErrorContains(t, u.componentSettings(t.Context(), &r, unit, "native"), "differs")
	t.Setenv("TF_WORKSPACE", "")
	t.Setenv("PATH", t.TempDir())
	require.ErrorContains(t, u.componentSettings(t.Context(), &r, unit, executableMode), "needs terragrunt")
	require.ErrorContains(t, u.setEngine(t.Context(), r.Dir, r.Project, tofu), "unavailable")
	write(t, filepath.Join(r.Dir, ".tool-versions"), "tofu")
	require.ErrorContains(t, u.setEngine(t.Context(), r.Dir, r.Project, tofu), "missing version")
	require.Error(t, u.project.set("[", "", y.TypeString))
	u.project.path = filepath.Join(unit, "sub", "atmos.yaml")
	u.project.after = []byte("change")
	require.Error(t, u.project.write())
}

type fixtureUnitImport struct {
	schema.ComponentConfigurationRuntime
	units   map[string]any
	handled bool
	err     error
}

func (f fixtureUnitImport) ImportComponents(context.Context, string) (map[string]any, bool, error) {
	return f.units, f.handled, f.err
}

func TestTerragruntImportValidation(t *testing.T) {
	r, unit := selectedUnit(t)
	u, err := r.unitDestination(unit)
	require.NoError(t, err)
	_, err = u.validate(t.Context())
	require.Error(t, err)
	for _, f := range []fixtureUnitImport{{}, {handled: true, units: map[string]any{"other": nil}}, {handled: true, units: map[string]any{"net": nil, "other": nil}}, {err: errors.New("unresolved HCL")}} {
		u.config.ComponentConfigurations = f
		require.Error(t, u.finish(t.Context(), &r))
		require.NoFileExists(t, u.manifest.path)
	}
	u.config.ComponentConfigurations = fixtureUnitImport{handled: true, units: map[string]any{"net": struct{}{}}}
	u.project.after = []byte(starter)
	require.NoError(t, u.addImport("dev"))
	require.NoError(t, u.finish(t.Context(), &r))
	require.FileExists(t, u.manifest.path)
	u.project.path = filepath.Join(unit, "child", "atmos.yaml")
	require.Error(t, u.finish(t.Context(), &r))
}

func TestTerragruntResolvedEngine(t *testing.T) {
	r, unit := selectedUnit(t)
	for _, tc := range []struct{ name, declared, command, pin string }{
		{"Terragrunt default", "", "tofu", "8.7.6"},
		{"HCL selects OpenTofu", "tofu", "", "8.7.6"},
		{"HCL selects wrapper", "./wrapper", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := r.unitDestination(unit)
			require.NoError(t, err)
			require.NoError(t, u.saveResolvedEngine(t.Context(), &r, tc.declared))
			entry := "components.terraform.net."
			cmd, _ := y.Get(u.manifest.after, entry+"command")
			require.Equal(t, tc.command, cmd)
			pin, _ := y.Get(u.manifest.after, entry+"dependencies.tools.tofu")
			require.Equal(t, tc.pin, pin)
			u.manifest.before = u.manifest.after
			require.NoError(t, u.saveResolvedEngine(t.Context(), &r, tc.declared))
			require.Equal(t, u.manifest.before, u.manifest.after)
		})
	}
	mismatch, err := r.unitDestination(unit)
	require.NoError(t, err)
	write(t, filepath.Join(r.Dir, ".tool-versions"), "opentofu 8.7.5")
	require.ErrorContains(t, mismatch.saveResolvedEngine(t.Context(), &r, "tofu"), "native unit uses 8.7.6")

	t.Setenv("PATH", t.TempDir())
	require.Equal(t, terraform, defaultWrappedCommand())
	r.DryRun = false
	require.ErrorContains(t, r.Save(t.Context()), "needs terragrunt")
}
