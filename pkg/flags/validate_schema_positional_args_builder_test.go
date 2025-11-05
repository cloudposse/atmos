//nolint:dupl // Similar test structure to other builder tests, but testing different domain-specific builder
package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSchemaPositionalArgsBuilder_WithSchemaType_Required(t *testing.T) {
	builder := NewValidateSchemaPositionalArgsBuilder()
	builder.WithSchemaType(true)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "schema-type", specs[0].Name)
	assert.Equal(t, "SchemaType", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Schema type to validate (jsonschema or opa)", specs[0].Description)

	// Check usage string.
	assert.Equal(t, "<schema-type>", usage)

	// Check validator requires exactly 1 arg.
	err := validator(nil, []string{"jsonschema"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)

	err = validator(nil, []string{"jsonschema", "opa"})
	assert.Error(t, err)
}

func TestValidateSchemaPositionalArgsBuilder_WithSchemaType_Optional(t *testing.T) {
	builder := NewValidateSchemaPositionalArgsBuilder()
	builder.WithSchemaType(false)

	specs, validator, usage := builder.Build()

	// Check specs.
	assert.Len(t, specs, 1)
	assert.Equal(t, "schema-type", specs[0].Name)
	assert.Equal(t, "SchemaType", specs[0].TargetField)
	assert.False(t, specs[0].Required)

	// Check usage string.
	assert.Equal(t, "[schema-type]", usage)

	// Check validator accepts 0 or 1 args.
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"jsonschema"})
	assert.NoError(t, err)

	err = validator(nil, []string{"jsonschema", "opa"})
	assert.Error(t, err)
}

func TestValidateSchemaPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining.
	builder := NewValidateSchemaPositionalArgsBuilder()
	result := builder.WithSchemaType(true)

	assert.Equal(t, builder, result, "WithSchemaType should return builder for chaining")
}

func TestValidateSchemaPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in validate schema command.
	_, validateAllValidator, validateAllUsage := NewValidateSchemaPositionalArgsBuilder().
		WithSchemaType(false).
		Build()

	// Check usage matches expected pattern for validate all schemas.
	assert.Equal(t, "[schema-type]", validateAllUsage)

	// Test validator with real validate schema scenarios.
	err := validateAllValidator(nil, []string{})
	assert.NoError(t, err, "should accept: atmos validate schema")

	err = validateAllValidator(nil, []string{"jsonschema"})
	assert.NoError(t, err, "should accept: atmos validate schema jsonschema")

	err = validateAllValidator(nil, []string{"jsonschema", "opa"})
	assert.Error(t, err, "should reject: atmos validate schema jsonschema opa (too many args)")
}

func TestValidateSchemaPositionalArgsBuilder_IntegrationWithCobra(t *testing.T) {
	// Test integration with actual Cobra command.
	_, validator, usage := NewValidateSchemaPositionalArgsBuilder().
		WithSchemaType(false).
		Build()

	// Simulate command setup.
	cmdUse := "schema " + usage
	assert.Equal(t, "schema [schema-type]", cmdUse)

	// Validate args work correctly.
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"jsonschema"})
	assert.NoError(t, err)

	err = validator(nil, []string{"jsonschema", "extra"})
	assert.Error(t, err)
}
