package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNativeCIE2ETflintHookRunsInPlanJob(t *testing.T) {
	path := filepath.Join(
		"fixtures",
		"scenarios",
		"native-ci-e2e",
		"stacks",
		"catalog",
		"bucket.yaml",
	)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var stack struct {
		Components struct {
			Terraform struct {
				Bucket struct {
					Hooks map[string]struct {
						Events []string `yaml:"events"`
					} `yaml:"hooks"`
				} `yaml:"bucket"`
			} `yaml:"terraform"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(data, &stack))

	hook, ok := stack.Components.Terraform.Bucket.Hooks["tflint-lint"]
	require.True(t, ok, "native CI fixture must include the tflint hook")
	require.Contains(t, hook.Events, "before.terraform.plan", "native-ci.yml runs terraform plan, not explicit terraform init")
}

func TestNativeCIToolchainInstallHasGHToken(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "native-ci.yml"))
	require.NoError(t, err)

	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))

	for _, jobName := range []string{"terraform-plan", "terraform-apply"} {
		job, ok := workflow.Jobs[jobName]
		require.Truef(t, ok, "native-ci.yml must define the %q job", jobName)

		var mirrorStep *struct {
			Name string            `yaml:"name"`
			Env  map[string]string `yaml:"env"`
		}
		for index := range job.Steps {
			if job.Steps[index].Name == "Mirror Terraform providers" {
				mirrorStep = &job.Steps[index]
				break
			}
		}

		require.NotNilf(t, mirrorStep, "native-ci.yml %q job must mirror Terraform providers", jobName)
		require.Equal(t, "${{ github.token }}", mirrorStep.Env["GH_TOKEN"])
	}
}
