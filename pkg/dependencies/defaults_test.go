package dependencies

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Installed fixtures exercise the public constructors without downloading tools.
func defaultsFixture(t *testing.T, manifest string) (*schema.AtmosConfiguration, string) {
	t.Helper()
	root := t.TempDir()
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	config := &schema.AtmosConfiguration{
		BasePath: root, BasePathAbsolute: root,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(root, "tools"),
			Aliases:     map[string]string{"terraform": "hashicorp/terraform", "tf": "hashicorp/terraform"},
		},
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tool-versions"), []byte(manifest), 0o600))
	return config, root
}

func installedDefaultBinary(t *testing.T, config *schema.AtmosConfiguration, owner, repo, version string) string {
	t.Helper()
	dir := filepath.Join(config.Toolchain.InstallPath, "bin", owner, repo, version)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	name := repo
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(binary, []byte("fixture"), 0o755))
	return binary
}

func TestComponentToolVersionsDefaults(t *testing.T) {
	for _, entrypoint := range []string{"component", "sections", "version"} {
		t.Run(entrypoint, func(t *testing.T) {
			config, root := defaultsFixture(t, "hashicorp/terraform 1.15.9 1.14.0\njqlang/jq 1.7.1\n")
			tf := installedDefaultBinary(t, config, "hashicorp", "terraform", "1.15.9")
			jq := installedDefaultBinary(t, config, "jqlang", "jq", "1.7.1")
			// The project manifest must be found even from a component directory.
			componentDir := filepath.Join(root, "components", "example")
			require.NoError(t, os.MkdirAll(componentDir, 0o755))
			t.Chdir(componentDir)
			var env *ToolchainEnvironment
			var err error
			switch entrypoint {
			case "component":
				env, err = ForComponent(config, "terraform", map[string]any{}, map[string]any{})
			case "sections":
				env, err = ForSections(config, map[string]any{})
			case "version":
				env, err = ForComponent(config, "terraform", nil, nil)
			}
			require.NoError(t, err)
			assert.Equal(t, tf, env.Resolve("terraform"))
			assert.Equal(t, jq, env.Resolve("jq"))
			assert.Contains(t, env.PATH(), filepath.Dir(tf))
			assert.ElementsMatch(t, []string{filepath.Dir(tf), filepath.Dir(jq)}, env.ToolchainDirs())
			absolute := filepath.Join(root, "custom", "terraform")
			assert.Equal(t, absolute, env.Resolve(absolute))
		})
	}
}

func dependencySection(version string) map[string]any {
	return map[string]any{"dependencies": map[string]any{"tools": map[string]any{"terraform": version}}}
}

func TestComponentToolVersionsOverrides(t *testing.T) {
	for _, scope := range []string{"global", "type", "component", "sections"} {
		t.Run(scope, func(t *testing.T) {
			const manifest = "# Project default is not installed.\nhashicorp/terraform 1.15.9\n"
			config, root := defaultsFixture(t, manifest)
			expected := installedDefaultBinary(t, config, "hashicorp", "terraform", "1.9.0")
			stack := map[string]any{}
			component := map[string]any{}
			switch scope {
			case "global":
				stack = dependencySection("1.9.0")
			case "type":
				stack = dependencySection("1.10.0")
				stack["terraform"] = dependencySection("1.9.0")
			case "component", "sections":
				stack = dependencySection("1.11.0")
				stack["terraform"] = dependencySection("1.10.0")
				component = dependencySection("1.9.0")
			}
			var env *ToolchainEnvironment
			var err error
			if scope == "sections" {
				env, err = ForSections(config, component)
			} else {
				env, err = ForComponent(config, "terraform", stack, component)
			}
			require.NoError(t, err)
			assert.Equal(t, expected, env.Resolve("terraform"))
			assert.Equal(t, []string{filepath.Dir(expected)}, env.ToolchainDirs())
			after, err := os.ReadFile(filepath.Join(root, ".tool-versions"))
			require.NoError(t, err)
			assert.Equal(t, manifest, string(after))
			require.NoDirExists(t, filepath.Join(config.Toolchain.InstallPath, "bin", "hashicorp", "terraform", "1.15.9"))
		})
	}
}

func TestComponentToolVersionsManifest(t *testing.T) {
	for _, entrypoint := range []string{"component", "sections"} {
		for _, scenario := range []string{"missing", "empty", "malformed", "unreadable", "custom path"} {
			t.Run(entrypoint+"/"+scenario, func(t *testing.T) {
				config, root := defaultsFixture(t, "")
				manifest := filepath.Join(root, ".tool-versions")
				expected := ""
				switch scenario {
				case "missing":
					require.NoError(t, os.Remove(manifest))
				case "malformed":
					require.NoError(t, os.WriteFile(manifest, []byte("terraform\n"), 0o600))
				case "unreadable":
					require.NoError(t, os.Remove(manifest))
					require.NoError(t, os.Mkdir(manifest, 0o755))
				case "custom path":
					config.Toolchain.VersionsFile = "project.tools"
					require.NoError(t, os.WriteFile(filepath.Join(root, "project.tools"), []byte("hashicorp/terraform 1.9.0\n"), 0o600))
					expected = installedDefaultBinary(t, config, "hashicorp", "terraform", "1.9.0")
				}
				var env *ToolchainEnvironment
				var err error
				if entrypoint == "component" {
					env, err = ForComponent(config, "terraform", nil, nil)
				} else {
					env, err = ForSections(config, nil)
				}
				if scenario == "malformed" || scenario == "unreadable" {
					require.ErrorContains(t, err, "failed to load .tool-versions")
					assert.Nil(t, env)
					return
				}
				require.NoError(t, err)
				if expected != "" {
					assert.Equal(t, expected, env.Resolve("terraform"))
					return
				}
				assert.Empty(t, env.PATH())
				assert.Nil(t, env.EnvVars())
				// A known executable on PATH still resolves without dependencies.
				t.Setenv("PATH", filepath.Dir(os.Args[0]))
				assert.Equal(t, os.Args[0], env.Resolve(filepath.Base(os.Args[0])))
			})
		}
	}
}

func TestExplicitEnvironmentDoesNotLoadDefaults(t *testing.T) {
	config, _ := defaultsFixture(t, "invalid-manifest\n")
	env, err := NewEnvironmentFromDeps(config, nil)
	require.NoError(t, err)
	assert.Empty(t, env.PATH())
}

func TestWorkflowToolVersionsAliasOverrides(t *testing.T) {
	for _, tc := range []struct{ name, defaultTool, explicitTool, version string }{
		{"short name overrides qualified", "hashicorp/terraform", "terraform", "1.9.0"},
		{"qualified overrides short name", "terraform", "hashicorp/terraform", "1.9.0"},
		{"configured alias", "hashicorp/terraform", "tf", "1.9.0"},
		{"constraint overrides pin", "hashicorp/terraform", "terraform", "~>1.9.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, root := defaultsFixture(t, tc.defaultTool+" 1.15.9\njqlang/jq 1.7.1\n")
			resolver := &toolchain.DefaultToolResolver{AtmosConfig: config}
			mockProv := NewMockToolProvisioner(gomock.NewController(t))
			expected := map[string]string{tc.explicitTool: tc.version, "jqlang/jq": "1.7.1"}
			mockProv.EXPECT().EnsureTools(expected).DoAndReturn(func(deps map[string]string) error {
				// Model installer constraint resolution without mutating workflow declarations.
				deps[tc.explicitTool] = "1.9.0"
				return nil
			})
			mockProv.EXPECT().ResolveToolName(gomock.Any()).DoAndReturn(resolver.Resolve).AnyTimes()
			tf := filepath.Join(root, "bin", "terraform")
			jq := filepath.Join(root, "bin", "jq")
			mockProv.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.9.0").Return(tf, nil)
			mockProv.EXPECT().FindBinaryPath("jqlang", "jq", "1.7.1").Return(jq, nil)
			mockProv.EXPECT().BuildPATH(config, map[string]string{tc.explicitTool: "1.9.0", "jqlang/jq": "1.7.1"}).Return(filepath.Dir(tf), nil)
			workflow := &schema.WorkflowDefinition{Dependencies: &schema.Dependencies{Tools: map[string]string{tc.explicitTool: tc.version}}}
			env, err := ForWorkflow(config, workflow, withProvisioner(mockProv))
			require.NoError(t, err)
			assert.Equal(t, tf, env.Resolve("terraform"))
			assert.Equal(t, tf, env.Resolve(tc.explicitTool))
			assert.Equal(t, jq, env.Resolve("jq"))
			assert.Equal(t, tc.version, workflow.Dependencies.Tools[tc.explicitTool])
		})
	}
}
