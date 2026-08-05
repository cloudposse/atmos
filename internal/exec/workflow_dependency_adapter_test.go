package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteWorkflow_DependenciesWorkflowsSameFile verifies a workflow's dependencies.workflows
// entry with no `file:` resolves against the SAME file the running workflow itself came from,
// and runs before the dependent's own steps.
func TestExecuteWorkflow_DependenciesWorkflowsSameFile(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	atmosConfig.BasePath = tmpDir
	atmosConfig.Workflows.BasePath = ""

	buildLog := filepath.Join(tmpDir, "build.txt")
	deployLog := filepath.Join(tmpDir, "deploy.txt")

	manifest := `
workflows:
  build:
    steps:
      - command: "echo build >> ` + buildLog + `"
        type: shell
  deploy:
    dependencies:
      workflows:
        - build
    steps:
      - command: "echo deploy >> ` + deployLog + `"
        type: shell
`
	workflowPath := filepath.Join(tmpDir, "same-file.yaml")
	require.NoError(t, os.WriteFile(workflowPath, []byte(manifest), 0o644))

	workflowConfig, err := LoadWorkflowConfig(workflowPath)
	require.NoError(t, err)
	deployDef := workflowConfig["deploy"]

	err = ExecuteWorkflow(atmosConfig, "deploy", workflowPath, &deployDef, false, "", "", "")
	require.NoError(t, err)

	assert.FileExists(t, buildLog, "same-file dependency 'build' must run")
	assert.FileExists(t, deployLog, "deploy's own step must still run after its dependency completes")
}

// TestExecuteWorkflow_DependenciesWorkflowsCrossFile verifies a dependencies.workflows entry
// with an explicit `file:` resolves against that OTHER file, the same way the CLI's
// `-f`/`--file` flag resolves a workflow file.
func TestExecuteWorkflow_DependenciesWorkflowsCrossFile(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	atmosConfig.BasePath = tmpDir
	atmosConfig.Workflows.BasePath = ""

	buildLog := filepath.Join(tmpDir, "build.txt")
	deployLog := filepath.Join(tmpDir, "deploy.txt")

	buildManifest := `
workflows:
  build:
    steps:
      - command: "echo build >> ` + buildLog + `"
        type: shell
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "build.yaml"), []byte(buildManifest), 0o644))

	deployManifest := `
workflows:
  deploy:
    dependencies:
      workflows:
        - name: build
          file: build.yaml
    steps:
      - command: "echo deploy >> ` + deployLog + `"
        type: shell
`
	deployPath := filepath.Join(tmpDir, "deploy.yaml")
	require.NoError(t, os.WriteFile(deployPath, []byte(deployManifest), 0o644))

	workflowConfig, err := LoadWorkflowConfig(deployPath)
	require.NoError(t, err)
	deployDef := workflowConfig["deploy"]

	err = ExecuteWorkflow(atmosConfig, "deploy", deployPath, &deployDef, false, "", "", "")
	require.NoError(t, err)

	assert.FileExists(t, buildLog, "cross-file dependency 'build' (resolved via file: build.yaml) must run")
	assert.FileExists(t, deployLog, "deploy's own step must still run after its cross-file dependency completes")
}
