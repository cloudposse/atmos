//nolint:dupl // Similar test structure to other builder tests, but testing different domain-specific builder
package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescribeDependentsPositionalArgsBuilder_WithComponent_Required(t *testing.T) {
	builder := NewDependentsPositionalArgsBuilder()
	builder.WithComponent(true)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Component name to find dependents for", specs[0].Description)

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

func TestDescribeDependentsPositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
	builder := NewDependentsPositionalArgsBuilder()
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

func TestDescribeDependentsPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining.
	builder := NewDependentsPositionalArgsBuilder()
	result := builder.WithComponent(true)

	assert.Equal(t, builder, result, "WithComponent should return builder for chaining")
}

func TestDescribeDependentsPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in describe dependents command.
	_, describeValidator, describeUsage := NewDependentsPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Check usage matches expected pattern for describe dependents command.
	assert.Equal(t, "<component>", describeUsage)

	// Test validator with real describe dependents scenarios.
	err := describeValidator(nil, []string{"vpc"})
	assert.NoError(t, err, "should accept: atmos describe dependents vpc")

	err = describeValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos describe dependents (missing component)")
}

func TestDescribeDependentsPositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command.
	_, validator, usage := NewDependentsPositionalArgsBuilder().
		WithComponent(true).
		Build()

	// Simulate command setup.
	cmdUse := "dependents " + usage
	assert.Equal(t, "dependents <component>", cmdUse)

	// Validate args work correctly.
	err := validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)
}
