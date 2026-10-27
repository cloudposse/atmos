package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformSecretVars_NoSecrets(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{"region": "us-west-2-nosecret-xyz", "count": 1},
	}
	ComputeTerraformSecretVarKeys(info)
	assert.Nil(t, info.TerraformSecretVarKeys, "no secret keys expected when no secrets present")

	env, err := SecretVarEnv(info)
	require.NoError(t, err)
	assert.Empty(t, env, "no TF_VAR_ entries expected when no secrets present")
}

func TestRejectComputedTerraformVars(t *testing.T) {
	assert.NoError(t, RejectComputedTerraformVars(map[string]any{"region": "us-east-1"}))
	err := RejectComputedTerraformVars(map[string]any{"value": degradation.AtmosComputedValue{}})
	assert.ErrorIs(t, err, errUtils.ErrUnresolvedComputedTerraformVar)
	err = RejectComputedTerraformVars(map[string]any{
		"nested": []any{map[string]any{"value": degradation.AtmosComputedValue{}}},
	})
	assert.ErrorIs(t, err, errUtils.ErrUnresolvedComputedTerraformVar)
}
