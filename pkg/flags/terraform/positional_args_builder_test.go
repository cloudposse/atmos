package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformPositionalArgsBuilder_WithComponent_Required(t *testing.T) {
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
	err := validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)

	err = validator(nil, []string{"vpc", "rds"})
	assert.Error(t, err)
}

func TestTerraformPositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
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

	err = validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{"vpc", "rds"})
	assert.Error(t, err)
}

func TestTerraformPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining
	builder := NewPositionalArgsBuilder()
	result := builder.WithComponent(true)

	assert.Equal(t, builder, result, "WithComponent should return builder for chaining")
}

func TestTerraformPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in terraform deploy command
	_, deployValidator, deployUsage := NewPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Check usage matches expected pattern for deploy command
	assert.Equal(t, "<component>", deployUsage)

	// Test validator with real terraform deploy scenarios
	err := deployValidator(nil, []string{"vpc"})
	assert.NoError(t, err, "should accept: atmos terraform deploy vpc")

	err = deployValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos terraform deploy (missing component)")
}
