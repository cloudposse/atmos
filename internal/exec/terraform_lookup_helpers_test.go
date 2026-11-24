package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	terraformcomponent "github.com/cloudposse/atmos/pkg/component/terraform"
	"github.com/cloudposse/atmos/pkg/schema"
)

// malformedYqDefault contains the `//` default operator but does not parse, so the YQ default
// evaluation itself fails.
const malformedYqDefault = `.missing // (`

func TestTerraformLookupErrorHandlersWithMalformedYqDefault(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	lookup := &terraformcomponent.TerraformStateLookup{YamlFunc: "!terraform.output vpc " + malformedYqDefault, Stack: "dev", Component: "vpc", Output: malformedYqDefault}
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
	lookup := &terraformcomponent.TerraformStateLookup{Stack: "dev", Component: "vpc", Output: ".vpc_id["}

	value, err := terraformExecutor.ResolveTerraformOutputWithMocks(&schema.AtmosConfiguration{}, lookup, map[string]any{"vpc_id": "vpc-mock"}, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "against component mocks")
	assert.Nil(t, value)
}

func TestTerraformLookupAuth(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	authContext, authManager := terraformLookupAuth(atmosConfig, nil)
	assert.Nil(t, authContext)
	assert.Nil(t, authManager)

	// AuthDisabled without an AuthManager still propagates downstream through the wrapper.
	stackInfo := &schema.ConfigAndStacksInfo{AuthDisabled: true}
	_, authManager = terraformLookupAuth(atmosConfig, stackInfo)
	wrapper, ok := authManager.(*terraformcomponent.AuthContextWrapper)
	require.True(t, ok)
	assert.Same(t, stackInfo, wrapper.StackInfo)

	// Auth enabled with no manager passes nothing through.
	_, authManager = terraformLookupAuth(atmosConfig, &schema.ConfigAndStacksInfo{})
	assert.Nil(t, authManager)
}
