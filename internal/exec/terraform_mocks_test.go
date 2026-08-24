package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func TestTerraformComponentMocksResolveStateAndOutput(t *testing.T) {
	sandbox, err := testhelpers.SetupSandbox(t, "../../tests/fixtures/scenarios/terraform-component-mocks")
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)
	t.Chdir(sandbox.OriginalWorkdir)
	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "app",
		ComponentType:    cfg.TerraformComponentType,
		Stack:            "dev",
		UseMocks:         true,
	}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)

	stackInfo := &schema.ConfigAndStacksInfo{UseMocks: true}
	state, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc vpc_id", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, "vpc-local", state)

	output, err := processTagTerraformOutput(&atmosConfig, "!terraform.output vpc '.network.cidr'", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.0/16", output)

	list, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc private_subnet_ids", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, []any{"subnet-a", "subnet-b"}, list)

	literal, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc literal_template", "dev", stackInfo)
	require.NoError(t, err)
	assert.Equal(t, "{{ .Environment }}", literal, "mock values must not be template-evaluated")

	nullable, err := processTagTerraformState(&atmosConfig, "!terraform.state vpc nullable_output", "dev", stackInfo)
	require.NoError(t, err)
	assert.Nil(t, nullable)
}

func TestTerraformComponentMocksFailClosed(t *testing.T) {
	sandbox, err := testhelpers.SetupSandbox(t, "../../tests/fixtures/scenarios/terraform-component-mocks")
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)
	t.Chdir(sandbox.OriginalWorkdir)
	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{Stack: "dev"}, true)
	require.NoError(t, err)

	_, err = processTagTerraformState(&atmosConfig, "!terraform.state app missing", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not declare `mocks`")

	_, err = processTagTerraformOutput(&atmosConfig, "!terraform.output vpc missing", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not declared")

	_, err = processTagTerraformOutput(&atmosConfig, "!terraform.output vpc '.network.missing'", "dev", &schema.ConfigAndStacksInfo{UseMocks: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not declared")
}
