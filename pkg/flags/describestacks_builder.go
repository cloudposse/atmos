package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// DescribeStacksOptionsBuilder builds a parser for describe stacks command flags.
type DescribeStacksOptionsBuilder struct {
	options []Option
}

// NewDescribeStacksOptionsBuilder creates a new builder for describe stacks options.
func NewDescribeStacksOptionsBuilder() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.NewDescribeStacksOptionsBuilder")()

	return &DescribeStacksOptionsBuilder{
		options: []Option{},
	}
}

// WithStack adds the --stack flag.
func (b *DescribeStacksOptionsBuilder) WithStack() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithStack")()

	b.options = append(b.options, WithStringFlag("stack", "s", "", "Atmos stack"))
	b.options = append(b.options, WithEnvVars("stack", "ATMOS_STACK"))
	return b
}

// WithFormat adds the --format flag.
func (b *DescribeStacksOptionsBuilder) WithFormat() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithFormat")()

	b.options = append(b.options, WithStringFlag("format", "f", "yaml", "Output format (valid: json, yaml)"))
	b.options = append(b.options, WithEnvVars("format", "ATMOS_FORMAT"))
	return b
}

// WithFile adds the --file flag.
func (b *DescribeStacksOptionsBuilder) WithFile() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithFile")()

	b.options = append(b.options, WithStringFlag("file", "", "", "Write output to file"))
	b.options = append(b.options, WithEnvVars("file", "ATMOS_FILE"))
	return b
}

// WithProcessTemplates adds the --process-templates flag.
func (b *DescribeStacksOptionsBuilder) WithProcessTemplates() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithProcessTemplates")()

	b.options = append(b.options, WithBoolFlag("process-templates", "", true, "Enable/disable Go template processing in Atmos stack manifests"))
	b.options = append(b.options, WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"))
	return b
}

// WithProcessFunctions adds the --process-functions flag.
func (b *DescribeStacksOptionsBuilder) WithProcessFunctions() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithProcessFunctions")()

	b.options = append(b.options, WithBoolFlag("process-functions", "", true, "Enable/disable YAML functions processing in Atmos stack manifests"))
	b.options = append(b.options, WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"))
	return b
}

// WithComponents adds the --components flag.
func (b *DescribeStacksOptionsBuilder) WithComponents() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithComponents")()

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "components",
			Shorthand:   "",
			Default:     []string{},
			Description: "Filter by specific components",
			EnvVars:     []string{"ATMOS_COMPONENTS"},
		})
	})
	return b
}

// WithComponentTypes adds the --component-types flag.
func (b *DescribeStacksOptionsBuilder) WithComponentTypes() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithComponentTypes")()

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "component-types",
			Shorthand:   "",
			Default:     []string{},
			Description: "Filter by component types (terraform, helmfile)",
			EnvVars:     []string{"ATMOS_COMPONENT_TYPES"},
		})
	})
	return b
}

// WithSections adds the --sections flag.
func (b *DescribeStacksOptionsBuilder) WithSections() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithSections")()

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "sections",
			Shorthand:   "",
			Default:     []string{},
			Description: "Output only specified component sections (backend, vars, env, etc.)",
			EnvVars:     []string{"ATMOS_SECTIONS"},
		})
	})
	return b
}

// WithIncludeEmptyStacks adds the --include-empty-stacks flag.
func (b *DescribeStacksOptionsBuilder) WithIncludeEmptyStacks() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithIncludeEmptyStacks")()

	b.options = append(b.options, WithBoolFlag("include-empty-stacks", "", false, "Include stacks with no components in output"))
	b.options = append(b.options, WithEnvVars("include-empty-stacks", "ATMOS_INCLUDE_EMPTY_STACKS"))
	return b
}

// WithSkip adds the --skip flag.
func (b *DescribeStacksOptionsBuilder) WithSkip() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithSkip")()

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

// WithQuery adds the --query flag.
func (b *DescribeStacksOptionsBuilder) WithQuery() *DescribeStacksOptionsBuilder {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.WithQuery")()

	b.options = append(b.options, WithStringFlag("query", "q", "", "JQ/JMESPath query to filter output"))
	b.options = append(b.options, WithEnvVars("query", "ATMOS_QUERY"))
	return b
}

// Build creates the DescribeStacksParser with all configured options.
func (b *DescribeStacksOptionsBuilder) Build() *DescribeStacksParser {
	defer perf.Track(nil, "flags.DescribeStacksOptionsBuilder.Build")()

	return &DescribeStacksParser{
		parser: NewStandardFlagParser(b.options...),
	}
}
