//nolint:dupl // Similar test structure to other builder tests, but testing different domain-specific builder
package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescribeComponentPositionalArgsBuilder_WithComponent_Required(t *testing.T) {
	builder := NewComponentPositionalArgsBuilder()
	builder.WithComponent(true)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Component name", specs[0].Description)

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

func TestDescribeComponentPositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
	builder := NewComponentPositionalArgsBuilder()
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

func TestDescribeComponentPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining.
	builder := NewComponentPositionalArgsBuilder()
	result := builder.WithComponent(true)

	assert.Equal(t, builder, result, "WithComponent should return builder for chaining")
}

func TestDescribeComponentPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in describe component command.
	_, describeValidator, describeUsage := NewComponentPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Check usage matches expected pattern for describe component command.
	assert.Equal(t, "<component>", describeUsage)

	// Test validator with real describe component scenarios.
	err := describeValidator(nil, []string{"vpc"})
	assert.NoError(t, err, "should accept: atmos describe component vpc")

	err = describeValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos describe component (missing component)")
}

func TestDescribeComponentPositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command.
	_, validator, usage := NewComponentPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Simulate command setup.
	cmdUse := "component " + usage
	assert.Equal(t, "component <component>", cmdUse)

	// Validate args work correctly.
	err := validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)
}
