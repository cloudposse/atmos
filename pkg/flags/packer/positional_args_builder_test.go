//nolint:dupl // Similar test structure to other builder tests, but testing different domain-specific builder
package packer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackerPositionalArgsBuilder_WithComponent_Required(t *testing.T) {
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
	err := validator(nil, []string{"ami"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)

	err = validator(nil, []string{"ami", "docker"})
	assert.Error(t, err)
}

func TestPackerPositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
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

	err = validator(nil, []string{"ami"})
	assert.NoError(t, err)

	err = validator(nil, []string{"ami", "docker"})
	assert.Error(t, err)
}

func TestPackerPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining
	builder := NewPositionalArgsBuilder()
	result := builder.WithComponent(true)

	assert.Equal(t, builder, result, "WithComponent should return builder for chaining")
}

func TestPackerPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in packer build command
	_, buildValidator, buildUsage := NewPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Check usage matches expected pattern for build command
	assert.Equal(t, "<component>", buildUsage)

	// Test validator with real packer build scenarios
	err := buildValidator(nil, []string{"ami"})
	assert.NoError(t, err, "should accept: atmos packer build ami")

	err = buildValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos packer build (missing component)")
}

func TestPackerPositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command
	_, validator, usage := NewPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Simulate command setup
	cmdUse := "build " + usage
	assert.Equal(t, "build <component>", cmdUse)

	// Validate args work correctly
	err := validator(nil, []string{"ami"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)
}
