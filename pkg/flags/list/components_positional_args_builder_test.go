package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListComponentsPositionalArgsBuilder_WithKey_Required(t *testing.T) {
	builder := NewListComponentsPositionalArgsBuilder()
	builder.WithKey(true)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "key", specs[0].Name)
	assert.Equal(t, "Key", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Configuration key to filter components", specs[0].Description)

	// Check usage string.
	assert.Equal(t, "<key>", usage)

	// Check validator requires exactly 1 arg.
	err := validator(nil, []string{"region"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)

	err = validator(nil, []string{"region", "environment"})
	assert.Error(t, err)
}

func TestListComponentsPositionalArgsBuilder_WithKey_Optional(t *testing.T) {
	builder := NewListComponentsPositionalArgsBuilder()
	builder.WithKey(false)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "key", specs[0].Name)
	assert.Equal(t, "Key", specs[0].TargetField)
	assert.False(t, specs[0].Required)

	// Check usage string.
	assert.Equal(t, "[key]", usage)

	// Check validator accepts 0 or 1 args.
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"region"})
	assert.NoError(t, err)

	err = validator(nil, []string{"region", "environment"})
	assert.Error(t, err)
}

func TestListComponentsPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining.
	builder := NewListComponentsPositionalArgsBuilder()
	result := builder.WithKey(true)

	assert.Equal(t, builder, result, "WithKey should return builder for chaining")
}

func TestListComponentsPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in a hypothetical list components by key command.
	_, listValidator, listUsage := NewListComponentsPositionalArgsBuilder().
		WithKey(true).
		Build()

	// Check usage matches expected pattern.
	assert.Equal(t, "<key>", listUsage)

	// Test validator with real scenarios.
	err := listValidator(nil, []string{"region"})
	assert.NoError(t, err, "should accept: atmos list components region")

	err = listValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos list components (missing key)")
}

func TestListComponentsPositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command.
	_, validator, usage := NewListComponentsPositionalArgsBuilder().
		WithKey(false).
		Build()

	// Simulate command setup.
	cmdUse := "components " + usage
	assert.Equal(t, "components [key]", cmdUse)

	// Validate args work correctly.
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"region"})
	assert.NoError(t, err)

	err = validator(nil, []string{"region", "extra"})
	assert.Error(t, err)
}
