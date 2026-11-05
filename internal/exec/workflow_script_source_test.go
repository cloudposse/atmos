package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

const scriptSourceWorkflow = `
workflows:
  deploy:
    steps:
      - name: main
        type: script
        interpreter: starlark
        script: !include scripts/main.star
      - name: local
        type: script
        interpreter: starlark
        script: !include ./local.star
`

func setupScriptSourceProject(t *testing.T) (root, workflowPath string) {
	t.Helper()
	root = setupTestWorkflowEnvironment(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	workflowsDir := filepath.Join(root, "stacks", "workflows")
	createTestWorkflowFile(t, workflowsDir, "deploy.yaml", scriptSourceWorkflow)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "scripts"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "scripts", "main.star"), []byte("print(\"main\")\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, "local.star"), []byte("print(\"local\")\n"), 0o600))
	return root, filepath.Join(workflowsDir, "deploy.yaml")
}

func TestLoadWorkflowConfig_IncludesResolveFromAnyWorkingDirectory(t *testing.T) {
	root, workflowPath := setupScriptSourceProject(t)
	config := initTestConfig(t)
	config.BasePath = root
	config.Workflows.BasePath = "stacks/workflows"
	require.NoError(t, cfg.AtmosConfigAbsolutePaths(&config))

	// A decoy next to the process working directory must not be read for the bare path.
	elsewhere := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(elsewhere, "scripts"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(elsewhere, "scripts", "main.star"), []byte("decoy"), 0o600))

	for _, dir := range []string{root, filepath.Join(root, "stacks"), elsewhere} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Chdir(dir)
			workflows, err := LoadWorkflowConfig(&config, workflowPath)
			require.NoError(t, err)
			steps := workflows["deploy"].Steps
			require.Len(t, steps, 2)
			assert.Equal(t, "print(\"main\")\n", steps[0].Script)
			assert.Equal(t, filepath.Join(config.BasePathAbsolute, "scripts", "main.star"), steps[0].ScriptSource)
			assert.Equal(t, "print(\"local\")\n", steps[1].Script)
			assert.Equal(t, filepath.Join(filepath.Dir(workflowPath), "local.star"), steps[1].ScriptSource)
		})
	}
}

func TestExecuteDescribeWorkflows_IncludesResolveFromAnyWorkingDirectory(t *testing.T) {
	root, _ := setupScriptSourceProject(t)
	config := initTestConfig(t)
	config.BasePath = root
	config.Workflows.BasePath = "stacks/workflows"
	require.NoError(t, cfg.AtmosConfigAbsolutePaths(&config))
	t.Chdir(t.TempDir())

	_, _, manifests, err := ExecuteDescribeWorkflows(config)

	require.NoError(t, err)
	var found bool
	for _, manifest := range manifests {
		if deploy, ok := manifest.Workflows["deploy"]; ok {
			found = true
			assert.Equal(t, "print(\"main\")\n", deploy.Steps[0].Script)
		}
	}
	assert.True(t, found, "the manifest with the includes must be listed, not skipped")
}
