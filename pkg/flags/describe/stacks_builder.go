package describe

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StacksBuilder builds a parser for describe stacks command flags.
type StacksBuilder struct {
	options []flags.Option
}

// NewStacksBuilder creates a new builder for describe stacks options.
func NewStacksBuilder() *StacksBuilder {
	defer perf.Track(nil, "flags.NewStacksBuilder")()

	return &StacksBuilder{
		options: []flags.Option{},
	}
}

// WithStack adds the --stack flag.
func (b *StacksBuilder) WithStack() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithStack")()

	b.options = append(b.options, flags.WithStringFlag("stack", "s", "", "Atmos stack"))
	b.options = append(b.options, flags.WithEnvVars("stack", "ATMOS_STACK"))
	return b
}

// WithFormat adds the --format flag.
func (b *StacksBuilder) WithFormat() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithFormat")()

	b.options = append(b.options, flags.WithStringFlag("format", "f", "yaml", "Output format (valid: json, yaml)"))
	b.options = append(b.options, flags.WithEnvVars("format", "ATMOS_FORMAT"))
	return b
}

// WithFile adds the --file flag.
func (b *StacksBuilder) WithFile() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithFile")()

	b.options = append(b.options, flags.WithStringFlag("file", "", "", "Write output to file"))
	b.options = append(b.options, flags.WithEnvVars("file", "ATMOS_FILE"))
	return b
}

// WithProcessTemplates adds the --process-templates flag.
func (b *StacksBuilder) WithProcessTemplates() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithProcessTemplates")()

	b.options = append(b.options, flags.WithBoolFlag("process-templates", "", true, "Enable/disable Go template processing in Atmos stack manifests"))
	b.options = append(b.options, flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"))
	return b
}

// WithProcessFunctions adds the --process-functions flag.
func (b *StacksBuilder) WithProcessFunctions() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithProcessFunctions")()

	b.options = append(b.options, flags.WithBoolFlag("process-functions", "", true, "Enable/disable YAML functions processing in Atmos stack manifests"))
	b.options = append(b.options, flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"))
	return b
}

// WithComponents adds the --components flag.
func (b *StacksBuilder) WithComponents() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithComponents")()

	b.options = append(b.options, flags.WithStringSliceFlag("components", "", []string{}, "Filter by specific components"))
	b.options = append(b.options, flags.WithEnvVars("components", "ATMOS_COMPONENTS"))
	return b
}

// WithComponentTypes adds the --component-types flag.
func (b *StacksBuilder) WithComponentTypes() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithComponentTypes")()

	b.options = append(b.options, flags.WithStringSliceFlag("component-types", "", []string{}, "Filter by component types (terraform, helmfile)"))
	b.options = append(b.options, flags.WithEnvVars("component-types", "ATMOS_COMPONENT_TYPES"))
	return b
}

// WithSections adds the --sections flag.
func (b *StacksBuilder) WithSections() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithSections")()

	b.options = append(b.options, flags.WithStringSliceFlag("sections", "", []string{}, "Output only specified component sections (backend, vars, env, etc.)"))
	b.options = append(b.options, flags.WithEnvVars("sections", "ATMOS_SECTIONS"))
	return b
}

// WithIncludeEmptyStacks adds the --include-empty-stacks flag.
func (b *StacksBuilder) WithIncludeEmptyStacks() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithIncludeEmptyStacks")()

	b.options = append(b.options, flags.WithBoolFlag("include-empty-stacks", "", false, "Include stacks with no components in output"))
	b.options = append(b.options, flags.WithEnvVars("include-empty-stacks", "ATMOS_INCLUDE_EMPTY_STACKS"))
	return b
}

// WithSkip adds the --skip flag.
func (b *StacksBuilder) WithSkip() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithSkip")()

	b.options = append(b.options, flags.WithStringSliceFlag("skip", "", []string{}, "Skip executing a YAML function in the Atmos stack manifests"))
	b.options = append(b.options, flags.WithEnvVars("skip", "ATMOS_SKIP"))
	return b
}

// WithQuery adds the --query flag.
func (b *StacksBuilder) WithQuery() *StacksBuilder {
	defer perf.Track(nil, "flags.StacksBuilder.WithQuery")()

	b.options = append(b.options, flags.WithStringFlag("query", "q", "", "JQ/JMESPath query to filter output"))
	b.options = append(b.options, flags.WithEnvVars("query", "ATMOS_QUERY"))
	return b
}

// Build creates the StacksParser with all configured options.
func (b *StacksBuilder) Build() *StacksParser {
	defer perf.Track(nil, "flags.StacksBuilder.Build")()

	return &StacksParser{
		Parser: flags.NewStandardFlagParser(b.options...),
	}
}
