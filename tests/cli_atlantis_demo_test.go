package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

type atlantisDemoStage struct {
	Steps []struct{ Run string }
}

func TestCLIAtlantisDemo(t *testing.T) {
	ensureAtmosRunner(t)
	projectDir := t.TempDir()
	require.NoError(t, os.CopyFS(projectDir, os.DirFS("../examples/demo-atlantis")))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", projectDir)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_BASE_PATH", "")
	cmd := atmosRunner.CommandContext(t.Context(), "atlantis", "build-all")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)

	var config struct {
		Version   int
		Projects  []schema.AtlantisProjectConfig
		Workflows map[string]struct{ Plan, Apply atlantisDemoStage }
	}
	repoConfig, err := os.ReadFile(filepath.Join(projectDir, "atlantis.yaml"))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(repoConfig, &config))
	require.Equal(t, 3, config.Version)
	require.Len(t, config.Projects, 1)
	project := config.Projects[0]
	require.Equal(t, "demo-hello", project.Name)
	require.Equal(t, "default", project.Workspace)
	require.Equal(t, "components/terraform/hello", project.Dir)
	require.Equal(t, []string{"approved"}, project.ApplyRequirements)
	require.True(t, project.Autoplan.Enabled)
	workflow, ok := config.Workflows[project.Workflow]
	require.True(t, ok, "project %s references workflow %q", project.Name, project.Workflow)
	require.NotEmpty(t, workflow.Plan.Steps)
	require.NotEmpty(t, workflow.Apply.Steps)

	componentDir := filepath.Join(projectDir, project.Dir)
	varfile := "varfiles/" + project.Name + ".tfvars.json"
	backend := "backends/" + project.Name + ".backend"
	for _, file := range []string{varfile, backend} {
		require.Contains(t, project.Autoplan.WhenModified, file)
		require.FileExists(t, filepath.Join(componentDir, file))
	}

	t.Run("reviewed local plan", func(t *testing.T) {
		terraform := RequireTerraformPath(t)
		shell := requireExecutablePath(t, "sh", "running the Atlantis demo workflow")
		t.Setenv("PATH", filepath.Dir(terraform)+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("PROJECT_NAME", project.Name)
		t.Setenv("WORKSPACE", project.Workspace)
		t.Setenv("PLANFILE", filepath.Join(projectDir, "reviewed plan"))
		t.Setenv("TF_WORKSPACE", "")
		t.Setenv("TF_DATA_DIR", "")

		run := func(stage atlantisDemoStage) {
			t.Helper()
			for _, step := range stage.Steps {
				cmd := exec.CommandContext(t.Context(), shell, "-c", step.Run)
				cmd.Dir = componentDir
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s\n%s", step.Run, out)
			}
		}
		run(workflow.Plan)
		require.FileExists(t, os.Getenv("PLANFILE"))
		// Changing the source after planning must not alter what apply deploys.
		require.NoError(t, os.WriteFile(filepath.Join(componentDir, varfile), []byte(`{"stage":"demo","greeting":"edited after review"}`), 0o600))
		run(workflow.Apply)
		state, err := os.ReadFile(filepath.Join(componentDir, "demo-hello.tfstate"))
		require.NoError(t, err)
		var result struct {
			Outputs map[string]struct{ Value string }
		}
		require.NoError(t, json.Unmarshal(state, &result))
		require.Equal(t, "Hello from Atlantis (demo)", result.Outputs["message"].Value)
	})
}
