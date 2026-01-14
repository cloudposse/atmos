package list

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Flag names.
	flagColumns = "columns"

	// Environment variables.
	envListColumns = "ATMOS_LIST_COLUMNS"

	// Flag descriptions.
	descColumns = "Columns to display (comma-separated, overrides atmos.yaml)"
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
		flags.WithStringFlag("format", "f", "", "Output format: table, json, yaml, csv, tsv, tree"),
		flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
		flags.WithValidValues("format", "table", "json", "yaml", "csv", "tsv", "tree"),
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

// WithInstancesColumnsFlag adds column selection flag for list instances command.
// Tab completion is registered via RegisterFlagCompletionFunc in the command init.
// Used by: instances.
func WithInstancesColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithInstancesColumnsFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag(flagColumns, "", []string{}, descColumns),
		flags.WithEnvVars(flagColumns, envListColumns),
	)
}

// WithMetadataColumnsFlag adds column selection flag for list metadata and components commands.
// Tab completion is registered via RegisterFlagCompletionFunc in the command init.
// Used by: metadata, components.
func WithMetadataColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithMetadataColumnsFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag(flagColumns, "", []string{}, descColumns),
		flags.WithEnvVars(flagColumns, envListColumns),
	)
}

// WithComponentsColumnsFlag adds column selection flag for list components command.
// Components command uses the same columns as metadata.
// Tab completion is registered via RegisterFlagCompletionFunc in the command init.
// Used by: components.
func WithComponentsColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithComponentsColumnsFlag")()

	// Components share metadata columns.
	WithMetadataColumnsFlag(options)
}

// WithStacksColumnsFlag adds column selection flag for list stacks command.
// Tab completion is registered via RegisterFlagCompletionFunc in the command init.
// Used by: stacks.
func WithStacksColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithStacksColumnsFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag(flagColumns, "", []string{}, descColumns),
		flags.WithEnvVars(flagColumns, envListColumns),
	)
}

// WithWorkflowsColumnsFlag adds column selection flag for list workflows command.
// Tab completion is registered via RegisterFlagCompletionFunc in the command init.
// Used by: workflows.
func WithWorkflowsColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithWorkflowsColumnsFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag(flagColumns, "", []string{}, descColumns),
		flags.WithEnvVars(flagColumns, envListColumns),
	)
}

// WithVendorColumnsFlag adds column selection flag for list vendor command.
// Tab completion is registered via RegisterFlagCompletionFunc in the command init.
// Used by: vendor.
func WithVendorColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithVendorColumnsFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag(flagColumns, "", []string{}, descColumns),
		flags.WithEnvVars(flagColumns, envListColumns),
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

// WithProvenanceFlag adds provenance display flag for tree format.
// Used by: instances, stacks.
func WithProvenanceFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithProvenanceFlag")()

	*options = append(*options,
		flags.WithBoolFlag("provenance", "", false, "Show import provenance (only works with --format=tree)"),
		flags.WithEnvVars("provenance", "ATMOS_PROVENANCE"),
	)
}

// WithAffectedColumnsFlag adds column selection flag for list affected command.
// Used by: affected.
func WithAffectedColumnsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithAffectedColumnsFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag(flagColumns, "", []string{}, descColumns),
		flags.WithEnvVars(flagColumns, envListColumns),
	)
}

// WithRefFlag adds git reference flag for comparing branches.
// Used by: affected.
func WithRefFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithRefFlag")()

	*options = append(*options,
		flags.WithStringFlag("ref", "", "", "Git reference with which to compare the current branch"),
		flags.WithEnvVars("ref", "ATMOS_AFFECTED_REF"),
	)
}

// WithSHAFlag adds git commit SHA flag for comparing branches.
// Used by: affected.
func WithSHAFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithSHAFlag")()

	*options = append(*options,
		flags.WithStringFlag("sha", "", "", "Git commit SHA with which to compare the current branch"),
		flags.WithEnvVars("sha", "ATMOS_AFFECTED_SHA"),
	)
}

// WithRepoPathFlag adds repository path flag for comparing with a cloned repo.
// Used by: affected.
func WithRepoPathFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithRepoPathFlag")()

	*options = append(*options,
		flags.WithStringFlag("repo-path", "", "", "Filesystem path to the already cloned target repository"),
		flags.WithEnvVars("repo-path", "ATMOS_AFFECTED_REPO_PATH"),
	)
}

// WithSSHKeyFlag adds SSH key path flag for cloning private repos.
// Used by: affected.
func WithSSHKeyFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithSSHKeyFlag")()

	*options = append(*options,
		flags.WithStringFlag("ssh-key", "", "", "Path to PEM-encoded private key to clone private repos using SSH"),
		flags.WithEnvVars("ssh-key", "ATMOS_AFFECTED_SSH_KEY"),
	)
}

// WithSSHKeyPasswordFlag adds SSH key password flag.
// Used by: affected.
func WithSSHKeyPasswordFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithSSHKeyPasswordFlag")()

	*options = append(*options,
		flags.WithStringFlag("ssh-key-password", "", "", "Encryption password for the PEM-encoded private key"),
		flags.WithEnvVars("ssh-key-password", "ATMOS_AFFECTED_SSH_KEY_PASSWORD"),
	)
}

// WithCloneTargetRefFlag adds clone target ref flag.
// Used by: affected.
func WithCloneTargetRefFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithCloneTargetRefFlag")()

	*options = append(*options,
		flags.WithBoolFlag("clone-target-ref", "", false, "Clone the target reference instead of checking it out"),
		flags.WithEnvVars("clone-target-ref", "ATMOS_AFFECTED_CLONE_TARGET_REF"),
	)
}

// WithIncludeDependentsFlag adds include dependents flag.
// Used by: affected.
func WithIncludeDependentsFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithIncludeDependentsFlag")()

	*options = append(*options,
		flags.WithBoolFlag("include-dependents", "", false, "Include dependent components and stacks"),
		flags.WithEnvVars("include-dependents", "ATMOS_AFFECTED_INCLUDE_DEPENDENTS"),
	)
}

// WithExcludeLockedFlag adds exclude locked components flag.
// Used by: affected.
func WithExcludeLockedFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithExcludeLockedFlag")()

	*options = append(*options,
		flags.WithBoolFlag("exclude-locked", "", false, "Exclude locked components (metadata.locked: true)"),
		flags.WithEnvVars("exclude-locked", "ATMOS_AFFECTED_EXCLUDE_LOCKED"),
	)
}

// WithSkipFlag adds skip YAML functions flag.
// Used by: affected.
func WithSkipFlag(options *[]flags.Option) {
	defer perf.Track(nil, "list.WithSkipFlag")()

	*options = append(*options,
		flags.WithStringSliceFlag("skip", "", nil, "Skip executing specific YAML functions"),
		flags.WithEnvVars("skip", "ATMOS_AFFECTED_SKIP"),
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
