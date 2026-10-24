package git

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Flag and env-var names for `atmos git list`.
const (
	listFlagColumns     = "columns"
	listFlagFormat      = "format"
	listFlagDelimiter   = "delimiter"
	listFlagCheckStatus = "check-status"

	listEnvColumns     = "ATMOS_GIT_LIST_COLUMNS"
	listEnvFormat      = "ATMOS_GIT_LIST_FORMAT"
	listEnvCheckStatus = "ATMOS_GIT_LIST_CHECK_STATUS"

	// Supported output formats for `atmos git list` (tree and matrix excluded).
	validFormats = "table|json|yaml|csv|tsv"
)

// GitListOptions holds parsed options for `atmos git list`.
type GitListOptions struct {
	global.Flags
	Columns     []string
	Format      string
	Delimiter   string
	CheckStatus bool
}

// listParser handles flag parsing for `atmos git list`.
var listParser *flags.StandardParser

// listCmd is the `atmos git list` subcommand.
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured Git repositories",
	Long: `List managed Git repositories configured under git.repositories in atmos.yaml.

Output includes name, URI, provider, branch, and resolved workdir for each repository.
Default table output also probes local workdirs and shows a status dot for each
repository. Pass --columns to customize the output; pass --check-status when a
custom column references status fields.

Formats: ` + validFormats + `.
tree and matrix are not supported for this flat repository list.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.list.RunE")()

		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := parseGitListOptions(cmd, v)
		return listGitRepositories(cmd, args, opts)
	},
}

// parseGitListOptions maps Viper state into a GitListOptions struct.
func parseGitListOptions(cmd *cobra.Command, v *viper.Viper) *GitListOptions {
	return &GitListOptions{
		Flags:       flags.ParseGlobalFlags(cmd, v),
		Columns:     v.GetStringSlice(viperKey(listViperPrefix, listFlagColumns)),
		Format:      v.GetString(viperKey(listViperPrefix, listFlagFormat)),
		Delimiter:   v.GetString(viperKey(listViperPrefix, listFlagDelimiter)),
		CheckStatus: v.GetBool(viperKey(listViperPrefix, listFlagCheckStatus)),
	}
}

// listGitRepositories implements the main pipeline for `atmos git list`.
func listGitRepositories(cmd *cobra.Command, args []string, opts *GitListOptions) error {
	defer perf.Track(nil, "git.listGitRepositories")()

	// Validate format early: reject unsupported formats.
	if err := validateGitListFormat(opts.Format); err != nil {
		return err
	}

	// Load Atmos config (without stack validation — git list does not need stacks).
	atmosConfig, err := loadGitListConfig(cmd, args, opts)
	if err != nil {
		return err
	}

	return renderGitRepositoriesList(&atmosConfig, opts)
}

// renderGitRepositoriesList runs the extraction and rendering pipeline given a
// pre-loaded Atmos configuration. It is split from listGitRepositories so the
// rendering logic can be exercised in unit tests without a full Atmos environment.
func renderGitRepositoriesList(atmosConfig *schema.AtmosConfiguration, opts *GitListOptions) error {
	defer perf.Track(nil, "git.renderGitRepositoriesList")()

	// Resolve format from config when not set via flag.
	if opts.Format == "" && atmosConfig.Git.List.Format != "" {
		if err := validateGitListFormat(atmosConfig.Git.List.Format); err != nil {
			return err
		}
		opts.Format = atmosConfig.Git.List.Format
	}

	cfg := &atmosConfig.Git

	outputFormat := format.Format(opts.Format)
	includeStatus := shouldProbeGitListStatus(atmosConfig, opts, outputFormat)

	// Extract rows. Default table output probes status so it can show the dot
	// column; custom columns opt in with --check-status.
	rows, err := extractGitRepoRows(cfg, includeStatus)
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		ui.Info("No repositories configured under git.repositories.")
		return nil
	}

	// Build column selector.
	cols := getGitListColumns(atmosConfig, opts.Columns, includeStatus, outputFormat)

	selector, err := column.NewSelector(cols, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Default sort: name ascending.
	sorters, err := buildGitListSorters("")
	if err != nil {
		return fmt.Errorf("error building default sorters: %w", err)
	}

	// No additional filters beyond extraction (repositories are already enumerated).
	var filters []filter.Filter

	r := renderer.New(filters, selector, sorters, outputFormat, opts.Delimiter)
	return r.Render(rows)
}

func shouldProbeGitListStatus(atmosConfig *schema.AtmosConfiguration, opts *GitListOptions, outputFormat format.Format) bool {
	if opts.CheckStatus {
		return true
	}
	if len(opts.Columns) > 0 || len(atmosConfig.Git.List.Columns) > 0 {
		return false
	}
	return outputFormat == "" || outputFormat == format.FormatTable
}

// validateGitListFormat returns an error when format is a value not supported
// by `atmos git list`. "tree" and "matrix" are explicitly excluded.
func validateGitListFormat(f string) error {
	switch format.Format(f) {
	case "", format.FormatTable, format.FormatJSON, format.FormatYAML,
		format.FormatCSV, format.FormatTSV:
		return nil
	case format.FormatTree:
		return fmt.Errorf("%w: tree format is not supported for atmos git list (flat list has no hierarchy); "+
			"supported formats: %s", errUtils.ErrInvalidFlag, validFormats)
	}
	// Reject "matrix" and any other unknown value.
	return fmt.Errorf("%w: unsupported format %q for atmos git list; supported formats: %s",
		errUtils.ErrInvalidFlag, f, validFormats)
}

// loadGitListConfig loads Atmos configuration for the list command.
// It honours global flags (--base-path, --config, --config-path, --profile)
// from opts but intentionally skips stack validation (git list needs only the
// top-level `git` section, not the full stacks tree).
func loadGitListConfig(_ *cobra.Command, _ []string, opts *GitListOptions) (schema.AtmosConfiguration, error) {
	defer perf.Track(nil, "git.loadGitListConfig")()

	globalFlags := opts.Flags
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return schema.AtmosConfiguration{}, fmt.Errorf("loading Atmos configuration: %w", err)
	}

	return atmosConfig, nil
}

// getGitListColumns returns the column configuration for the list output.
// Precedence: --columns flag > atmos.yaml git.list.columns > defaults.
func getGitListColumns(
	atmosConfig *schema.AtmosConfiguration,
	columnsFlag []string,
	includeStatus bool,
	outputFormat format.Format,
) []column.Config {
	defer perf.Track(nil, "git.getGitListColumns")()

	if len(columnsFlag) > 0 {
		return parseGitColumnsFlag(columnsFlag)
	}

	if len(atmosConfig.Git.List.Columns) > 0 {
		cols := make([]column.Config, 0, len(atmosConfig.Git.List.Columns))
		for _, c := range atmosConfig.Git.List.Columns {
			cols = append(cols, column.Config{
				Name:  c.Name,
				Value: c.Value,
				Width: c.Width,
			})
		}
		return cols
	}

	return defaultGitListColumns(includeStatus, outputFormat)
}

// defaultGitListColumns returns the default column set for git list.
// The status column is appended only when --check-status is set.
func defaultGitListColumns(includeStatus bool, outputFormat format.Format) []column.Config {
	cols := []column.Config{
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "URI", Value: "{{ .uri }}"},
		{Name: "Provider", Value: "{{ .provider }}"},
		{Name: "Branch", Value: "{{ .branch }}"},
		{Name: "Workdir", Value: "{{ .workdir }}"},
	}
	if !includeStatus {
		return cols
	}
	switch outputFormat {
	case format.FormatJSON, format.FormatYAML, format.FormatCSV, format.FormatTSV:
		return append(cols, column.Config{Name: "Status", Value: "{{ .status_text }}"})
	default:
		return append([]column.Config{{Name: " ", Value: "{{ .status }}", Width: 1}}, cols...)
	}
}

// parseGitColumnsFlag converts string column specs into column.Config values.
// Supports "Name" (maps to {{ .name }}) and "Name={{ .template }}" formats.
func parseGitColumnsFlag(specs []string) []column.Config {
	defer perf.Track(nil, "git.parseGitColumnsFlag")()

	var cols []column.Config
	for _, spec := range specs {
		c := parseGitColumnSpec(spec)
		if c.Name != "" {
			cols = append(cols, c)
		}
	}
	return cols
}

// parseGitColumnSpec parses a single column specification.
// "Name" is a shorthand for "Name={{ .name }}" (lowercased key).
func parseGitColumnSpec(spec string) column.Config {
	for i, ch := range spec {
		if ch == '=' {
			name := spec[:i]
			value := spec[i+1:]
			return column.Config{Name: name, Value: value}
		}
	}
	// Shorthand: column name only → derive template key from lowercased name.
	if spec == "" {
		return column.Config{}
	}
	// Use lowercase key derived from the column name.
	key := lowerFirst(spec)
	return column.Config{Name: spec, Value: fmt.Sprintf("{{ .%s }}", key)}
}

// lowerFirst returns s with the first rune lower-cased.
func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	r := rune(s[0])
	if r >= 'A' && r <= 'Z' {
		return string(r+32) + s[1:]
	}
	return s
}

// buildGitListSorters returns the default sorter (name ascending).
// The sort specification is reserved for future --sort flag support.
func buildGitListSorters(sortSpec string) ([]*listSort.Sorter, error) {
	defer perf.Track(nil, "git.buildGitListSorters")()

	if sortSpec != "" {
		return listSort.ParseSortSpec(sortSpec)
	}
	return []*listSort.Sorter{
		listSort.NewSorter("Name", listSort.Ascending),
	}, nil
}

// columnsCompletionForGitList provides dynamic tab completion for --columns.
// Returns column names from atmos.yaml git.list.columns, plus defaults.
func columnsCompletionForGitList(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "git.columnsCompletionForGitList")()

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return defaultGitColumnNames(false), cobra.ShellCompDirectiveNoFileComp
	}

	if len(atmosConfig.Git.List.Columns) > 0 {
		names := make([]string, 0, len(atmosConfig.Git.List.Columns))
		for _, c := range atmosConfig.Git.List.Columns {
			names = append(names, c.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}

	return defaultGitColumnNames(true), cobra.ShellCompDirectiveNoFileComp
}

// defaultGitColumnNames returns default column name suggestions for tab completion.
func defaultGitColumnNames(includeStatus bool) []string {
	names := []string{"Name", "URI", "Provider", "Branch", "Workdir"}
	if includeStatus {
		names = append(names, "Status")
	}
	return names
}

func init() {
	listParser = flags.NewStandardParser(
		flags.WithViperPrefix(listViperPrefix),
		flags.WithStringSliceFlag(listFlagColumns, "", []string{}, "Columns to display (comma-separated, overrides atmos.yaml)"),
		flags.WithStringFlag(listFlagFormat, "f", "", "Output format: "+validFormats),
		flags.WithStringFlag(listFlagDelimiter, "", "", "Delimiter for CSV/TSV output"),
		flags.WithBoolFlag(listFlagCheckStatus, "", false, "Probe filesystem for clone status (missing/cloned/dirty)"),
		flags.WithEnvVars(listFlagColumns, listEnvColumns),
		flags.WithEnvVars(listFlagFormat, listEnvFormat),
		flags.WithEnvVars(listFlagCheckStatus, listEnvCheckStatus),
	)

	listParser.RegisterFlags(listCmd)

	if err := listCmd.RegisterFlagCompletionFunc(listFlagColumns, columnsCompletionForGitList); err != nil {
		panic(fmt.Sprintf("git list: RegisterFlagCompletionFunc columns: %v", err))
	}

	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git list: BindToViper: %v", err))
	}

	gitCmd.AddCommand(listCmd)
}
