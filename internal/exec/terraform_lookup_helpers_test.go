package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// malformedYqDefault contains the `//` default operator but does not parse, so the YQ default
// evaluation itself fails.
const malformedYqDefault = `.missing // (`

func TestTerraformLookupErrorHandlersWithMalformedYqDefault(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	lookup := &terraformStateLookup{yamlFunc: "!terraform.output vpc " + malformedYqDefault, stack: "dev", component: "vpc", output: malformedYqDefault}
	notProvisioned := fmt.Errorf("not yet: %w", errUtils.ErrTerraformStateNotProvisioned)

	t.Run("output error keeps the original recoverable error", func(t *testing.T) {
		value, err := handleTerraformOutputError(atmosConfig, lookup, notProvisioned)
		require.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
		assert.Nil(t, value)
	})

	t.Run("missing output stays nil", func(t *testing.T) {
		value, err := handleMissingTerraformOutput(atmosConfig, lookup)
		require.NoError(t, err)
		assert.Nil(t, value)
	})

	t.Run("state error wraps the original recoverable error", func(t *testing.T) {
		value, err := handleTerraformStateError(atmosConfig, lookup, notProvisioned)
		require.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
		assert.Contains(t, err.Error(), "failed to evaluate YQ default")
		assert.Nil(t, value)
	})
}

func TestResolveTerraformOutputWithMocksMalformedExpression(t *testing.T) {
	lookup := &terraformStateLookup{stack: "dev", component: "vpc", output: ".vpc_id["}

	value, err := resolveTerraformOutputWithMocks(&schema.AtmosConfiguration{}, lookup, map[string]any{"vpc_id": "vpc-mock"}, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "against component mocks")
	assert.Nil(t, value)
}

func TestTerraformMocksModeGuards(t *testing.T) {
	assert.Equal(t, schema.TerraformMocksModeFallback, terraformMocksMode(nil))

	invalid := &schema.AtmosConfiguration{}
	invalid.Components.Terraform.Mocks.Mode = "bogus"
	require.NoError(t, validateTerraformMocksMode(invalid, nil), "a nil stack info means mocks are off")
}

func TestTerraformLookupAuth(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	authContext, authManager := terraformLookupAuth(atmosConfig, nil)
	assert.Nil(t, authContext)
	assert.Nil(t, authManager)

	// AuthDisabled without an AuthManager still propagates downstream through the wrapper.
	stackInfo := &schema.ConfigAndStacksInfo{AuthDisabled: true}
	_, authManager = terraformLookupAuth(atmosConfig, stackInfo)
	wrapper, ok := authManager.(*authContextWrapper)
	require.True(t, ok)
	assert.Same(t, stackInfo, wrapper.stackInfo)

	// Auth enabled with no manager passes nothing through.
	_, authManager = terraformLookupAuth(atmosConfig, &schema.ConfigAndStacksInfo{})
	assert.Nil(t, authManager)
}
