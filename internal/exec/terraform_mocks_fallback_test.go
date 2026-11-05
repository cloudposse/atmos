package exec

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

const mocksFallbackFixture = "../../tests/fixtures/scenarios/terraform-component-mocks-fallback"

// requireTerraformCommandForMocks skips the test when neither tofu nor terraform is installed and
// points components.terraform.command at whichever one is found.
func requireTerraformCommandForMocks(t *testing.T) {
	t.Helper()

	for _, command := range []string{"tofu", "terraform"} {
		if _, err := exec.LookPath(command); err == nil {
			t.Setenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND", command)
			return
		}
	}
	t.Skip("skipping: neither 'tofu' nor 'terraform' binary found in PATH (required for the real-state mocks test)")
}

// resetTerraformLookupCaches clears the state and output caches now and after the test, so lookups
// before and after an apply in the same process never see each other's results.
func resetTerraformLookupCaches(t *testing.T) {
	t.Helper()

	ResetStateCache()
	tfoutput.ResetOutputsCache()
	t.Cleanup(func() {
		ResetStateCache()
		tfoutput.ResetOutputsCache()
	})
}

// mocksFallbackConfig loads the fixture's configuration with components.terraform.mocks.mode set.
func mocksFallbackConfig(t *testing.T, mode schema.TerraformMocksMode) *schema.AtmosConfiguration {
	t.Helper()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{Stack: "dev"}, true)
	require.NoError(t, err)
	atmosConfig.Components.Terraform.Mocks.Mode = mode
	return &atmosConfig
}

// TestTerraformComponentMocksFallbackRealState runs `--use-mocks` lookups against real local
// Terraform state: first with nothing applied, then after applying the producer. It covers what
// the gomock-based tests cannot: the real state reader, the real `terraform output` runner, and
// how Terraform itself records (or omits) outputs.
func TestTerraformComponentMocksFallbackRealState(t *testing.T) {
	requireTerraformCommandForMocks(t)
	resetTerraformLookupCaches(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")
	setupTerraformYamlFunctionSandbox(t, mocksFallbackFixture)
	t.Chdir(mocksFallbackFixture)

	mocksOn := &schema.ConfigAndStacksInfo{UseMocks: true}
	fallback := mocksFallbackConfig(t, schema.TerraformMocksModeFallback)

	state := func(t *testing.T, atmosConfig *schema.AtmosConfiguration, expr string) any {
		t.Helper()
		value, err := processTagTerraformState(atmosConfig, "!terraform.state "+expr, "dev", mocksOn)
		require.NoError(t, err, expr)
		return value
	}
	output := func(t *testing.T, atmosConfig *schema.AtmosConfiguration, expr string) any {
		t.Helper()
		value, err := processTagTerraformOutput(atmosConfig, "!terraform.output "+expr, "dev", mocksOn)
		require.NoError(t, err, expr)
		return value
	}

	t.Run("not provisioned", func(t *testing.T) {
		assert.Equal(t, "vpc-local", state(t, fallback, "vpc vpc_id"))
		assert.Equal(t, "never-mock", state(t, fallback, "never_applied id"))
		assert.Equal(t, "never-mock", output(t, fallback, "never_applied id"))

		// A producer without mocks keeps the normal not-provisioned error in fallback mode.
		_, err := processTagTerraformState(fallback, "!terraform.state nomocks id", "dev", mocksOn)
		require.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
	})

	err := ExecuteTerraform(schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentType:    "terraform",
		ComponentFromArg: "vpc",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	})
	require.NoError(t, err)
	// The apply invalidates the producer's cached "not provisioned" entry; clear the rest too.
	ResetStateCache()
	tfoutput.ResetOutputsCache()

	t.Run("provisioned, fallback", func(t *testing.T) {
		assert.Equal(t, "vpc-real", state(t, fallback, "vpc vpc_id"))
		assert.Equal(t, "vpc-real", output(t, fallback, "vpc vpc_id"))
		assert.Equal(t, "vpc-real", state(t, fallback, `vpc '.vpc_id // "dflt"'`))
		assert.Equal(t, false, state(t, fallback, "vpc enabled"))

		// The mock fills a key missing from the real map output; the real key wins.
		assert.Equal(t, 2, state(t, fallback, "vpc .config.b"))
		assert.Equal(t, 1, state(t, fallback, "vpc .config.a"))

		// An output declared only in mocks resolves from the mock.
		assert.Equal(t, []any{"subnet-mock-1"}, state(t, fallback, "vpc subnet_ids"))

		// Terraform does not record null outputs, so the real null is indistinguishable from a
		// missing output and the mock fills it (documented limitation).
		assert.Equal(t, "mock-not-null", state(t, fallback, "vpc nullable"))

		// Missing from both real state and mocks: nil, the same as without mocks.
		assert.Nil(t, state(t, fallback, "vpc does_not_exist"))
	})

	t.Run("provisioned, always", func(t *testing.T) {
		always := mocksFallbackConfig(t, schema.TerraformMocksModeAlways)

		assert.Equal(t, "vpc-local", state(t, always, "vpc vpc_id"))
		assert.Equal(t, "vpc-local", output(t, always, "vpc vpc_id"))
		assert.Equal(t, 0, state(t, always, "vpc .config.a"))
	})

	t.Run("provisioned, mocks off", func(t *testing.T) {
		value, err := processTagTerraformState(fallback, "!terraform.state vpc subnet_ids", "dev", &schema.ConfigAndStacksInfo{})
		require.NoError(t, err)
		assert.Nil(t, value)
	})
}
