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
