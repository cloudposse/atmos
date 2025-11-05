//nolint:dupl // Similar test structure to other builder tests, but testing different domain-specific builder
package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListSettingsPositionalArgsBuilder_WithComponent_Required(t *testing.T) {
	builder := NewListSettingsPositionalArgsBuilder()
	builder.WithComponent(true)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Component name to filter settings", specs[0].Description)

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

func TestListSettingsPositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
	builder := NewListSettingsPositionalArgsBuilder()
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

func TestListSettingsPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining.
	builder := NewListSettingsPositionalArgsBuilder()
	result := builder.WithComponent(true)

	assert.Equal(t, builder, result, "WithComponent should return builder for chaining")
}

func TestListSettingsPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in list settings command.
	_, listAllValidator, listAllUsage := NewListSettingsPositionalArgsBuilder().
		WithComponent(false).
		Build()

	// Check usage matches expected pattern for list all settings.
	assert.Equal(t, "[component]", listAllUsage)

	// Test validator with real list settings scenarios.
	err := listAllValidator(nil, []string{})
	assert.NoError(t, err, "should accept: atmos list settings")

	err = listAllValidator(nil, []string{"vpc"})
	assert.NoError(t, err, "should accept: atmos list settings vpc")

	err = listAllValidator(nil, []string{"vpc", "ecs"})
	assert.Error(t, err, "should reject: atmos list settings vpc ecs (too many args)")
}

func TestListSettingsPositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command.
	_, validator, usage := NewListSettingsPositionalArgsBuilder().
		WithComponent(false).
		Build()

	// Simulate command setup.
	cmdUse := "settings " + usage
	assert.Equal(t, "settings [component]", cmdUse)

	// Validate args work correctly.
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{"vpc", "extra"})
	assert.Error(t, err)
}
