package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateSchemaPositionalArgsBuilder provides domain-specific builder for validate schema command positional arguments.
//
// Features:
//   - Semantic method names (WithSchemaType vs generic AddArg)
//   - Auto-configures TargetField mapping to StandardOptions.SchemaType
//   - Auto-generates Cobra Args validator
//   - Auto-generates usage string
//
// Usage:
//
//	specs, validator, usage := flags.NewValidateSchemaPositionalArgsBuilder().
//	    WithSchemaType(true).  // SchemaType is required
//	    Build()
//
//	parser := flags.NewStandardOptionsBuilder().
//	    WithPositionalArgs(specs, validator, usage).
//	    Build()
type ValidateSchemaPositionalArgsBuilder struct {
	builder *PositionalArgsBuilder
}

// NewValidateSchemaPositionalArgsBuilder creates a new ValidateSchemaPositionalArgsBuilder.
func NewValidateSchemaPositionalArgsBuilder() *ValidateSchemaPositionalArgsBuilder {
	defer perf.Track(nil, "flags.NewValidateSchemaPositionalArgsBuilder")()

	return &ValidateSchemaPositionalArgsBuilder{
		builder: NewPositionalArgsBuilder(),
	}
}

// WithSchemaType adds the schema type positional argument.
// This maps to StandardOptions.SchemaType field.
//
// Parameters:
//   - required: Whether schema type argument is required
//
// Example:
//
//	builder.WithSchemaType(true) // <schema-type> - required
func (b *ValidateSchemaPositionalArgsBuilder) WithSchemaType(required bool) *ValidateSchemaPositionalArgsBuilder {
	defer perf.Track(nil, "flags.ValidateSchemaPositionalArgsBuilder.WithSchemaType")()

	b.builder.AddArg(&PositionalArgSpec{
		Name:        "schema-type",
		Description: "Schema type to validate (jsonschema or opa)",
		Required:    required,
		TargetField: "SchemaType", // Maps to StandardOptions.SchemaType field
	})

	return b
}

// Build generates the positional args configuration.
//
// Returns:
//   - specs: Array of positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function
//   - usage: Usage string for Cobra Use field (e.g., "<schema-type>")
func (b *ValidateSchemaPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
	defer perf.Track(nil, "flags.ValidateSchemaPositionalArgsBuilder.Build")()

	return b.builder.Build()
}
