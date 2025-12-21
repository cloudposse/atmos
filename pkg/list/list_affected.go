package list

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// Table columns for list affected - uses colored status dot for interactive display.
var tableAffectedColumns = []column.Config{
	{Name: " ", Value: "{{ .status }}", Width: 1},
	{Name: "Component", Value: "{{ .component }}"},
	{Name: "Stack", Value: "{{ .stack }}"},
	{Name: "Type", Value: "{{ .component_type }}"},
	{Name: "Affected", Value: "{{ .affected }}"},
	{Name: "File", Value: "{{ .file }}"},
}

// Data columns for list affected - uses semantic status text for machine-readable output.
var dataAffectedColumns = []column.Config{
	{Name: "Status", Value: "{{ .status_text }}"},
	{Name: "Component", Value: "{{ .component }}"},
	{Name: "Stack", Value: "{{ .stack }}"},
	{Name: "Type", Value: "{{ .component_type }}"},
	{Name: "Affected", Value: "{{ .affected }}"},
	{Name: "File", Value: "{{ .file }}"},
}

// AffectedCommandOptions contains options for the list affected command.
type AffectedCommandOptions struct {
	Info        *schema.ConfigAndStacksInfo
	Cmd         *cobra.Command
	Args        []string
	ColumnsFlag []string
	FilterSpec  string
	SortSpec    string
	Delimiter   string

	// Git comparison options.
	Ref            string
	SHA            string
	RepoPath       string
	SSHKeyPath     string
	SSHKeyPassword string
	CloneTargetRef bool

	// Content options.
	IncludeDependents bool
	Stack             string

	// Processing options.
	ProcessTemplates bool
	ProcessFunctions bool
	Skip             []string
	ExcludeLocked    bool
}

// ExecuteListAffectedCmd executes the list affected command.
func ExecuteListAffectedCmd(opts *AffectedCommandOptions) error {
	defer perf.Track(nil, "list.ExecuteListAffectedCmd")()

	log.Trace("ExecuteListAffectedCmd starting")

	// Initialize CLI config.
	atmosConfig, err := cfg.InitCliConfig(*opts.Info, true)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Get format flag.
	formatFlag, err := opts.Cmd.Flags().GetString("format")
	if err != nil {
		return fmt.Errorf("failed to get format flag: %w", err)
	}

	// Default to table format if not specified.
	if formatFlag == "" {
		formatFlag = string(format.FormatTable)
	}

	// Get affected components using existing describe affected logic.
	// Always include settings so we can extract enabled/locked status.
	var result *affectedResult
	err = spinner.ExecWithSpinnerDynamic(
		"Comparing",
		func() (string, error) {
			var innerErr error
			result, innerErr = getAffectedComponents(&atmosConfig, opts)
			if innerErr != nil {
				return "", innerErr
			}
			// Return dynamic completion message with refs (base...head = "what changed from base to head").
			if result.LocalRef != "" && result.RemoteRef != "" {
				return fmt.Sprintf("Compared `%s`...`%s`", result.RemoteRef, result.LocalRef), nil
			}
			return "Compared branches", nil
		})
	if err != nil {
		return fmt.Errorf("failed to get affected components: %w", err)
	}

	if len(result.Affected) == 0 {
		_ = ui.Success("No affected components found")
		return nil
	}

	// Render output for affected components.
	return renderAffected(&atmosConfig, result.Affected, opts, formatFlag)
}

// renderAffected renders the affected components to output.
func renderAffected(atmosConfig *schema.AtmosConfiguration, affected []schema.Affected, opts *AffectedCommandOptions, formatFlag string) error {
	defer perf.Track(atmosConfig, "list.renderAffected")()

	// Extract affected into renderer-compatible format.
	data := extract.Affected(affected, opts.IncludeDependents)

	// Get column configuration (format-aware: table uses colored dot, data formats use semantic text).
	columns := getAffectedColumns(atmosConfig, opts.ColumnsFlag, formatFlag)

	// Create column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("failed to create column selector: %w", err)
	}

	// Build filters.
	filters, err := buildAffectedFilters(opts.FilterSpec)
	if err != nil {
		return fmt.Errorf("failed to build filters: %w", err)
	}

	// Build sorters.
	sorters, err := buildAffectedSorters(opts.SortSpec, columns)
	if err != nil {
		return fmt.Errorf("failed to build sorters: %w", err)
	}

	// Create renderer and render output.
	r := renderer.New(filters, selector, sorters, format.Format(formatFlag), opts.Delimiter)
	return r.Render(data)
}

// affectedResult holds the results from getAffectedComponents.
type affectedResult struct {
	Affected     []schema.Affected
	LocalRef     string
	RemoteRef    string
	RemoteRepoID string
}

// getAffectedComponents calls the existing describe affected logic.
func getAffectedComponents(atmosConfig *schema.AtmosConfiguration, opts *AffectedCommandOptions) (*affectedResult, error) {
	defer perf.Track(atmosConfig, "list.getAffectedComponents")()

	logicResult, err := executeAffectedLogic(atmosConfig, opts)
	if err != nil {
		return nil, err
	}

	result := &affectedResult{
		Affected:     logicResult.affected,
		RemoteRepoID: logicResult.remoteRepoID,
	}
	setRefNames(result, opts, logicResult.localHead)
	return result, nil
}

// affectedLogicResult holds results from executeAffectedLogic.
type affectedLogicResult struct {
	affected     []schema.Affected
	localHead    *plumbing.Reference
	remoteRepoID string
}

// executeAffectedLogic calls the appropriate describe affected function based on options.
func executeAffectedLogic(atmosConfig *schema.AtmosConfiguration, opts *AffectedCommandOptions) (*affectedLogicResult, error) {
	includeSettings := true

	switch {
	case opts.RepoPath != "":
		affected, _, _, repoID, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
			atmosConfig,
			opts.RepoPath,
			false, // includeSpaceliftAdminStacks
			includeSettings,
			opts.Stack,
			opts.ProcessTemplates,
			opts.ProcessFunctions,
			opts.Skip,
			opts.ExcludeLocked,
		)
		if err != nil {
			return nil, err
		}
		return &affectedLogicResult{affected: affected, localHead: nil, remoteRepoID: repoID}, nil
	case opts.CloneTargetRef:
		affected, localHead, _, repoID, err := e.ExecuteDescribeAffectedWithTargetRefClone(
			atmosConfig,
			opts.Ref,
			opts.SHA,
			opts.SSHKeyPath,
			opts.SSHKeyPassword,
			false, // includeSpaceliftAdminStacks
			includeSettings,
			opts.Stack,
			opts.ProcessTemplates,
			opts.ProcessFunctions,
			opts.Skip,
			opts.ExcludeLocked,
		)
		if err != nil {
			return nil, err
		}
		return &affectedLogicResult{affected: affected, localHead: localHead, remoteRepoID: repoID}, nil
	default:
		affected, localHead, _, repoID, err := e.ExecuteDescribeAffectedWithTargetRefCheckout(
			atmosConfig,
			opts.Ref,
			opts.SHA,
			false, // includeSpaceliftAdminStacks
			includeSettings,
			opts.Stack,
			opts.ProcessTemplates,
			opts.ProcessFunctions,
			opts.Skip,
			opts.ExcludeLocked,
		)
		if err != nil {
			return nil, err
		}
		return &affectedLogicResult{affected: affected, localHead: localHead, remoteRepoID: repoID}, nil
	}
}

// setRefNames sets the local and remote ref names in the result based on the option type and local repo head.
// When localRepoHead is nil (e.g., detached HEAD state), LocalRef remains empty which is handled gracefully
// by the caller - the spinner will display a generic "Compared branches" message instead.
func setRefNames(result *affectedResult, opts *AffectedCommandOptions, localRepoHead *plumbing.Reference) {
	if opts.RepoPath != "" {
		result.LocalRef = "HEAD"
		result.RemoteRef = opts.RepoPath
	} else {
		if localRepoHead != nil {
			result.LocalRef = localRepoHead.Name().String()
		}
		result.RemoteRef = selectRemoteRef(opts.SHA, opts.Ref)
	}
}

// getAffectedColumns returns column configuration from CLI flag, atmos.yaml, or defaults.
// For table format, uses colored status dot; for data formats (JSON/YAML/CSV/TSV), uses semantic status text.
func getAffectedColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string, formatFlag string) []column.Config {
	defer perf.Track(atmosConfig, "list.getAffectedColumns")()

	// If --columns flag is provided, parse it (user takes responsibility for format).
	if len(columnsFlag) > 0 {
		columns, err := parseAffectedColumnsFlag(columnsFlag)
		if err == nil && len(columns) > 0 {
			return columns
		}
	}

	// Select columns based on output format.
	switch format.Format(formatFlag) {
	case format.FormatJSON, format.FormatYAML, format.FormatCSV, format.FormatTSV:
		return dataAffectedColumns
	default:
		return tableAffectedColumns
	}
}

// selectRemoteRef returns the remote ref to compare against.
// Prefers SHA if provided, otherwise uses Ref, falls back to refs/remotes/origin/HEAD.
func selectRemoteRef(sha, ref string) string {
	switch {
	case sha != "":
		return sha
	case ref != "":
		return ref
	default:
		return "refs/remotes/origin/HEAD"
	}
}

// parseAffectedColumnsFlag parses column specifications from CLI flag.
// Each flag value should be in the format: "Name=TemplateExpression".
func parseAffectedColumnsFlag(columnsFlag []string) ([]column.Config, error) {
	defer perf.Track(nil, "list.parseAffectedColumnsFlag")()

	if len(columnsFlag) == 0 {
		return tableAffectedColumns, nil
	}

	columns := make([]column.Config, 0, len(columnsFlag))
	for _, spec := range columnsFlag {
		// Split on first '=' to separate name from template.
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			continue
		}

		name := parts[0]
		value := parts[1]

		if name == "" || value == "" {
			continue
		}

		columns = append(columns, column.Config{
			Name:  name,
			Value: value,
		})
	}

	if len(columns) == 0 {
		return tableAffectedColumns, nil
	}

	return columns, nil
}

// buildAffectedFilters creates filters from filter specification.
func buildAffectedFilters(filterSpec string) ([]filter.Filter, error) {
	defer perf.Track(nil, "list.buildAffectedFilters")()

	if filterSpec == "" {
		return nil, nil
	}

	// Support filtering by affected reason, component, stack, etc.
	// Format: "field:value" or "field=value".
	var filters []filter.Filter

	// Parse filter spec (could be extended later).
	idx := -1
	for i, c := range filterSpec {
		if c == ':' || c == '=' {
			idx = i
			break
		}
	}
	if idx > 0 && idx < len(filterSpec)-1 {
		field := filterSpec[:idx]
		value := filterSpec[idx+1:]
		filters = append(filters, filter.NewColumnFilter(field, value))
	}

	return filters, nil
}

// buildAffectedSorters creates sorters from sort specification.
func buildAffectedSorters(sortSpec string, columns []column.Config) ([]*listSort.Sorter, error) {
	defer perf.Track(nil, "list.buildAffectedSorters")()

	// If user provided explicit sort spec, use it.
	if sortSpec != "" {
		return listSort.ParseSortSpec(sortSpec)
	}

	// Build map of available column names.
	columnNames := make(map[string]bool)
	for _, col := range columns {
		columnNames[col.Name] = true
	}

	// Default sort by Stack, then Component.
	if columnNames["Stack"] && columnNames["Component"] {
		return []*listSort.Sorter{
			listSort.NewSorter("Stack", listSort.Ascending),
			listSort.NewSorter("Component", listSort.Ascending),
		}, nil
	}

	return nil, nil
}
