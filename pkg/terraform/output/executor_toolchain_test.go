package output

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// outputToolchain is a pre-seeded toolchain install: no tool is downloaded, and the install
// directories are absent from the ambient PATH.
type outputToolchain struct {
	config  *schema.AtmosConfiguration
	tools   map[string]string
	ambient string
}

func newOutputToolchain(t *testing.T, versions ...string) outputToolchain {
	t.Helper()
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	ambient := t.TempDir()
	t.Setenv("PATH", ambient)
	t.Setenv("CUSTOM_VALUE", "ambient")
	t.Setenv("AWS_PROFILE", "ambient-profile")

	config := validAtmosConfig(t)
	config.BasePathAbsolute = config.BasePath
	config.Toolchain = schema.Toolchain{
		Install: schema.ToolchainInstallNever, InstallPath: filepath.Join(config.BasePath, "tools"),
		Aliases: map[string]string{"tofu": "opentofu/opentofu", "fixture-tool": "acme/fixture-tool"},
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	tools := map[string]string{}
	for _, version := range versions {
		for name, owner := range map[string]string{"tofu": "opentofu/opentofu", "fixture-tool": "acme/fixture-tool"} {
			file := name
			if runtime.GOOS == "windows" {
				file += ".exe"
			}
			target := filepath.Join(config.Toolchain.InstallPath, "bin", filepath.FromSlash(owner), version, file)
			require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
			require.NoError(t, os.WriteFile(target, content, 0o755))
			tools[name+"@"+version] = target
		}
	}
	return outputToolchain{config: config, tools: tools, ambient: ambient}
}

// outputRun is what one dependency output read handed to Terraform.
type outputRun struct {
	executable string
	env        map[string]string
	outputs    map[string]any
}

func (o outputToolchain) read(t *testing.T, version string, mutate func(sections map[string]any), auth *schema.AuthContext) outputRun {
	t.Helper()
	ctrl := gomock.NewController(t)
	runner := NewMockTerraformRunner(ctrl)
	run := outputRun{}
	runner.EXPECT().SetEnv(gomock.Any()).DoAndReturn(func(env map[string]string) error { run.env = env; return nil }).AnyTimes()
	runner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	runner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"id": {Value: []byte(`"value"`)}}, nil)
	executor := NewExecutor(NewMockComponentDescriber(ctrl), WithRunnerFactory(func(_, executable string) (TerraformRunner, error) {
		run.executable = executable
		return runner, nil
	}))
	sections := validSections()
	sections[cfg.CommandSectionName] = "tofu"
	sections["dependencies"] = map[string]any{"tools": map[string]any{"tofu": version, "fixture-tool": version}}
	if mutate != nil {
		mutate(sections)
	}
	outputs, err := executor.ExecuteWithSections(o.config, "test-component", "test-stack", sections, auth)
	require.NoError(t, err)
	run.outputs = outputs
	return run
}

// hasFixtureTool reports whether the fixture tool executable is on the given PATH value.
func hasFixtureTool(path string) (string, bool) {
	name := "fixture-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// A dependency output read (the !terraform.output and atmos.Component path) runs the executable
// pinned through dependencies.tools, and a tool declared there is on the PATH Terraform sees
// even though neither is on the ambient PATH. Component env wins over ambient env.
func TestDependencyOutputReadUsesPinnedExecutableAndToolchainPath(t *testing.T) {
	setup := newOutputToolchain(t, "1.0.0")
	run := setup.read(t, "1.0.0", func(sections map[string]any) {
		sections[cfg.EnvSectionName] = map[string]any{"CUSTOM_VALUE": "from-stack"}
	}, nil)

	require.Equal(t, setup.tools["tofu@1.0.0"], run.executable, "the pinned toolchain executable, not the bare name")
	found, ok := hasFixtureTool(run.env["PATH"])
	require.True(t, ok, "dependencies.tools executables must be on the Terraform PATH: %s", run.env["PATH"])
	require.Equal(t, setup.tools["fixture-tool@1.0.0"], found)
	_, ambientHasTool := hasFixtureTool(os.Getenv("PATH"))
	require.False(t, ambientHasTool)
	require.Equal(t, "from-stack", run.env["CUSTOM_VALUE"], "component env beats ambient env")
	require.Equal(t, "value", run.outputs["id"])
}

// A PATH set by the component survives behind the toolchain directories.
func TestDependencyOutputReadKeepsComponentPath(t *testing.T) {
	setup := newOutputToolchain(t, "1.0.0")
	custom := filepath.Join(t.TempDir(), "custom-bin")
	run := setup.read(t, "1.0.0", func(sections map[string]any) {
		sections[cfg.EnvSectionName] = map[string]any{"PATH": custom}
	}, nil)

	entries := filepath.SplitList(run.env["PATH"])
	require.Contains(t, entries, custom)
	_, ok := hasFixtureTool(run.env["PATH"])
	require.True(t, ok)
	// Toolchain directories (their relative order is unspecified) precede the component PATH.
	require.ElementsMatch(t, []string{filepath.Dir(setup.tools["tofu@1.0.0"]), filepath.Dir(setup.tools["fixture-tool@1.0.0"])}, entries[:2])
	require.Equal(t, custom, entries[2])
}

// The authenticated identity's profile replaces an ambient profile, and two consecutive reads with
// different identities and tool versions never share PATH entries, executables, or credentials.
func TestDependencyOutputReadIsolatesIdentitiesAndVersions(t *testing.T) {
	setup := newOutputToolchain(t, "1.0.0", "2.0.0")
	credentials, config := filepath.Join(t.TempDir(), "credentials"), filepath.Join(t.TempDir(), "config")
	identity := func(profile string) *schema.AuthContext {
		return &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: profile, CredentialsFile: credentials, ConfigFile: config, Region: "us-east-2"}}
	}
	first := setup.read(t, "1.0.0", nil, identity("development"))
	second := setup.read(t, "2.0.0", nil, identity("production"))

	require.Equal(t, "development", first.env["AWS_PROFILE"])
	require.Equal(t, "production", second.env["AWS_PROFILE"])
	require.Equal(t, setup.tools["tofu@1.0.0"], first.executable)
	require.Equal(t, setup.tools["tofu@2.0.0"], second.executable)
	for version, run := range map[string]outputRun{"1.0.0": first, "2.0.0": second} {
		found, ok := hasFixtureTool(run.env["PATH"])
		require.True(t, ok, version)
		require.Equal(t, setup.tools["fixture-tool@"+version], found)
		for _, entry := range filepath.SplitList(run.env["PATH"]) {
			other := "1.0.0"
			if version == "1.0.0" {
				other = "2.0.0"
			}
			require.False(t, strings.Contains(entry, string(filepath.Separator)+other), "%s run must not see the %s toolchain: %s", version, other, entry)
		}
	}
	// The ambient process environment is untouched.
	require.Equal(t, "ambient-profile", os.Getenv("AWS_PROFILE"))
	require.Equal(t, setup.ambient, os.Getenv("PATH"))
}

// A plugin-owned component's output read starts from the identity's sanitized environment, never
// the ambient one, and still receives the pinned executable and the toolchain directories.
func TestPluginOwnedOutputReadUsesIdentityEnvironmentAndToolchain(t *testing.T) {
	setup := newOutputToolchain(t, "1.0.0")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "ambient-token-must-not-return")
	identityBin := filepath.Join(t.TempDir(), "identity-bin")

	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{SanitizedEnv: []string{"AWS_PROFILE=production", "PATH=" + identityBin + string(os.PathListSeparator) + setup.ambient}}).AnyTimes()
	runner := NewMockTerraformRunner(ctrl)
	var env map[string]string
	runner.EXPECT().SetEnv(gomock.Any()).DoAndReturn(func(got map[string]string) error { env = got; return nil }).AnyTimes()
	runner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	runner.EXPECT().WorkspaceSelect(gomock.Any(), "prod").Return(nil)
	runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"id": {Value: []byte(`"value"`)}}, nil)
	var executable string
	executor := NewExecutor(NewMockComponentDescriber(ctrl), WithRunnerFactory(func(_, got string) (TerraformRunner, error) {
		executable = got
		return runner, nil
	}))
	stateRoot := t.TempDir()
	setup.config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
		return &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{"path": filepath.Join(stateRoot, "default.tfstate")}, Workspace: "prod", WorkingDirectory: stateRoot}, nil
	}}
	sections := validSections()
	sections["settings"] = map[string]any{"configuration": true}
	sections[cfg.CommandSectionName] = "tofu"
	sections["dependencies"] = map[string]any{"tools": map[string]any{"tofu": "1.0.0", "fixture-tool": "1.0.0"}}

	outputs, err := executor.execute(t.Context(), setup.config, "network", "live", sections, nil, &OutputOptions{SkipInit: true}, true, manager)
	require.NoError(t, err)
	require.Equal(t, "value", outputs["id"])

	require.Equal(t, setup.tools["tofu@1.0.0"], executable)
	require.Equal(t, "production", env["AWS_PROFILE"], "identity environment is the base")
	require.NotContains(t, env, "AWS_WEB_IDENTITY_TOKEN_FILE", "ambient credentials must not return behind an identity")
	entries := filepath.SplitList(env["PATH"])
	require.Contains(t, entries, identityBin, "the identity PATH is extended, not replaced")
	found, ok := hasFixtureTool(env["PATH"])
	require.True(t, ok)
	require.Equal(t, setup.tools["fixture-tool@1.0.0"], found)
}
