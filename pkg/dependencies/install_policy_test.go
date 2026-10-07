package dependencies

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema field these tests configure.
var _ = schema.Toolchain{Install: schema.ToolchainInstallNever}

const installPolicyManifest = "hashicorp/terraform 1.15.9\njqlang/jq 1.7.1\nkubernetes-sigs/kubectl 1.30.0\n"

var (
	allManifestTools = map[string]string{
		"hashicorp/terraform":     "1.15.9",
		"jqlang/jq":               "1.7.1",
		"kubernetes-sigs/kubectl": "1.30.0",
	}
	terraformOnly = map[string]string{"hashicorp/terraform": "1.15.9"}
	jqOnly        = map[string]string{"jqlang/jq": "1.7.1"}
	terraformAndJ = map[string]string{"hashicorp/terraform": "1.15.9", "jqlang/jq": "1.7.1"}
)

// runKind names the constructors the install policy applies to.
type runKind string

const (
	runComponent    runKind = "component"
	runSections     runKind = "sections"
	runWorkflow     runKind = "workflow"
	runCommand      runKind = "command"
	runDependencies runKind = "dependencies"
)

var allRunKinds = []runKind{runComponent, runSections, runWorkflow, runCommand, runDependencies}

// buildFor calls the constructor for kind with the explicit dependencies.
func buildFor(config *schema.AtmosConfiguration, kind runKind, explicit map[string]string, option envOption) (*ToolchainEnvironment, error) {
	switch kind {
	case runComponent:
		return forComponent(config, "terraform", nil, map[string]any{"dependencies": map[string]any{"tools": toAnyMap(explicit)}}, option)
	case runSections:
		sections := map[string]any{
			"component_info": map[string]any{"component_type": "terraform"},
			"dependencies":   map[string]any{"tools": toAnyMap(explicit)},
		}
		return forSections(config, sections, option)
	case runWorkflow:
		return ForWorkflow(config, &schema.WorkflowDefinition{Dependencies: &schema.Dependencies{Tools: explicit}}, option)
	case runCommand:
		return ForCommand(config, &schema.Command{Dependencies: &schema.Dependencies{Tools: explicit}}, option)
	default:
		return ForDependencies(config, explicit, option)
	}
}

// installPolicyFixture writes the manifest and installs only jq, so every policy
// can be told apart: terraform is pinned but missing, jq is present, kubectl is
// pinned but missing.
func installPolicyFixture(t *testing.T, policy schema.ToolchainInstall) (*schema.AtmosConfiguration, *provisionerRecorder, envOption) {
	t.Helper()
	config, _ := defaultsFixture(t, installPolicyManifest)
	config.Toolchain.Install = policy
	installTools(t, config, manifestTool{"jqlang", "jq", "1.7.1"})
	rec, option := recordingProvisioner(t, config)
	return config, rec, option
}

func TestInstallPolicyManifestHandling(t *testing.T) {
	type outcome struct {
		install map[string]string // What the installer is asked to install; nil means it must not run.
		path    map[string]string // What reaches PATH construction; nil means nothing does.
	}
	tests := []struct {
		name   string
		policy schema.ToolchainInstall
		want   map[runKind]outcome
	}{
		{
			name:   "never installs nothing and keeps installed manifest tools on PATH",
			policy: schema.ToolchainInstallNever,
			want: map[runKind]outcome{
				runComponent:    {path: jqOnly},
				runSections:     {path: jqOnly},
				runWorkflow:     {path: jqOnly},
				runCommand:      {path: jqOnly},
				runDependencies: {path: jqOnly},
			},
		},
		{
			name:   "declared keeps installed manifest tools and installs workflow defaults",
			policy: schema.ToolchainInstallDeclared,
			want: map[runKind]outcome{
				runComponent:    {path: jqOnly},
				runSections:     {path: jqOnly},
				runWorkflow:     {install: allManifestTools, path: allManifestTools},
				runCommand:      {path: jqOnly},
				runDependencies: {path: jqOnly},
			},
		},
		{
			name:   "auto installs the selected executable and keeps other installed tools on PATH",
			policy: schema.ToolchainInstallAuto,
			want: map[runKind]outcome{
				runComponent:    {install: terraformOnly, path: terraformAndJ},
				runSections:     {install: terraformOnly, path: terraformAndJ},
				runWorkflow:     {install: allManifestTools, path: allManifestTools},
				runCommand:      {path: jqOnly},
				runDependencies: {path: jqOnly},
			},
		},
		{
			name:   "an unset policy behaves like auto",
			policy: "",
			want: map[runKind]outcome{
				runComponent:    {install: terraformOnly, path: terraformAndJ},
				runSections:     {install: terraformOnly, path: terraformAndJ},
				runWorkflow:     {install: allManifestTools, path: allManifestTools},
				runCommand:      {path: jqOnly},
				runDependencies: {path: jqOnly},
			},
		},
		{
			name:   "always installs every manifest tool for every run",
			policy: schema.ToolchainInstallAlways,
			want: map[runKind]outcome{
				runComponent:    {install: allManifestTools, path: allManifestTools},
				runSections:     {install: allManifestTools, path: allManifestTools},
				runWorkflow:     {install: allManifestTools, path: allManifestTools},
				runCommand:      {install: allManifestTools, path: allManifestTools},
				runDependencies: {install: allManifestTools, path: allManifestTools},
			},
		},
	}

	for _, tt := range tests {
		for _, kind := range allRunKinds {
			t.Run(tt.name+"/"+string(kind), func(t *testing.T) {
				config, rec, option := installPolicyFixture(t, tt.policy)

				_, err := buildFor(config, kind, nil, option)
				require.NoError(t, err)

				want := tt.want[kind]
				assert.Equal(t, want.install, rec.ensured)
				if want.install == nil {
					assert.Zero(t, rec.ensureCalls, "the installer must not run")
				}
				assert.Equal(t, want.path, rec.built)
			})
		}
	}
}

func TestInstallPolicyExplicitDependencies(t *testing.T) {
	const yq = "mikefarah/yq"
	explicit := map[string]string{yq: "4.40.0"}

	// Explicit dependencies are installed by every policy except never, and they
	// override the manifest entry for the same tool.
	for _, policy := range []schema.ToolchainInstall{schema.ToolchainInstallDeclared, schema.ToolchainInstallAuto, schema.ToolchainInstallAlways} {
		for _, kind := range allRunKinds {
			t.Run(string(policy)+"/"+string(kind)+" installs explicit dependencies", func(t *testing.T) {
				config, rec, option := installPolicyFixture(t, policy)

				_, err := buildFor(config, kind, explicit, option)
				require.NoError(t, err)

				assert.Equal(t, 1, rec.ensureCalls)
				assert.Equal(t, "4.40.0", rec.ensured[yq])
				assert.Equal(t, "4.40.0", rec.built[yq])
			})
		}
	}

	t.Run("an explicit dependency overrides the manifest entry under always", func(t *testing.T) {
		config, rec, option := installPolicyFixture(t, schema.ToolchainInstallAlways)

		_, err := buildFor(config, runCommand, map[string]string{"terraform": "1.9.0"}, option)
		require.NoError(t, err)

		assert.Equal(t, map[string]string{"terraform": "1.9.0", "jqlang/jq": "1.7.1", "kubernetes-sigs/kubectl": "1.30.0"}, rec.ensured)
	})
}

func TestInstallPolicyNeverUsesInstalledDependencies(t *testing.T) {
	const yq = "mikefarah/yq"

	for _, kind := range allRunKinds {
		t.Run(string(kind)+"/installed explicit dependency is PATH-only", func(t *testing.T) {
			config, rec, option := installPolicyFixture(t, schema.ToolchainInstallNever)
			installTools(t, config, manifestTool{"mikefarah", "yq", "4.40.0"})

			_, err := buildFor(config, kind, map[string]string{yq: "4.40.0"}, option)
			require.NoError(t, err)

			assert.Zero(t, rec.ensureCalls, "never must not call the installer")
			assert.Equal(t, map[string]string{yq: "4.40.0", "jqlang/jq": "1.7.1"}, rec.built)
		})

		t.Run(string(kind)+"/missing explicit dependency is an error", func(t *testing.T) {
			config, rec, option := installPolicyFixture(t, schema.ToolchainInstallNever)

			env, err := buildFor(config, kind, map[string]string{yq: "4.40.0", "hashicorp/packer": "1.11.0"}, option)

			require.ErrorIs(t, err, errUtils.ErrToolNotInstalled)
			assert.Nil(t, env)
			assert.Zero(t, rec.ensureCalls)
			assert.Zero(t, rec.buildCalls, "no environment may be built after the error")
			formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			assert.Contains(t, formatted, "mikefarah/yq 4.40.0")
			assert.Contains(t, formatted, "hashicorp/packer 1.11.0")
			assert.Contains(t, formatted, "atmos toolchain install")
		})
	}

	t.Run("a missing manifest tool is skipped silently", func(t *testing.T) {
		config, rec, option := installPolicyFixture(t, schema.ToolchainInstallNever)

		_, err := buildFor(config, runComponent, nil, option)
		require.NoError(t, err)

		assert.NotContains(t, rec.built, "hashicorp/terraform")
		assert.NotContains(t, rec.built, "kubernetes-sigs/kubectl")
	})

	t.Run("an explicit dependency that cannot be resolved is an error", func(t *testing.T) {
		config, rec, option := installPolicyFixture(t, schema.ToolchainInstallNever)

		_, err := buildFor(config, runCommand, map[string]string{"nodejs": "20.1.0"}, option)

		require.ErrorIs(t, err, errUnknownTool)
		assert.Zero(t, rec.ensureCalls)
	})
}

func TestInstallPolicyNeverResolvesConstraintsFromInstalledVersions(t *testing.T) {
	const yq = "mikefarah/yq"
	listInstalled := func(owner, repo string) ([]string, error) {
		if owner == "mikefarah" && repo == "yq" {
			return []string{"4.39.0", "4.40.2", "4.41.0"}, nil
		}
		return nil, nil
	}

	tests := []struct {
		name       string
		constraint string
		installed  []string // Versions that exist on disk.
		wantPath   string
		wantErr    bool
	}{
		{name: "highest installed match", constraint: "~>4.40.0", installed: []string{"4.40.2"}, wantPath: "4.40.2"},
		{name: "installed version outside the constraint", constraint: "~>5.0.0", installed: []string{"4.40.2"}, wantErr: true},
		{name: "matching version listed but missing on disk", constraint: "~>4.40.0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, rec, option := installPolicyFixture(t, schema.ToolchainInstallNever)
			for _, version := range tt.installed {
				installTools(t, config, manifestTool{"mikefarah", "yq", version})
			}

			_, err := buildFor(config, runCommand, map[string]string{yq: tt.constraint}, func(c *envConfig) {
				option(c)
				withListInstalled(listInstalled)(c)
			})

			assert.Zero(t, rec.ensureCalls)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrToolNotInstalled)
				assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), yq+" "+tt.constraint)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, rec.built[yq])
		})
	}

	t.Run("manifest constraints resolve to the installed version", func(t *testing.T) {
		config, _ := defaultsFixture(t, "mikefarah/yq ~>4.40.0\n")
		config.Toolchain.Install = schema.ToolchainInstallNever
		installTools(t, config, manifestTool{"mikefarah", "yq", "4.40.2"})
		rec, option := recordingProvisioner(t, config)

		_, err := ForCommand(config, nil, option, withListInstalled(listInstalled))
		require.NoError(t, err)

		assert.Equal(t, map[string]string{yq: "4.40.2"}, rec.built)
	})
}

func TestInstallPolicyAlwaysReadsTheManifest(t *testing.T) {
	// Installation policy never hides an unreadable project baseline.
	for _, policy := range schema.ToolchainInstallValues {
		for _, kind := range allRunKinds {
			t.Run(string(policy)+"/"+string(kind), func(t *testing.T) {
				config, root := defaultsFixture(t, "")
				config.Toolchain.Install = policy
				manifest := filepath.Join(root, ".tool-versions")
				require.NoError(t, os.Remove(manifest))
				require.NoError(t, os.Mkdir(manifest, 0o755))
				_, option := recordingProvisioner(t, config)

				_, err := buildFor(config, kind, nil, option)

				require.ErrorContains(t, err, "failed to load .tool-versions")
			})
		}
	}
}

func TestInstallPolicyAlwaysSelectsInstalledManifestTools(t *testing.T) {
	// Installed project selections win over system PATH under every download policy.
	decoyDir := t.TempDir()
	decoyName := "terraform"
	if runtime.GOOS == "windows" {
		decoyName += ".exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(decoyDir, decoyName), []byte("decoy"), 0o755))

	for _, policy := range schema.ToolchainInstallValues {
		t.Run(string(policy), func(t *testing.T) {
			config, _ := defaultsFixture(t, "hashicorp/terraform 1.15.9\n")
			config.Toolchain.Install = policy
			stub := installedDefaultBinary(t, config, "hashicorp", "terraform", "1.15.9")
			t.Setenv("PATH", decoyDir)

			env, err := ForComponent(config, "terraform", nil, nil)
			require.NoError(t, err)

			assert.Equal(t, stub, env.Resolve("terraform"))
			assert.Equal(t, []string{filepath.Dir(stub)}, env.ToolchainDirs())
			assert.NotNil(t, env.EnvVars())
		})
	}
}

func TestInstallPolicyNeverMissingExplicitDependencyWithRealInstaller(t *testing.T) {
	config, _ := defaultsFixture(t, "")
	config.Toolchain.Install = schema.ToolchainInstallNever
	section := map[string]any{"dependencies": map[string]any{"tools": map[string]any{"hashicorp/terraform": "1.9.0"}}}

	env, err := ForComponent(config, "terraform", nil, section)

	require.ErrorIs(t, err, errUtils.ErrToolNotInstalled)
	assert.Nil(t, env)
	require.NoDirExists(t, filepath.Join(config.Toolchain.InstallPath, "bin", "hashicorp", "terraform", "1.9.0"))
}

func TestInstallPolicyInvalidValue(t *testing.T) {
	for _, kind := range allRunKinds {
		t.Run(string(kind), func(t *testing.T) {
			config, rec, option := installPolicyFixture(t, "sometimes")

			env, err := buildFor(config, kind, nil, option)

			require.ErrorIs(t, err, errUtils.ErrInvalidToolchainInstall)
			assert.Nil(t, env)
			assert.Zero(t, rec.ensureCalls)
			formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			assert.Contains(t, formatted, "sometimes")
			for _, value := range []string{"never", "declared", "auto", "always"} {
				assert.Contains(t, formatted, value)
			}
		})
	}

	t.Run("NewEnvironmentFromDeps", func(t *testing.T) {
		config, _ := defaultsFixture(t, "")
		config.Toolchain.Install = "sometimes"

		_, err := NewEnvironmentFromDeps(config, map[string]string{"jqlang/jq": "1.7.1"})

		require.ErrorIs(t, err, errUtils.ErrInvalidToolchainInstall)
	})
}

func TestInstallPolicyResolution(t *testing.T) {
	tests := []struct {
		name    string
		config  *schema.AtmosConfiguration
		want    schema.ToolchainInstall
		wantErr bool
	}{
		{name: "nil configuration", config: nil, want: schema.ToolchainInstallAuto},
		{name: "unset", config: &schema.AtmosConfiguration{}, want: schema.ToolchainInstallAuto},
		{name: "never", config: &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: "never"}}, want: schema.ToolchainInstallNever},
		{name: "declared", config: &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: "declared"}}, want: schema.ToolchainInstallDeclared},
		{name: "always", config: &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: "always"}}, want: schema.ToolchainInstallAlways},
		{name: "unknown", config: &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: "Never"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InstallPolicy(tt.config)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrInvalidToolchainInstall)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			automatic, err := InstallsAutomatically(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.want != schema.ToolchainInstallNever, automatic)
		})
	}

	t.Run("InstallsAutomatically propagates an invalid policy", func(t *testing.T) {
		_, err := InstallsAutomatically(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: "bogus"}})
		require.ErrorIs(t, err, errUtils.ErrInvalidToolchainInstall)
	})
}

func TestNewEnvironmentFromDepsHonorsNever(t *testing.T) {
	deps := map[string]string{"jqlang/jq": "1.7.1", "hashicorp/terraform": "1.15.9", "nodejs": "20.1.0"}

	t.Run("never installs nothing and keeps only installed tools", func(t *testing.T) {
		config, _ := defaultsFixture(t, "")
		config.Toolchain.Install = schema.ToolchainInstallNever
		installTools(t, config, manifestTool{"jqlang", "jq", "1.7.1"})
		rec, option := recordingProvisioner(t, config)

		_, err := newEnvironmentFromDeps(config, deps, option)
		require.NoError(t, err)

		assert.Zero(t, rec.ensureCalls)
		assert.Equal(t, jqOnly, rec.built)
	})

	t.Run("never with nothing installed yields an empty environment", func(t *testing.T) {
		config, _ := defaultsFixture(t, "")
		config.Toolchain.Install = schema.ToolchainInstallNever
		rec, option := recordingProvisioner(t, config)

		env, err := newEnvironmentFromDeps(config, deps, option)
		require.NoError(t, err)

		assert.Zero(t, rec.ensureCalls)
		assert.Zero(t, rec.buildCalls)
		assert.Empty(t, env.PATH())
	})

	for _, policy := range []schema.ToolchainInstall{"", schema.ToolchainInstallDeclared, schema.ToolchainInstallAuto, schema.ToolchainInstallAlways} {
		t.Run("policy "+string(policy)+" still installs every tool in the map", func(t *testing.T) {
			config, _ := defaultsFixture(t, "")
			config.Toolchain.Install = policy
			rec, option := recordingProvisioner(t, config)

			_, err := newEnvironmentFromDeps(config, deps, option)
			require.NoError(t, err)

			assert.Equal(t, 1, rec.ensureCalls)
			assert.Equal(t, deps, rec.ensured)
		})
	}
}

func TestInstallPolicyDoesNotMutateInputs(t *testing.T) {
	config, _, option := installPolicyFixture(t, schema.ToolchainInstallNever)
	installTools(t, config, manifestTool{"mikefarah", "yq", "4.40.0"})
	explicit := map[string]string{"mikefarah/yq": "4.40.0"}

	_, err := buildFor(config, runDependencies, explicit, option)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"mikefarah/yq": "4.40.0"}, explicit)
}
