package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

func TestApplyFlags(t *testing.T) {
	registry := ApplyFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "ApplyFlags should return non-nil registry")
}

func TestApplyPositionalArgs(t *testing.T) {
	builder := ApplyPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "ApplyPositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "ApplyPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "ApplyPositionalArgs usage should be <component>")
}

func TestDestroyFlags(t *testing.T) {
	registry := DestroyFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "DestroyFlags should return non-nil registry")
}

func TestDestroyPositionalArgs(t *testing.T) {
	builder := DestroyPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "DestroyPositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "DestroyPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "DestroyPositionalArgs usage should be <component>")
}

func TestInitFlags(t *testing.T) {
	registry := InitFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "InitFlags should return non-nil registry")
}

func TestInitPositionalArgs(t *testing.T) {
	builder := InitPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "InitPositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "InitPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "InitPositionalArgs usage should be <component>")
}

func TestOutputFlags(t *testing.T) {
	registry := OutputFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "OutputFlags should return non-nil registry")
}

func TestOutputPositionalArgs(t *testing.T) {
	builder := OutputPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "OutputPositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "OutputPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "OutputPositionalArgs usage should be <component>")
}

func TestPlanFlags(t *testing.T) {
	registry := PlanFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "PlanFlags should return non-nil registry")
}

func TestPlanPositionalArgs(t *testing.T) {
	builder := PlanPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "PlanPositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "PlanPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "PlanPositionalArgs usage should be <component>")
}

func TestValidateFlags(t *testing.T) {
	registry := ValidateFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "ValidateFlags should return non-nil registry")
}

func TestValidatePositionalArgs(t *testing.T) {
	builder := ValidatePositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "ValidatePositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "ValidatePositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "ValidatePositionalArgs usage should be <component>")
}

func TestRefreshCompatibilityAliases(t *testing.T) {
	aliases := RefreshCompatibilityAliases()

	require.NotNil(t, aliases)
	assert.NotEmpty(t, aliases, "RefreshCompatibilityAliases should return non-empty map")
}

func TestUntaintCompatibilityAliases(t *testing.T) {
	aliases := UntaintCompatibilityAliases()

	require.NotNil(t, aliases)
	assert.NotEmpty(t, aliases, "UntaintCompatibilityAliases should return non-empty map")
}

func TestForceUnlockCompatibilityAliases(t *testing.T) {
	aliases := ForceUnlockCompatibilityAliases()

	require.NotNil(t, aliases)
	assert.NotEmpty(t, aliases, "ForceUnlockCompatibilityAliases should return non-empty map")
}

func TestRegisterCompatibilityAliases(t *testing.T) {
	// Create a test provider
	testProvider := func() map[string]flags.CompatibilityAlias {
		return map[string]flags.CompatibilityAlias{
			"-test": {
				Behavior: flags.MapToAtmosFlag,
				Target:   "--test",
			},
		}
	}

	// Register the provider
	RegisterCompatibilityAliases("test-command", testProvider)

	// Verify registration by checking CompatibilityAliases returns the registered aliases
	aliases := CompatibilityAliases("test-command")
	require.NotNil(t, aliases)
	assert.NotEmpty(t, aliases, "RegisterCompatibilityAliases should register aliases for test-command")
	assert.Contains(t, aliases, "-test", "Should contain the registered test alias")
}
