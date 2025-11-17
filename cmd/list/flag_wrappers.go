package list

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Named wrapper functions for list command flags.
// Follow With* naming convention from pkg/flags/ API.
// Each function appends flag options to the provided slice.
//
// Design principles:
// - One function per flag (granular composition)
// - Consistent naming: With{FlagName}Flag
// - Reusable across multiple list commands
// - Each command chooses only the flags it needs
// - Single source of truth for flag configuration

// WithFormatFlag adds output format flag with environment variable support.
// Used by: components, stacks, workflows, vendor, values, vars, metadata, settings, instances.
func WithFormatFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithFormatFlag")()

	*options = append(*options,
		flags.WithStringFlag("format", "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
		flags.WithValidValues("format", "table", "json", "yaml", "csv", "tsv"),
	)
}

// WithDelimiterFlag adds CSV/TSV delimiter flag.
// Used by: workflows, vendor, values, vars, metadata, settings, instances.
func WithDelimiterFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithDelimiterFlag")()

	*options = append(*options,
		flags.WithStringFlag("delimiter", "", "", "Delimiter for CSV/TSV output"),
		flags.WithEnvVars("delimiter", "ATMOS_LIST_DELIMITER"),
	)
}

// WithColumnsFlag adds column selection flag with environment variable support.
// Allows CLI override of atmos.yaml column configuration.
// Used by: components, stacks, workflows, vendor, instances.
func WithColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithColumnsFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag("columns", "", []string{}, "Columns to display (comma-separated, overrides atmos.yaml)"),
		flags.WithEnvVars("columns", "ATMOS_LIST_COLUMNS"),
	)
}

// WithStackFlag adds stack filter flag for filtering by stack pattern (glob).
// Used by: components, vendor, values, vars, metadata, settings, instances.
func WithStackFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithStackFlag")()

	*options = append(*options,
		flags.WithStringFlag("stack", "s", "", "Filter by stack pattern (glob, e.g., 'plat-*-prod')"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
	)
}

// WithFilterFlag adds YQ filter expression flag with environment variable support.
// Used by: components, vendor, instances.
func WithFilterFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithFilterFlag")()

	*options = append(*options,
		flags.WithStringFlag("filter", "", "", "Filter expression using YQ syntax"),
		flags.WithEnvVars("filter", "ATMOS_LIST_FILTER"),
	)
}

// WithSortFlag adds sort specification flag with environment variable support.
// Format: "column1:asc,column2:desc".
// Used by: components, stacks, workflows, vendor, instances.
func WithSortFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithSortFlag")()

	*options = append(*options,
		flags.WithStringFlag("sort", "", "", "Sort by column:order (e.g., 'stack:asc,component:desc')"),
		flags.WithEnvVars("sort", "ATMOS_LIST_SORT"),
	)
}

// WithEnabledFlag adds enabled filter flag for filtering by enabled status.
// Nil value = all, true = enabled only, false = disabled only.
// Used by: components.
func WithEnabledFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithEnabledFlag")()

	*options = append(*options,
		flags.WithBoolFlag("enabled", "", false, "Filter by enabled status (omit for all, --enabled=true for enabled only)"),
		flags.WithEnvVars("enabled", "ATMOS_COMPONENT_ENABLED"),
	)
}

// WithLockedFlag adds locked filter flag for filtering by locked status.
// Nil value = all, true = locked only, false = unlocked only.
// Used by: components.
func WithLockedFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithLockedFlag")()

	*options = append(*options,
		flags.WithBoolFlag("locked", "", false, "Filter by locked status (omit for all, --locked=true for locked only)"),
		flags.WithEnvVars("locked", "ATMOS_COMPONENT_LOCKED"),
	)
}

// WithTypeFlag adds component type filter flag with environment variable support.
// Valid values: "real", "abstract", "all".
// Used by: components.
func WithTypeFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithTypeFlag")()

	*options = append(*options,
		flags.WithStringFlag("type", "t", "real", "Component type: real, abstract, all"),
		flags.WithEnvVars("type", "ATMOS_COMPONENT_TYPE"),
		flags.WithValidValues("type", "real", "abstract", "all"),
	)
}

// WithComponentFlag adds component filter flag for filtering stacks by component.
// Used by: stacks.
func WithComponentFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithComponentFlag")()

	*options = append(*options,
		flags.WithStringFlag("component", "c", "", "Filter stacks by component name"),
		flags.WithEnvVars("component", "ATMOS_COMPONENT"),
	)
}

// WithFileFlag adds workflow file filter flag.
// Used by: workflows.
func WithFileFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithFileFlag")()

	*options = append(*options,
		flags.WithStringFlag("file", "", "", "Filter workflows by file path"),
		flags.WithEnvVars("file", "ATMOS_WORKFLOW_FILE"),
	)
}

// WithMaxColumnsFlag adds max columns limit flag for values/metadata/settings.
// Used by: values, vars, metadata, settings.
func WithMaxColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithMaxColumnsFlag")()

	*options = append(*options,
		flags.WithIntFlag("max-columns", "", 0, "Maximum number of columns to display (0 = no limit)"),
		flags.WithEnvVars("max-columns", "ATMOS_LIST_MAX_COLUMNS"),
	)
}

// WithQueryFlag adds YQ query expression flag for filtering values.
// Used by: values, vars, metadata, settings.
func WithQueryFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithQueryFlag")()

	*options = append(*options,
		flags.WithStringFlag("query", "q", "", "YQ expression to filter values (e.g., '.vars.region')"),
		flags.WithEnvVars("query", "ATMOS_LIST_QUERY"),
	)
}

// WithAbstractFlag adds abstract component inclusion flag.
// Used by: values, vars.
func WithAbstractFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithAbstractFlag")()

	*options = append(*options,
		flags.WithBoolFlag("abstract", "", false, "Include abstract components in output"),
		flags.WithEnvVars("abstract", "ATMOS_ABSTRACT"),
	)
}

// WithProcessTemplatesFlag adds template processing flag.
// Used by: values, vars, metadata, settings.
func WithProcessTemplatesFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithProcessTemplatesFlag")()

	*options = append(*options,
		flags.WithBoolFlag("process-templates", "", true, "Enable/disable Go template processing"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
	)
}

// WithProcessFunctionsFlag adds template function processing flag.
// Used by: values, vars, metadata, settings.
func WithProcessFunctionsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithProcessFunctionsFlag")()

	*options = append(*options,
		flags.WithBoolFlag("process-functions", "", true, "Enable/disable template function processing"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
	)
}

// WithUploadFlag adds upload to Pro API flag.
// Used by: instances.
func WithUploadFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithUploadFlag")()

	*options = append(*options,
		flags.WithBoolFlag("upload", "", false, "Upload instances to Atmos Pro API"),
		flags.WithEnvVars("upload", "ATMOS_UPLOAD"),
	)
}

// NewListParser creates a StandardParser with specified flag builders.
// Each command composes only the flags it needs by passing the appropriate With* functions.
//
// Example:
//
//	parser := NewListParser(
//	    WithFormatFlag,
//	    WithColumnsFlag,
//	    WithStackFlag,
//	)
func NewListParser(builders ...func(*[]flags.Option)) *flags.StandardParser {
	defer perf.Track(nil, "list.NewListParser")()

	options := []flags.Option{}

	// Apply each builder function to compose the flag set
	for _, builder := range builders {
		builder(&options)
	}

	return flags.NewStandardParser(options...)
}
