package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNativeCIFlociSurvivesRunnerHardening(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "native-ci.yml"))
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]struct {
			Services map[string]struct {
				Options string `yaml:"options"`
			} `yaml:"services"`
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	for _, jobName := range []string{"terraform-plan", "terraform-apply"} {
		t.Run(jobName, func(t *testing.T) {
			job, ok := workflow.Jobs[jobName]
			require.True(t, ok)
			floci, ok := job.Services["floci"]
			require.True(t, ok)
			require.Contains(t, floci.Options, "--restart=always", "runner hardening can restart Docker")
			var names []string
			for _, step := range job.Steps {
				names = append(names, step.Name)
			}
			harden := slices.Index(names, "Harden Runner")
			wait := slices.Index(names, "Wait for Floci")
			mirror := slices.Index(names, "Mirror Terraform providers")
			require.NotEqual(t, -1, harden)
			require.Greater(t, wait, harden, "check readiness after Docker may have restarted")
			require.Greater(t, mirror, wait, "provider mirroring authenticates through Floci STS")
			probe := job.Steps[wait].Run
			require.Contains(t, probe, "http://127.0.0.1:4566/")
			require.Contains(t, probe, "--retry-connrefused")
			require.Contains(t, probe, "--max-time 5")
			require.Contains(t, probe, "--retry-max-time 60")
		})
	}
}

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
