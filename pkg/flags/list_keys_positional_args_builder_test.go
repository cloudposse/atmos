//nolint:dupl // Similar test structure to other builder tests, but testing different domain-specific builder
package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListKeysPositionalArgsBuilder_WithComponent_Required(t *testing.T) {
	builder := NewListKeysPositionalArgsBuilder()
	builder.WithComponent(true)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Component name to filter keys", specs[0].Description)

	// Check usage string.
	assert.Equal(t, "<component>", usage)

	// Check validator requires exactly 1 arg.
	err := validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)

	err = validator(nil, []string{"vpc", "ecs"})
	assert.Error(t, err)
}

func TestListKeysPositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
	builder := NewListKeysPositionalArgsBuilder()
	builder.WithComponent(false)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.False(t, specs[0].Required)

	// Check usage string.
	assert.Equal(t, "[component]", usage)

	// Check validator accepts 0 or 1 args.
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{"vpc", "ecs"})
	assert.Error(t, err)
}

func TestListKeysPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining.
	builder := NewListKeysPositionalArgsBuilder()
	result := builder.WithComponent(true)

	assert.Equal(t, builder, result, "WithComponent should return builder for chaining")
}

func TestListKeysPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in list values/vars command.
	_, listValidator, listUsage := NewListKeysPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Check usage matches expected pattern for list values command.
	assert.Equal(t, "<component>", listUsage)

	// Test validator with real list values scenarios.
	err := listValidator(nil, []string{"vpc"})
	assert.NoError(t, err, "should accept: atmos list values vpc")

	err = listValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos list values (missing component)")
}

func TestListKeysPositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command.
	_, validator, usage := NewListKeysPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Simulate command setup.
	cmdUse := "values " + usage
	assert.Equal(t, "values <component>", cmdUse)

	// Validate args work correctly.
	err := validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)
}
