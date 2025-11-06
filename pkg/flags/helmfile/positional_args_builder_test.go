//nolint:dupl // Similar test structure to other builder tests, but testing different domain-specific builder
package helmfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHelmfilePositionalArgsBuilder_WithComponent_Required(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	builder.WithComponent(true)

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Component name", specs[0].Description)

	// Check usage string
	assert.Equal(t, "<component>", usage)

	// Check validator requires exactly 1 arg
	err := validator(nil, []string{"nginx"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)

	err = validator(nil, []string{"nginx", "redis"})
	assert.Error(t, err)
}

func TestHelmfilePositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	builder.WithComponent(false)

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.False(t, specs[0].Required)

	// Check usage string
	assert.Equal(t, "[component]", usage)

	// Check validator accepts 0 or 1 args
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"nginx"})
	assert.NoError(t, err)

	err = validator(nil, []string{"nginx", "redis"})
	assert.Error(t, err)
}

func TestHelmfilePositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining
	builder := NewPositionalArgsBuilder()
	result := builder.WithComponent(true)

	assert.Equal(t, builder, result, "WithComponent should return builder for chaining")
}

func TestHelmfilePositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in helmfile apply command
	_, applyValidator, applyUsage := NewPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Check usage matches expected pattern for apply command
	assert.Equal(t, "<component>", applyUsage)

	// Test validator with real helmfile apply scenarios
	err := applyValidator(nil, []string{"nginx"})
	assert.NoError(t, err, "should accept: atmos helmfile apply nginx")

	err = applyValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos helmfile apply (missing component)")
}

func TestHelmfilePositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command
	_, validator, usage := NewPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Simulate command setup
	cmdUse := "apply " + usage
	assert.Equal(t, "apply <component>", cmdUse)

	// Validate args work correctly
	err := validator(nil, []string{"nginx"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)
}
