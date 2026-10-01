package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeMinimalStackFixture writes a minimal atmos.yaml plus a single stack
// manifest into the current directory (expected to already be a t.TempDir()),
// enough for ProcessStacks to resolve component config without needing real
// component/terraform files.
func writeMinimalStackFixture(t *testing.T, atmosYAML, stackYAML string) {
	t.Helper()

	require.NoError(t, os.WriteFile("atmos.yaml", []byte(atmosYAML), 0o644))
	require.NoError(t, os.MkdirAll("stacks", 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join("components", "terraform", "mock"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("stacks", "dev.yaml"), []byte(stackYAML), 0o644))
}

// TestProcessStacksPropagatesSpaceliftStackNameBuildError verifies that
// ProcessStacks surfaces an error when BuildSpaceliftStackNameFromComponentConfig
// fails, instead of silently ignoring it.
//
// The fixture needs a `name_pattern` (so the Directory-branch ContextPrefix
// computation, which only consults the pattern, succeeds) and an `http` backend
// (so BuildTerraformWorkspace short-circuits on `isWorkspacesEnabled` before its
// own, separate `NameTemplate` check). `NameTemplate` is poisoned only after
// InitCliConfig, so it never affects the stack file discovery that InitCliConfig
// itself performs — only BuildSpaceliftStackNameFromComponentConfig's later,
// independent `ProcessTmpl` call sees the malformed value.
func TestProcessStacksPropagatesSpaceliftStackNameBuildError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	require.NoError(t, os.WriteFile("atmos.yaml", []byte(`
base_path: "./"
stacks:
  base_path: stacks
  included_paths:
    - "**/*"
  name_pattern: "{stage}"
components:
  terraform:
    base_path: components/terraform
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join("stacks", "dev"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join("components", "terraform", "mock"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("stacks", "dev", "main.yaml"), []byte(`
vars:
  stage: dev
components:
  terraform:
    mock:
      backend_type: http
      settings:
        spacelift:
          workspace_enabled: true
      vars:
        stage: dev
`), 0o644))

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "mock",
		Stack:            "dev/main",
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)
	require.Equal(t, "Directory", atmosConfig.StackType)

	atmosConfig.Stacks.NameTemplate = "{{ .vars.broken"

	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)

	_, err = ProcessStacks(&atmosConfig, info, true, true, false, nil, nil)
	require.ErrorContains(t, err, "unclosed action")
}

// TestProcessStacksPropagatesAtlantisProjectNameBuildError verifies that
// ProcessStacks surfaces an error when BuildAtlantisProjectNameFromComponentConfig
// fails to decode `settings.atlantis.project_template` (a string where a bool
// field is expected), instead of silently ignoring it.
func TestProcessStacksPropagatesAtlantisProjectNameBuildError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	writeMinimalStackFixture(t, `
base_path: "./"
stacks:
  base_path: stacks
  included_paths:
    - "**/*"
components:
  terraform:
    base_path: components/terraform
`, `
vars:
  stage: dev
components:
  terraform:
    mock:
      settings:
        atlantis:
          project_template:
            name: "mock-project"
            delete_source_branch_on_merge: "not-a-bool"
      vars:
        stage: dev
`)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "mock",
		Stack:            "dev",
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)

	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)

	_, err = ProcessStacks(&atmosConfig, info, true, true, false, nil, nil)
	require.ErrorContains(t, err, "delete_source_branch_on_merge")
}
