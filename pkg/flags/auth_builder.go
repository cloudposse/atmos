package flags

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthOptionsBuilder provides a type-safe, fluent interface for building AuthParser
// with strongly-typed flag definitions that map directly to AuthOptions fields.
//
// Benefits:
//   - Compile-time guarantee that flags map to AuthOptions fields
//   - Refactoring-safe: renaming struct fields updates flag definitions
//   - Clear intent: method names match struct field names
//   - Testable: each method can be unit tested independently
//
// Example:
//
//	parser := flags.NewAuthOptionsBuilder().
//	    WithVerbose().          // Verbose flag → .Verbose field
//	    WithOutput("table").    // Output flag with default → .Output field
//	    WithDestination().      // Destination flag → .Destination field
//	    WithDuration("1h").     // Duration flag with default → .Duration field
//	    Build()
//
//	opts, _ := parser.Parse(ctx, args)
//	fmt.Println(opts.Verbose)     // Type-safe!
//	fmt.Println(opts.Destination) // Type-safe!
type AuthOptionsBuilder struct {
	options []Option
}

// NewAuthOptionsBuilder creates a new builder for AuthParser.
func NewAuthOptionsBuilder() *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.NewAuthOptionsBuilder")()

	return &AuthOptionsBuilder{
		options: []Option{},
	}
}

// WithVerbose adds the verbose flag.
// Maps to AuthOptions.Verbose field.
func (b *AuthOptionsBuilder) WithVerbose() *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.WithVerbose")()

	b.options = append(b.options, WithBoolFlag("verbose", "v", false, "Enable verbose output"))
	b.options = append(b.options, WithEnvVars("verbose", "ATMOS_VERBOSE"))
	return b
}

// WithOutput adds the output flag with explicit valid values and default.
// Maps to AuthOptions.Output field.
//
// Parameters:
//   - validOutputs: List of valid output values (e.g., []string{"table", "json", "yaml"})
//   - defaultValue: Default output format to use when flag not provided
//
// Example:
//
//	WithOutput([]string{"table", "json"}, "table")  // auth whoami
func (b *AuthOptionsBuilder) WithOutput(validOutputs []string, defaultValue string) *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.WithOutput")()

	description := fmt.Sprintf("Output format (valid: %s)", strings.Join(validOutputs, ", "))
	b.options = append(b.options, WithStringFlag("output", "o", defaultValue, description))
	b.options = append(b.options, WithEnvVars("output", "ATMOS_OUTPUT"))
	b.options = append(b.options, WithValidValues("output", validOutputs...))
	return b
}

// WithDestination adds the destination flag for console navigation.
// Maps to AuthOptions.Destination field.
func (b *AuthOptionsBuilder) WithDestination() *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.WithDestination")()

	b.options = append(b.options, WithStringFlag("destination", "", "", "Console page to navigate to (AWS service alias or URL)"))
	b.options = append(b.options, WithEnvVars("destination", "ATMOS_CONSOLE_DESTINATION"))
	return b
}

// WithDuration adds the duration flag for console session duration.
// Maps to AuthOptions.Duration field.
//
// Parameters:
//   - defaultValue: default duration as string (e.g., "1h", "2h30m")
func (b *AuthOptionsBuilder) WithDuration(defaultValue string) *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.WithDuration")()

	b.options = append(b.options, WithStringFlag("duration", "", defaultValue, "Console session duration (provider may have max limits)"))
	b.options = append(b.options, WithEnvVars("duration", "ATMOS_CONSOLE_DURATION"))
	return b
}

// WithIssuer adds the issuer flag for console session issuer.
// Maps to AuthOptions.Issuer field.
//
// Parameters:
//   - defaultValue: default issuer (typically "atmos")
func (b *AuthOptionsBuilder) WithIssuer(defaultValue string) *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.WithIssuer")()

	b.options = append(b.options, WithStringFlag("issuer", "", defaultValue, "Issuer identifier for console session (AWS only)"))
	b.options = append(b.options, WithEnvVars("issuer", "ATMOS_CONSOLE_ISSUER"))
	return b
}

// WithPrintOnly adds the print-only flag.
// Maps to AuthOptions.PrintOnly field.
func (b *AuthOptionsBuilder) WithPrintOnly() *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.WithPrintOnly")()

	b.options = append(b.options, WithBoolFlag("print-only", "", false, "Print console URL to stdout without opening browser"))
	b.options = append(b.options, WithEnvVars("print-only", "ATMOS_CONSOLE_PRINT_ONLY"))
	return b
}

// WithNoOpen adds the no-open flag.
// Maps to AuthOptions.NoOpen field.
func (b *AuthOptionsBuilder) WithNoOpen() *AuthOptionsBuilder {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.WithNoOpen")()

	b.options = append(b.options, WithBoolFlag("no-open", "", false, "Generate URL but don't open browser automatically"))
	b.options = append(b.options, WithEnvVars("no-open", "ATMOS_CONSOLE_NO_OPEN"))
	return b
}

// Build creates the AuthParser with all configured flags.
func (b *AuthOptionsBuilder) Build() *AuthParser {
	defer perf.Track(nil, "flags.AuthOptionsBuilder.Build")()

	return &AuthParser{
		parser: NewStandardFlagParser(b.options...),
	}
}
