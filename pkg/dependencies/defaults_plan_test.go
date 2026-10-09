package dependencies

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema fields these tests configure.
var _ = schema.Command{Dependencies: &schema.Dependencies{Tools: map[string]string{}}}

var errUnknownTool = errors.New("unknown tool")

// provisionerRecorder captures what the environment constructors hand to the
// installer and the PATH builder, so tests can assert what was (not) installed.
type provisionerRecorder struct {
	ensured     map[string]string
	ensureCalls int
	built       map[string]string
	buildCalls  int
}

type manifestTool struct{ owner, repo, version string }

// recordingProvisioner returns a mock-backed option. Name resolution is offline
// (aliases and owner/repo only) and unknown names fail, so no test reaches the
// network. FindBinaryPath reports only stub binaries created with installTools.
func recordingProvisioner(t *testing.T, config *schema.AtmosConfiguration) (*provisionerRecorder, envOption) {
	t.Helper()
	rec := &provisionerRecorder{}
	ids := newToolIdentity(config, &envConfig{})
	mock := NewMockToolProvisioner(gomock.NewController(t))
	mock.EXPECT().EnsureTools(gomock.Any()).DoAndReturn(func(deps map[string]string) error {
		rec.ensureCalls++
		rec.ensured = maps.Clone(deps)
		return nil
	}).AnyTimes()
	mock.EXPECT().ResolveToolName(gomock.Any()).DoAndReturn(func(tool string) (string, string, error) {
		id, ok := ids.offline(tool)
		if !ok {
			return "", "", errUnknownTool
		}
		owner, repo, _ := splitOwnerRepo(id)
		return owner, repo, nil
	}).AnyTimes()
	mock.EXPECT().FindBinaryPath(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(owner, repo, version string, _ ...string) (string, error) {
			name := repo
			if filepath.Ext(name) == "" && os.PathSeparator == '\\' {
				name += ".exe"
			}
			path := filepath.Join(config.Toolchain.InstallPath, "bin", owner, repo, version, name)
			if _, err := os.Stat(path); err != nil {
				return "", err
			}
			return path, nil
		},
	).AnyTimes()
	mock.EXPECT().BuildPATH(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *schema.AtmosConfiguration, deps map[string]string) (string, error) {
			rec.buildCalls++
			rec.built = maps.Clone(deps)
			return "toolchain-path", nil
		},
	).AnyTimes()
	return rec, withProvisioner(mock)
}

func installTools(t *testing.T, config *schema.AtmosConfiguration, tools ...manifestTool) {
	t.Helper()
	for _, tool := range tools {
		installedDefaultBinary(t, config, tool.owner, tool.repo, tool.version)
	}
}

func TestComponentSelectedToolsFromManifest(t *testing.T) {
	const terraformPath = "hashicorp/terraform"
	for _, tc := range []struct {
		name          string
		manifest      string
		installed     []manifestTool
		componentType string
		component     map[string]any
		commandConfig string
		wantInstall   map[string]string
		wantPATH      map[string]string
	}{
		{
			name:          "selected executable installed and other tools only when already installed",
			manifest:      "hashicorp/terraform 1.15.9\njqlang/jq 1.7.1\nkubernetes-sigs/kubectl 1.30.0\n",
			installed:     []manifestTool{{"kubernetes-sigs", "kubectl", "1.30.0"}},
			componentType: "terraform",
			wantInstall:   map[string]string{terraformPath: "1.15.9"},
			wantPATH:      map[string]string{terraformPath: "1.15.9", "kubernetes-sigs/kubectl": "1.30.0"},
		},
		{
			name:          "executable missing from manifest installs nothing",
			manifest:      "jqlang/jq 1.7.1\n",
			installed:     []manifestTool{{"jqlang", "jq", "1.7.1"}},
			componentType: "terraform",
			wantPATH:      map[string]string{"jqlang/jq": "1.7.1"},
		},
		{
			name:          "no installed tools and no selected tool leaves the environment empty",
			manifest:      "jqlang/jq 1.7.1\nhashicorp/packer 1.11.0\n",
			componentType: "terraform",
		},
		{
			name:          "command section resolved through builtin alias",
			manifest:      "opentofu/opentofu 1.8.0\nhashicorp/terraform 1.15.9\n",
			componentType: "terraform",
			component:     map[string]any{"command": "tofu"},
			wantInstall:   map[string]string{"opentofu/opentofu": "1.8.0"},
		},
		{
			name:          "bare manifest key equal to the command",
			manifest:      "tofu 1.8.0\n",
			componentType: "terraform",
			component:     map[string]any{"command": "tofu"},
			wantInstall:   map[string]string{"tofu": "1.8.0"},
		},
		{
			name:          "command path selects nothing",
			manifest:      "hashicorp/terraform 1.15.9\n",
			componentType: "terraform",
			component:     map[string]any{"command": filepath.Join(string(filepath.Separator), "opt", "bin", "terraform")},
		},
		{
			name:          "command from atmos.yaml when the section has none",
			manifest:      "hashicorp/terraform 1.15.9\nopentofu/opentofu 1.8.0\n",
			componentType: "terraform",
			commandConfig: "tofu",
			wantInstall:   map[string]string{"opentofu/opentofu": "1.8.0"},
		},
		{
			name:          "command section wins over atmos.yaml",
			manifest:      "hashicorp/terraform 1.15.9\nopentofu/opentofu 1.8.0\n",
			componentType: "terraform",
			component:     map[string]any{"command": "terraform"},
			commandConfig: "tofu",
			wantInstall:   map[string]string{terraformPath: "1.15.9"},
		},
		{
			name:          "helmfile installs helmfile and the helm companion",
			manifest:      "helmfile/helmfile 0.169.0\nhelm/helm 3.16.0\njqlang/jq 1.7.1\n",
			componentType: "helmfile",
			wantInstall:   map[string]string{"helmfile/helmfile": "0.169.0", "helm/helm": "3.16.0"},
		},
		{
			name:          "helmfile without a helm entry installs only helmfile",
			manifest:      "helmfile/helmfile 0.169.0\n",
			componentType: "helmfile",
			wantInstall:   map[string]string{"helmfile/helmfile": "0.169.0"},
		},
		{
			name:          "terraform does not select the helm companion",
			manifest:      "hashicorp/terraform 1.15.9\nhelm/helm 3.16.0\n",
			componentType: "terraform",
			wantInstall:   map[string]string{terraformPath: "1.15.9"},
		},
		{
			name:          "packer",
			manifest:      "hashicorp/packer 1.11.0\nhashicorp/terraform 1.15.9\n",
			componentType: "packer",
			wantInstall:   map[string]string{"hashicorp/packer": "1.11.0"},
		},
		{
			name:          "ansible",
			manifest:      "ansible/ansible 11.0.0\nhashicorp/terraform 1.15.9\n",
			componentType: "ansible",
			wantInstall:   map[string]string{"ansible/ansible": "11.0.0"},
		},
		{
			name:          "native helm selects helm",
			manifest:      "helm/helm 3.16.0\nhashicorp/terraform 1.15.9\n",
			componentType: "helm",
			wantInstall:   map[string]string{"helm/helm": "3.16.0"},
		},
		{
			name:          "native kubernetes runs in process and selects nothing",
			manifest:      "kubernetes-sigs/kubectl 1.30.0\nhashicorp/terraform 1.15.9\n",
			installed:     []manifestTool{{"kubernetes-sigs", "kubectl", "1.30.0"}},
			componentType: "kubernetes",
			wantPATH:      map[string]string{"kubernetes-sigs/kubectl": "1.30.0"},
		},
		{
			name:          "explicit dependency overrides the selected manifest tool by identity",
			manifest:      "hashicorp/terraform 1.15.9\n",
			componentType: "terraform",
			component:     dependencySection("1.9.0"),
			wantInstall:   map[string]string{"terraform": "1.9.0"},
		},
		{
			name:          "unusable and unresolvable entries are skipped without aborting",
			manifest:      "hashicorp/terraform 1.15.9\nnodejs 20.1.0\nkubectl system\nhashicorp/packer ref:main\njqlang/jq path:/opt/jq\nhashicorp/vault ~>\n",
			componentType: "terraform",
			wantInstall:   map[string]string{terraformPath: "1.15.9"},
		},
	} {
		for _, entrypoint := range []string{"component", "sections"} {
			t.Run(tc.name+"/"+entrypoint, func(t *testing.T) {
				config, _ := defaultsFixture(t, tc.manifest)
				config.Components.Terraform.Command = tc.commandConfig
				installTools(t, config, tc.installed...)
				rec, option := recordingProvisioner(t, config)

				var err error
				if entrypoint == "component" {
					_, err = forComponent(config, tc.componentType, nil, tc.component, option)
				} else {
					sections := maps.Clone(tc.component)
					if sections == nil {
						sections = map[string]any{}
					}
					sections["component_info"] = map[string]any{"component_type": tc.componentType}
					_, err = forSections(config, sections, option)
				}
				require.NoError(t, err)

				assert.Equal(t, tc.wantInstall, rec.ensured)
				if tc.wantInstall == nil {
					assert.Zero(t, rec.ensureCalls, "the installer must not run when nothing is selected")
				}
				if tc.wantPATH == nil {
					tc.wantPATH = tc.wantInstall
				}
				assert.Equal(t, tc.wantPATH, rec.built)
			})
		}
	}
}

func TestComponentSelectedToolsNotPassedToInstallerWhenMissing(t *testing.T) {
	config, _ := defaultsFixture(t, "hashicorp/terraform 1.15.9\njqlang/jq 1.7.1\nhashicorp/packer 1.11.0\n")
	rec, option := recordingProvisioner(t, config)

	_, err := forComponent(config, "terraform", nil, nil, option)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"hashicorp/terraform": "1.15.9"}, rec.ensured)
	assert.NotContains(t, rec.built, "jqlang/jq", "a missing tool must not reach PATH construction")
	assert.NotContains(t, rec.built, "hashicorp/packer")
}

func TestForDependenciesIsPathOnlyBaseline(t *testing.T) {
	for _, entrypoint := range []string{"dependencies", "command"} {
		for _, tc := range []struct {
			name        string
			explicit    map[string]string
			wantInstall map[string]string
			wantPATH    map[string]string
		}{
			{
				name:     "manifest tools are never installed",
				wantPATH: map[string]string{"jqlang/jq": "1.7.1"},
			},
			{
				name:        "explicit dependency is installed next to installed manifest tools",
				explicit:    map[string]string{"mikefarah/yq": "4.40.0"},
				wantInstall: map[string]string{"mikefarah/yq": "4.40.0"},
				wantPATH:    map[string]string{"mikefarah/yq": "4.40.0", "jqlang/jq": "1.7.1"},
			},
			{
				name:        "explicit dependency overrides the manifest by alias identity",
				explicit:    map[string]string{"jq": "1.6.0"},
				wantInstall: map[string]string{"jq": "1.6.0"},
				wantPATH:    map[string]string{"jq": "1.6.0"},
			},
			{
				name:        "explicit dependency for an uninstalled manifest tool replaces it",
				explicit:    map[string]string{"hashicorp/terraform": "1.9.0"},
				wantInstall: map[string]string{"hashicorp/terraform": "1.9.0"},
				wantPATH:    map[string]string{"hashicorp/terraform": "1.9.0", "jqlang/jq": "1.7.1"},
			},
		} {
			t.Run(entrypoint+"/"+tc.name, func(t *testing.T) {
				config, _ := defaultsFixture(t, "hashicorp/terraform 1.15.9\njqlang/jq 1.7.1\n")
				config.Toolchain.Aliases["jq"] = "jqlang/jq"
				installTools(t, config, manifestTool{"jqlang", "jq", "1.7.1"})
				rec, option := recordingProvisioner(t, config)

				var err error
				if entrypoint == "dependencies" {
					_, err = ForDependencies(config, tc.explicit, option)
				} else {
					command := &schema.Command{Dependencies: &schema.Dependencies{Tools: tc.explicit}}
					_, err = ForCommand(config, command, option)
				}
				require.NoError(t, err)
				assert.Equal(t, tc.wantInstall, rec.ensured)
				assert.Equal(t, tc.wantPATH, rec.built)
			})
		}
	}
}

func TestForCommandInheritsInstalledManifestTools(t *testing.T) {
	config, _ := defaultsFixture(t, "jqlang/jq 1.7.1\n")
	jq := installedDefaultBinary(t, config, "jqlang", "jq", "1.7.1")

	env, err := ForCommand(config, &schema.Command{Name: "demo"})
	require.NoError(t, err)
	assert.Equal(t, jq, env.Resolve("jq"))
	assert.Equal(t, []string{filepath.Dir(jq)}, env.ToolchainDirs())
	assert.Contains(t, env.PATH(), filepath.Dir(jq))

	// A nil command still yields the baseline.
	env, err = ForCommand(config, nil)
	require.NoError(t, err)
	assert.Equal(t, jq, env.Resolve("jq"))
}

func TestForWorkflowInstallsEveryUsableManifestTool(t *testing.T) {
	for _, tc := range []struct {
		name        string
		manifest    string
		explicit    map[string]string
		wantInstall map[string]string
	}{
		{
			name:        "all usable entries",
			manifest:    "hashicorp/terraform 1.15.9\njqlang/jq 1.7.1\n",
			wantInstall: map[string]string{"hashicorp/terraform": "1.15.9", "jqlang/jq": "1.7.1"},
		},
		{
			name:        "skipped and unresolvable entries do not abort",
			manifest:    "hashicorp/terraform 1.15.9\nnodejs 20.1.0\nkubectl system\nhashicorp/packer ref:main\njqlang/jq path:/opt/jq\nhashicorp/vault ~>\n",
			wantInstall: map[string]string{"hashicorp/terraform": "1.15.9"},
		},
		{
			name:        "workflow dependency overrides by identity",
			manifest:    "hashicorp/terraform 1.15.9\njqlang/jq 1.7.1\n",
			explicit:    map[string]string{"terraform": "1.9.0"},
			wantInstall: map[string]string{"terraform": "1.9.0", "jqlang/jq": "1.7.1"},
		},
		{
			name:        "same tool under two names with the same version collapses",
			manifest:    "tofu 1.12.6\nopentofu/opentofu 1.12.6\n",
			wantInstall: map[string]string{"opentofu/opentofu": "1.12.6"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, _ := defaultsFixture(t, tc.manifest)
			rec, option := recordingProvisioner(t, config)
			workflow := &schema.WorkflowDefinition{Dependencies: &schema.Dependencies{Tools: tc.explicit}}

			_, err := ForWorkflow(config, workflow, option)
			require.NoError(t, err)
			assert.Equal(t, tc.wantInstall, rec.ensured)
		})
	}
}

func TestDuplicateManifestIdentities(t *testing.T) {
	for _, entrypoint := range []string{"workflow", "command", "component"} {
		for _, tc := range []struct {
			name      string
			manifest  string
			explicit  map[string]string
			wantError bool
			wantKeys  []string
		}{
			{name: "different versions are a conflict", manifest: "tofu 1.12.6\nopentofu/opentofu 1.11.0\n", wantError: true, wantKeys: []string{"tofu", "opentofu/opentofu"}},
			{name: "configured alias conflicts with qualified name", manifest: "terraform 1.15.9\nhashicorp/terraform 1.14.0\n", wantError: true, wantKeys: []string{"terraform", "hashicorp/terraform"}},
			{name: "overridden conflict is ignored", manifest: "tofu 1.12.6\nopentofu/opentofu 1.11.0\n", explicit: map[string]string{"opentofu/opentofu": "1.10.0"}},
			{name: "unresolvable duplicates are not compared", manifest: "nodejs 20\nnode 22\n"},
			{name: "distinct tools never conflict", manifest: "tofu 1.12.6\nhashicorp/terraform 1.11.0\n"},
		} {
			t.Run(entrypoint+"/"+tc.name, func(t *testing.T) {
				config, _ := defaultsFixture(t, tc.manifest)
				rec, option := recordingProvisioner(t, config)
				deps := &schema.Dependencies{Tools: tc.explicit}

				var err error
				switch entrypoint {
				case "workflow":
					_, err = ForWorkflow(config, &schema.WorkflowDefinition{Dependencies: deps}, option)
				case "command":
					_, err = ForCommand(config, &schema.Command{Dependencies: deps}, option)
				case "component":
					_, err = forComponent(config, "terraform", nil, map[string]any{"dependencies": map[string]any{"tools": toAnyMap(tc.explicit)}}, option)
				}

				if !tc.wantError {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, errUtils.ErrToolVersionsConflict)
				assert.Contains(t, err.Error(), "failed to overlay tool defaults")
				assert.Nil(t, rec.ensured, "nothing may install after a conflict")
				assert.Zero(t, rec.ensureCalls)
				formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
				for _, key := range tc.wantKeys {
					assert.Contains(t, formatted, key)
				}
			})
		}
	}
}

func toAnyMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func TestDefaultsResolutionFailures(t *testing.T) {
	cause := errors.New("registry unavailable")
	resolve := func(failing string) envOption {
		return withResolveFunc(func(tool string) (string, string, error) {
			if tool == failing {
				return "", "", cause
			}
			return "owner", tool, nil
		})
	}
	noInstall := withEnsureTools(func(map[string]string) error {
		t.Fatal("must not install after a resolution failure")
		return nil
	})

	t.Run("explicit dependencies stay strict", func(t *testing.T) {
		config, _ := defaultsFixture(t, "default 1.0.0\n")
		env, err := newEnvironmentWithDefaults(config, map[string]string{"explicit": "2.0.0"}, defaultsRequest{installAll: true}, resolve("explicit"), noInstall)
		require.ErrorIs(t, err, cause)
		assert.Contains(t, err.Error(), "explicit")
		assert.Nil(t, env)
	})

	t.Run("defaults are best effort", func(t *testing.T) {
		config, _ := defaultsFixture(t, "default 1.0.0\n")
		var installed map[string]string
		_, err := newEnvironmentWithDefaults(
			config, map[string]string{"owner/explicit": "2.0.0"}, defaultsRequest{installAll: true}, resolve("default"),
			withEnsureTools(func(deps map[string]string) error { installed = maps.Clone(deps); return nil }),
			withFindBinaryPath(func(_, _, _ string, _ ...string) (string, error) { return "", os.ErrNotExist }),
			withBuildPATH(func(*schema.AtmosConfiguration, map[string]string) (string, error) { return "", nil }),
			withEntrypointDirs(func(_, _, _ string) []string { return nil }),
		)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"owner/explicit": "2.0.0"}, installed)
	})
}

func TestPlanToolDefaultsIsolation(t *testing.T) {
	config, _ := defaultsFixture(t, "")
	ids := newToolIdentity(config, &envConfig{})
	manifest := map[string]string{"hashicorp/terraform": "1.15.9", "jqlang/jq": "1.7.1"}
	explicit := map[string]string{"mikefarah/yq": "4.40.0"}
	wantManifest, wantExplicit := maps.Clone(manifest), maps.Clone(explicit)

	plan, err := planToolDefaults(manifest, explicit, defaultsRequest{installAll: true}, ids)
	require.NoError(t, err)
	want := map[string]string{"hashicorp/terraform": "1.15.9", "jqlang/jq": "1.7.1", "mikefarah/yq": "4.40.0"}
	assert.Equal(t, want, plan.install)

	// Result to source: the installer resolves version ranges in place.
	plan.install["jqlang/jq"] = "9.9.9"
	plan.install["new"] = "1.0.0"
	assert.Equal(t, wantManifest, manifest)
	assert.Equal(t, wantExplicit, explicit)

	// Source to result.
	plan, err = planToolDefaults(manifest, explicit, defaultsRequest{installAll: true}, ids)
	require.NoError(t, err)
	manifest["hashicorp/terraform"] = "0.0.1"
	explicit["mikefarah/yq"] = "0.0.1"
	assert.Equal(t, want, plan.install)
}

func TestPlanToolDefaultsExactKeyOverridesWithoutResolution(t *testing.T) {
	ids := &toolIdentity{resolve: func(string) (string, string, error) {
		t.Fatal("exact key overrides must not resolve tools")
		return "", "", nil
	}}
	plan, err := planToolDefaults(
		map[string]string{"unknown": "~>2.0.0"},
		map[string]string{"unknown": "1.0.0"},
		defaultsRequest{installAll: true}, ids,
	)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"unknown": "1.0.0"}, plan.install)
	assert.Empty(t, plan.present)
}

func TestUsableDefaults(t *testing.T) {
	manifest := map[string]string{
		"a/system": "system", "b/ref": "ref:main", "c/path": "path:/opt/c", "d/exact": "1.2.3", "e/constraint": "~>1.9.0",
		"f/latest": "latest",
	}
	for _, operator := range []string{"~>", ">=", "<=", ">", "<", "=", "^", "~", "!="} {
		manifest["op/"+operator] = operator
	}
	assert.Equal(t, map[string]string{"d/exact": "1.2.3", "e/constraint": "~>1.9.0", "f/latest": "latest"}, usableDefaults(manifest))
	assert.Empty(t, usableDefaults(nil))
}

func TestKeyMatchesExecutable(t *testing.T) {
	config, _ := defaultsFixture(t, "")
	ids := newToolIdentity(config, &envConfig{})
	for _, tc := range []struct {
		key, executable string
		want            bool
	}{
		{"terraform", "terraform", true},
		{"hashicorp/terraform", "terraform", true},
		{"tf", "terraform", true},
		{"terraform", "tf", true},
		{"opentofu/opentofu", "tofu", true},
		{"tofu", "tofu", true},
		{"jqlang/jq", "jq", true},
		{"hashicorp/terraform", "tofu", false},
		{"opentofu/opentofu", "terraform", false},
		{"hashicorp/terraform-docs", "terraform", false},
		{"nodejs", "terraform", false},
		{"opentofu", "tofu", false},
	} {
		t.Run(tc.key+"/"+tc.executable, func(t *testing.T) {
			assert.Equal(t, tc.want, ids.keyMatchesExecutable(tc.key, tc.executable))
		})
	}
}

func TestSelectedComponentTools(t *testing.T) {
	absolute := filepath.Join(string(filepath.Separator), "opt", "bin", "terraform")
	for _, tc := range []struct {
		name          string
		componentType string
		section       map[string]any
		command       string
		want          []string
	}{
		{"terraform default", "terraform", nil, "", []string{"terraform"}},
		{"terraform section", "terraform", map[string]any{"command": "tofu"}, "tofu-config", []string{"tofu"}},
		{"terraform atmos.yaml", "terraform", map[string]any{}, "tofu", []string{"tofu"}},
		{"empty section command falls through", "terraform", map[string]any{"command": ""}, "tofu", []string{"tofu"}},
		{"non-string section command falls through", "terraform", map[string]any{"command": 1}, "", []string{"terraform"}},
		{"windows suffix", "terraform", map[string]any{"command": "tofu.exe"}, "", []string{"tofu"}},
		{"absolute path", "terraform", map[string]any{"command": absolute}, "", nil},
		{"relative path", "terraform", map[string]any{"command": filepath.Join("bin", "terraform")}, "", nil},
		{"helmfile", "helmfile", nil, "", []string{"helmfile", "helm"}},
		{"helmfile path still selects helm", "helmfile", map[string]any{"command": absolute}, "", []string{"helm"}},
		{"packer", "packer", nil, "", []string{"packer"}},
		{"ansible", "ansible", nil, "", []string{"ansible"}},
		{"helm", "helm", map[string]any{"command": "other"}, "", []string{"helm"}},
		{"kubernetes", "kubernetes", map[string]any{"command": "kubectl"}, "", nil},
		{"unknown type without command", "mock", nil, "", nil},
		{"unknown type with command", "", map[string]any{"command": "tofu"}, "", []string{"tofu"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Components.Terraform.Command = tc.command
			assert.Equal(t, tc.want, selectedComponentTools(config, tc.componentType, tc.section))
		})
	}
	assert.Equal(t, []string{"terraform"}, selectedComponentTools(nil, "terraform", nil), "nil config uses the type default")
}

func TestComponentTypeFromSections(t *testing.T) {
	assert.Equal(t, "helmfile", componentTypeFromSections(map[string]any{"component_info": map[string]any{"component_type": "helmfile"}}))
	assert.Empty(t, componentTypeFromSections(nil))
	assert.Empty(t, componentTypeFromSections(map[string]any{"component_info": "invalid"}))
	assert.Empty(t, componentTypeFromSections(map[string]any{"component_info": map[string]any{"component_type": 1}}))
}
