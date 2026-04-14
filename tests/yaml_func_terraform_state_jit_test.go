package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformStateJITWorkdir verifies that !terraform.state resolves correctly
// for components with provision.workdir.enabled: true.
//
// Before the fix, !terraform.state always looked in components/terraform/<component>/
// regardless of workdir configuration. JIT workdir components never populate that
// path, so !terraform.state always returned ErrTerraformStateNotProvisioned.
//
// Regression test for https://github.com/cloudposse/atmos/issues/2167.
func TestTerraformStateJITWorkdir(t *testing.T) {
	t.Chdir("./fixtures/scenarios/terraform-state-jit-workdir")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.True(t, atmosConfig.Initialized, "atmos config should be initialized")

	// consumer has: vars.foo: !terraform.state producer test foo
	// This resolves producer's state file from .workdir/terraform/test-producer/
	componentSection, err := e.ExecuteDescribeComponent(
		&e.ExecuteDescribeComponentParams{
			Component:            "consumer",
			Stack:                "test",
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		},
	)
	require.NoError(t, err, "!terraform.state should resolve for JIT workdir components (issue #2167)")
	require.NotNil(t, componentSection)

	vars, ok := componentSection["vars"].(map[string]interface{})
	require.True(t, ok, "vars should be a map")

	assert.Equal(t, "foo-from-jit-state", vars["foo"],
		"vars.foo should be resolved from pre-populated JIT state file")
}
