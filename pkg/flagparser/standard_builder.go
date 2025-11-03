package flagparser

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// StandardInterpreterBuilder provides a type-safe, fluent interface for building StandardParser
// with strongly-typed flag definitions that map directly to StandardInterpreter fields.
//
// Benefits:
//   - Compile-time guarantee that flags map to interpreter fields
//   - Refactoring-safe: renaming struct fields updates flag definitions
//   - Clear intent: method names match struct field names
//   - Testable: each method can be unit tested independently
//
// Example:
//
//	parser := flagparser.NewStandardInterpreterBuilder().
//	    WithStack(true).        // Required stack flag → .Stack field
//	    WithFormat("yaml").     // Format flag with default → .Format field
//	    WithQuery().            // Optional query flag → .Query field
//	    Build()
//
//	interpreter, _ := parser.Parse(ctx, args)
//	fmt.Println(interpreter.Stack)   // Type-safe!
//	fmt.Println(interpreter.Format)  // Type-safe!
type StandardInterpreterBuilder struct {
	options []Option
}

// NewStandardInterpreterBuilder creates a new builder for StandardParser.
func NewStandardInterpreterBuilder() *StandardInterpreterBuilder {
	defer perf.Track(nil, "flagparser.NewStandardInterpreterBuilder")()

	return &StandardInterpreterBuilder{
		options: []Option{},
	}
}

// WithStack adds the stack flag.
// Maps to StandardInterpreter.Stack field.
//
// Parameters:
//   - required: if true, flag is marked as required
func (b *StandardInterpreterBuilder) WithStack(required bool) *StandardInterpreterBuilder {
	if required {
		b.options = append(b.options, WithRequiredStringFlag("stack", "s", "Atmos stack"))
	} else {
		b.options = append(b.options, WithStringFlag("stack", "s", "", "Atmos stack"))
	}
	b.options = append(b.options, WithEnvVars("stack", "ATMOS_STACK"))
	return b
}

// WithComponent adds the component flag.
// Maps to StandardInterpreter.Component field.
//
// Parameters:
//   - required: if true, flag is marked as required
func (b *StandardInterpreterBuilder) WithComponent(required bool) *StandardInterpreterBuilder {
	if required {
		b.options = append(b.options, WithRequiredStringFlag("component", "c", "Atmos component"))
	} else {
		b.options = append(b.options, WithStringFlag("component", "c", "", "Atmos component"))
	}
	b.options = append(b.options, WithEnvVars("component", "ATMOS_COMPONENT"))
	return b
}

// WithFormat adds the format flag with specified default value.
// Maps to StandardInterpreter.Format field.
//
// Parameters:
//   - defaultValue: default format (e.g., "yaml", "json")
func (b *StandardInterpreterBuilder) WithFormat(defaultValue string) *StandardInterpreterBuilder {
	b.options = append(b.options, WithStringFlag("format", "f", defaultValue, "Output format"))
	b.options = append(b.options, WithEnvVars("format", "ATMOS_FORMAT"))
	return b
}

// WithFile adds the file output flag.
// Maps to StandardInterpreter.File field.
func (b *StandardInterpreterBuilder) WithFile() *StandardInterpreterBuilder {
	b.options = append(b.options, WithStringFlag("file", "", "", "Write output to file"))
	b.options = append(b.options, WithEnvVars("file", "ATMOS_FILE"))
	return b
}

// WithProcessTemplates adds the process-templates flag with specified default.
// Maps to StandardInterpreter.ProcessTemplates field.
//
// Parameters:
//   - defaultValue: default value (typically true)
func (b *StandardInterpreterBuilder) WithProcessTemplates(defaultValue bool) *StandardInterpreterBuilder {
	b.options = append(b.options, WithBoolFlag("process-templates", "", defaultValue, "Enable/disable Go template processing in Atmos stack manifests"))
	b.options = append(b.options, WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"))
	return b
}

// WithProcessFunctions adds the process-functions flag with specified default.
// Maps to StandardInterpreter.ProcessYamlFunctions field.
//
// Parameters:
//   - defaultValue: default value (typically true)
func (b *StandardInterpreterBuilder) WithProcessFunctions(defaultValue bool) *StandardInterpreterBuilder {
	b.options = append(b.options, WithBoolFlag("process-functions", "", defaultValue, "Enable/disable YAML functions processing in Atmos stack manifests"))
	b.options = append(b.options, WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"))
	return b
}

// WithSkip adds the skip flag for skipping YAML functions.
// Maps to StandardInterpreter.Skip field.
func (b *StandardInterpreterBuilder) WithSkip() *StandardInterpreterBuilder {
	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "skip",
			Shorthand:   "",
			Default:     []string{},
			Description: "Skip executing a YAML function in the Atmos stack manifests",
			EnvVars:     []string{"ATMOS_SKIP"},
		})
	})
	return b
}

// WithDryRun adds the dry-run flag.
// Maps to StandardInterpreter.DryRun field.
func (b *StandardInterpreterBuilder) WithDryRun() *StandardInterpreterBuilder {
	b.options = append(b.options, WithBoolFlag("dry-run", "", false, "Simulate operation without making changes"))
	b.options = append(b.options, WithEnvVars("dry-run", "ATMOS_DRY_RUN"))
	return b
}

// WithQuery adds the query flag for JQ/JMESPath queries.
// Maps to StandardInterpreter.Query field.
func (b *StandardInterpreterBuilder) WithQuery() *StandardInterpreterBuilder {
	b.options = append(b.options, WithStringFlag("query", "q", "", "JQ/JMESPath query to filter output"))
	b.options = append(b.options, WithEnvVars("query", "ATMOS_QUERY"))
	return b
}

// WithProvenance adds the provenance tracking flag.
// Maps to StandardInterpreter.Provenance field.
func (b *StandardInterpreterBuilder) WithProvenance() *StandardInterpreterBuilder {
	b.options = append(b.options, WithBoolFlag("provenance", "", false, "Enable provenance tracking to show where configuration values originated"))
	b.options = append(b.options, WithEnvVars("provenance", "ATMOS_PROVENANCE"))
	return b
}

// Build creates the StandardParser with all configured flags.
// Returns a parser ready for RegisterFlags() and Parse() operations.
func (b *StandardInterpreterBuilder) Build() *StandardParser {
	defer perf.Track(nil, "flagparser.StandardInterpreterBuilder.Build")()

	return NewStandardParser(b.options...)
}
