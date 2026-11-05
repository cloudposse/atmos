package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// TestExecuteTerraformAll_RefreshesDependencyOutputs exercises real applies using
// only Terraform's built-in terraform_data resource and local state. Preflight
// caches the producer's outputs before applying it; the consumer must see the new
// outputs in the same invocation, both on first deployment and after an update.
// Regression coverage for the !terraform.output path in issue #2355.
func TestExecuteTerraformAll_RefreshesDependencyOutputs(t *testing.T) {
	t.Run("fresh deployment", func(t *testing.T) {
		testTerraformAllOutputCache(t, false)
	})
	t.Run("existing deployment", func(t *testing.T) {
		testTerraformAllOutputCache(t, true)
	})
}

func testTerraformAllOutputCache(t *testing.T, existing bool) {
	t.Helper()

	command, err := exec.LookPath("tofu")
	if err != nil {
		command, err = exec.LookPath("terraform")
		if err != nil {
			t.Skipf("requires Terraform or OpenTofu with the built-in terraform_data resource")
		}
	}

	ResetStateCache()
	tfoutput.ResetOutputsCache()
	t.Cleanup(func() {
		ResetStateCache()
		tfoutput.ResetOutputsCache()
	})

	root := t.TempDir()
	writeFile := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	writeFile("atmos.yaml", fmt.Sprintf(`base_path: "."
components:
  terraform:
    command: %q
    base_path: components/terraform
    auto_generate_backend_file: true
stacks:
  base_path: stacks
  included_paths: ["**/*.yaml"]
  name_template: '{{ .vars.stage }}'
settings:
  telemetry:
    enabled: false
`, command))
	writeFile("stacks/test.yaml", `vars:
  stage: test
components:
  terraform:
    producer:
      backend_type: local
    consumer:
      backend_type: local
      dependencies:
        components:
          - name: producer
      vars:
        vpc_id: !terraform.output producer vpc_id
`)
	producer := func(version string) string {
		return fmt.Sprintf(`variable "stage" { type = string }
resource "terraform_data" "vpc" { input = "network" }
output "vpc_id" { value = "${terraform_data.vpc.id}-%s" }
`, version)
	}
	writeFile("components/terraform/producer/main.tf", producer("v1"))
	writeFile("components/terraform/consumer/main.tf", `variable "stage" { type = string }
variable "vpc_id" { type = string }
resource "terraform_data" "cluster" {
  input = var.vpc_id
  triggers_replace = var.vpc_id
}
output "observed_vpc" { value = terraform_data.cluster.output }
output "cluster_id" { value = terraform_data.cluster.id }
`)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("TF_IN_AUTOMATION", "true")
	t.Setenv("TF_INPUT", "false")
	t.Chdir(root)

	if existing {
		// Seed a deployment with individual applies, as in issue #2355.
		for _, component := range []string{"producer", "consumer"} {
			require.NoError(t, ExecuteTerraform(schema.ConfigAndStacksInfo{
				Stack:                  "test",
				ComponentType:          "terraform",
				ComponentFromArg:       component,
				SubCommand:             "apply",
				AdditionalArgsAndFlags: []string{"-auto-approve"},
				ProcessTemplates:       true,
				ProcessFunctions:       true,
			}))
		}
	}

	applyAll := func() {
		t.Helper()
		require.NoError(t, ExecuteTerraformAll(&schema.ConfigAndStacksInfo{
			Stack:                  "test",
			ComponentType:          "terraform",
			SubCommand:             "apply",
			AdditionalArgsAndFlags: []string{"-auto-approve"},
			ProcessTemplates:       true,
			ProcessFunctions:       true,
		}))
	}
	readOutputs := func(component string) map[string]struct{ Value string } {
		t.Helper()
		cmd := exec.Command(command, "output", "-json")
		cmd.Dir = filepath.Join(root, "components", "terraform", component)
		data, err := cmd.Output()
		require.NoError(t, err)
		var outputs map[string]struct{ Value string }
		require.NoError(t, json.Unmarshal(data, &outputs))
		return outputs
	}
	assertCurrentOutputs := func() string {
		t.Helper()
		upstream := readOutputs("producer")
		downstream := readOutputs("consumer")
		require.NotEmpty(t, upstream["vpc_id"].Value)
		require.Equal(t, upstream["vpc_id"].Value, downstream["observed_vpc"].Value,
			"consumer must receive the current output in the same bulk apply")
		require.NotEmpty(t, downstream["cluster_id"].Value)
		return downstream["cluster_id"].Value
	}

	applyAll()
	initialID := assertCurrentOutputs()

	// Change only the producer. Do not clear caches between applies.
	writeFile("components/terraform/producer/main.tf", producer("v2"))
	applyAll()
	updatedID := assertCurrentOutputs()
	require.NotEqual(t, initialID, updatedID, "the changed dependency must replace the consumer immediately")

	applyAll()
	require.Equal(t, updatedID, assertCurrentOutputs(), "a repeat apply must not defer another replacement")
}
