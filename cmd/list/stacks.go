package list

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/dependencies"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/importresolver"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/list/tree"
	log "github.com/cloudposse/atmos/pkg/logger"
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/ui"
)

var stacksParser *flags.StandardParser

// StacksOptions contains parsed flags for the stacks command.
type StacksOptions struct {
	global.Flags
	Component        string
	Format           string
	Columns          []string
	Sort             string
	Provenance       bool
	ProcessTemplates bool
	ProcessFunctions bool
	Skip             []string
	ErrorMode        string
	Tags             []string
	LabelsRaw        string
	// IncludeDependencies/IncludeDependents preview the dependency closure
	// (0 = off, -1 = unlimited, N>0 = N levels): the listed stacks are exactly
	// the ones a terraform bulk run with the same selection flags would touch.
	IncludeDependencies int
	IncludeDependents   int
}

// stacksCmd lists atmos stacks.
var stacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List all Atmos stacks with filtering, sorting, and formatting options",
	Long:  `List Atmos stacks with support for filtering by component, custom column selection, sorting, and multiple output formats.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Check Atmos configuration (honors --base-path, --config, --config-path, --profile).
		// Skip the stacks-directory-exists check here: listStacksWithOptions below already
		// reports a friendly "No stacks found" message when there are no stack manifests yet,
		// whether the stacks directory is missing or simply empty (a brand-new project).
		if err := checkAtmosConfig(cmd, v, true); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := stacksParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := parseStacksOptions(cmd, v)
		if err := parseListClosureOptions(v, &opts.IncludeDependencies, &opts.IncludeDependents); err != nil {
			return err
		}

		return listStacksWithOptions(cmd, args, opts)
	},
}

// parseStacksOptions maps viper state into a StacksOptions struct.
// Extracted from the RunE closure so the viper→options mapping can be
// unit-tested without driving the whole cobra command.
func parseStacksOptions(cmd *cobra.Command, v *viper.Viper) *StacksOptions {
	return &StacksOptions{
		Flags:            flags.ParseGlobalFlags(cmd, v),
		Component:        v.GetString("component"),
		Format:           v.GetString("format"),
		Columns:          v.GetStringSlice("columns"),
		Sort:             v.GetString("sort"),
		Provenance:       v.GetBool("provenance"),
		ProcessTemplates: v.GetBool("process-templates"),
		ProcessFunctions: v.GetBool("process-functions"),
		Skip:             v.GetStringSlice("skip"),
		ErrorMode:        v.GetString("error-mode"),
		Tags:             tags.ParseTagsFlag(v.GetString("tags")),
		LabelsRaw:        v.GetString("labels"),
	}
}

// columnsCompletionForStacks provides dynamic tab completion for --columns flag.
// Returns column names from atmos.yaml stacks.list.columns configuration.
func columnsCompletionForStacks(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "list.stacks.columnsCompletionForStacks")()

	// Load atmos configuration with CLI flags.
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Extract column names from atmos.yaml configuration.
	if len(atmosConfig.Stacks.List.Columns) > 0 {
		var columnNames []string
		for _, col := range atmosConfig.Stacks.List.Columns {
			columnNames = append(columnNames, col.Name)
		}
		return columnNames, cobra.ShellCompDirectiveNoFileComp
	}

	// If no custom columns configured, return empty list.
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	// Create parser with stacks-specific flags using flag wrappers.
	stacksParser = NewListParser(
		WithFormatFlag,
		WithStacksColumnsFlag,
		WithSortFlag,
		WithComponentFlag,
		WithTagsFlag,
		WithLabelsFlag,
		WithClosureFlags,
		WithProvenanceFlag,
		WithProcessTemplatesFlag,
		WithProcessFunctionsFlag,
		WithSkipFlag,
		WithErrorModeFlag,
	)

	// Register flags.
	stacksParser.RegisterFlags(stacksCmd)

	// Register dynamic tab completion for --columns flag.
	if err := stacksCmd.RegisterFlagCompletionFunc("columns", columnsCompletionForStacks); err != nil {
		panic(err)
	}

	// Bind flags to Viper for environment variable support.
	if err := stacksParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listStacksWithOptions(cmd *cobra.Command, args []string, opts *StacksOptions) error {
	defer perf.Track(nil, "list.stacks.listStacksWithOptions")()

	// Early validation: --provenance only works with --format=tree.
	if err := validateProvenanceFlag(opts); err != nil {
		return err
	}

	// Initialize configuration and auth.
	atmosConfig, authManager, err := initStacksConfig(cmd, args, opts)
	if err != nil {
		if errors.Is(err, errUtils.ErrFailedToFindImport) || errors.Is(err, errUtils.ErrNoStackManifestsFound) {
			ui.Info("No stacks found")
			return nil
		}
		return err
	}

	// Build the error-mode options once and share the same Collector across every
	// describe-stacks call this command makes (table path, and the tree path's
	// re-processing pass below), so the end-of-command summary reports one combined
	// count instead of printing separately per call site. Deferred so it fires after
	// whichever render path below has finished writing its output.
	errOpts, collector := describeStacksErrorOptions(opts.ErrorMode)
	defer printErrorModeSummary(opts.ErrorMode, collector)

	// Execute describe stacks and extract results.
	stacks, stacksMap, err := executeAndExtractStacks(&atmosConfig, opts, authManager, errOpts)
	if err != nil {
		return err
	}
	if len(stacks) == 0 {
		ui.Info("No stacks found")
		return nil
	}

	// Handle tree format specially - it shows import hierarchies.
	if opts.Format == string(format.FormatTree) {
		return renderStacksTreeFormat(&atmosConfig, stacks, opts, authManager, errOpts)
	}
	_ = stacksMap // Unused in non-tree format.

	// Render stacks with filters, columns, and sorters.
	return renderStacksTable(&atmosConfig, stacks, opts)
}

// validateProvenanceFlag checks that --provenance is only used with --format=tree.
func validateProvenanceFlag(opts *StacksOptions) error {
	if opts.Provenance && opts.Format != "" && opts.Format != string(format.FormatTree) {
		return fmt.Errorf("%w: --provenance flag only works with --format=tree", errUtils.ErrInvalidFlag)
	}
	return nil
}

// initStacksConfig initializes configuration and authentication for the stacks command.
func initStacksConfig(
	cmd *cobra.Command,
	args []string,
	opts *StacksOptions,
) (schema.AtmosConfiguration, auth.AuthManager, error) {
	defer perf.Track(nil, "list.stacks.initStacksConfig")()

	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, err
	}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, fmt.Errorf("%w: %w", errUtils.ErrInitializingCLIConfig, err)
	}

	applyConfigDefaultedFormat(opts, atmosConfig.Stacks.List.Format)

	// Resolve --error-mode: explicit flag/env value wins, else atmos.yaml's
	// list.error_mode, else "warn".
	opts.ErrorMode = e.ResolveErrorMode(opts.ErrorMode, atmosConfig.List.ErrorMode)

	// Validate provenance after resolving format from config.
	if opts.Provenance && opts.Format != string(format.FormatTree) {
		return schema.AtmosConfiguration{}, nil, fmt.Errorf("%w: --provenance flag only works with --format=tree", errUtils.ErrInvalidFlag)
	}

	authManager, err := createAuthManagerForList(cmd, &atmosConfig, opts.ProcessTemplates, opts.ProcessFunctions)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, err
	}

	return atmosConfig, authManager, nil
}

// applyConfigDefaultedFormat applies stacks.list.format from atmos.yaml when --format
// wasn't set via flag (tree by default — journaled in pkg/edition, so an edition pin
// restores the table). A defaulted tree steps aside for row-shaped flags (tree renders
// the import hierarchy and has no rows or columns); only an explicit --format=tree
// conflicts with them.
func applyConfigDefaultedFormat(opts *StacksOptions, configFormat string) {
	if opts.Format != "" || configFormat == "" {
		return
	}
	opts.Format = configFormat
	if opts.Format == string(format.FormatTree) && len(opts.Columns) > 0 {
		opts.Format = ""
	}
}

// executeAndExtractStacks runs describe stacks and extracts the results.
func executeAndExtractStacks(
	atmosConfig *schema.AtmosConfiguration,
	opts *StacksOptions,
	authManager auth.AuthManager,
	errOpts e.DescribeStacksErrorOptions,
) ([]map[string]any, map[string]any, error) {
	defer perf.Track(nil, "list.stacks.executeAndExtractStacks")()
	skip := skipCredentialBackedYAMLFunctionsForInventory(opts.Skip, authManager)

	labels, err := tags.ParseLabelsFlag(opts.LabelsRaw)
	if err != nil {
		return nil, nil, err
	}

	// With closure flags, the whole describe/extract flow goes through the
	// shared scoped closure engine: only the stacks (and components) the
	// closure touches are ever evaluated, matching the terraform bulk paths.
	closureRequested := opts.IncludeDependencies != 0 || opts.IncludeDependents != 0
	if closureRequested {
		return extractStacksViaScopedClosure(atmosConfig, opts, labels, &scopedDescribeDeps{authManager: authManager, skip: skip, errOpts: errOpts})
	}

	// Without closure flags, --tags/--labels also scope the describe pass
	// (early-skip): components excluded by the selectors never evaluate
	// templates/YAML functions/auth.
	stacksMap, err := e.ExecuteDescribeStacksScoped(
		atmosConfig, "", nil, nil, nil,
		false, // ignoreMissingFiles
		opts.ProcessTemplates,
		opts.ProcessFunctions,
		false, // includeEmptyStacks
		skip,
		authManager,
		authManager == nil,
		opts.Tags,
		labels,
		errOpts,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	var stacks []map[string]any
	if opts.Component != "" {
		stacks, err = extract.StacksForComponent(opts.Component, stacksMap, opts.Tags, labels)
	} else {
		stacks, err = extract.Stacks(stacksMap, opts.Tags, labels)
	}
	if err != nil {
		return nil, nil, err
	}

	return stacks, stacksMap, nil
}

// scopedDescribeDeps bundles the describe-side dependencies shared by the
// list commands' scoped closure helpers, keeping the helpers within the
// argument-count limit.
type scopedDescribeDeps struct {
	authManager auth.AuthManager
	skip        []string
	errOpts     e.DescribeStacksErrorOptions
}

// newScopedDescribeFunc builds the dependencies.DescribeFunc the list
// commands hand to ResolveScopedClosure: one ExecuteDescribeStacksScoped pass
// per closure stack, narrowed to the closure's own components, never to the
// caller's tags/labels (closure scoping owns selection).
func newScopedDescribeFunc(atmosConfig *schema.AtmosConfiguration, describeDeps *scopedDescribeDeps) dependencies.DescribeFunc {
	return func(stackName string, closureComponents []string, processTemplates, processFunctions bool) (map[string]any, error) {
		return e.ExecuteDescribeStacksScoped(
			atmosConfig, stackName, closureComponents, nil, nil,
			false, // ignoreMissingFiles
			processTemplates,
			processFunctions,
			false, // includeEmptyStacks
			describeDeps.skip,
			describeDeps.authManager,
			describeDeps.authManager == nil,
			nil, // tagsFilter: closure scoping owns selection.
			nil, // labelsFilter: closure scoping owns selection.
			describeDeps.errOpts,
		)
	}
}

// extractStacksViaScopedClosure lists the stacks the dependency closure
// touches, using the shared three-phase scoped evaluation: a lightweight
// structural pass seeds the closure (optional component + tags/labels
// selectors), and only the closure's own components are then fully evaluated —
// exactly the stacks a terraform bulk run with the same selection flags would
// touch, without evaluating anything outside the closure. Closure members are
// kept even when they do not match the selectors that seeded them.
func extractStacksViaScopedClosure(
	atmosConfig *schema.AtmosConfiguration,
	opts *StacksOptions,
	labels map[string]string,
	describeDeps *scopedDescribeDeps,
) ([]map[string]any, map[string]any, error) {
	defer perf.Track(nil, "list.stacks.extractStacksViaScopedClosure")()

	describe := newScopedDescribeFunc(atmosConfig, describeDeps)

	var components []string
	if opts.Component != "" {
		components = []string{opts.Component}
	}
	direction, depths := dependencies.ClosureScope(opts.IncludeDependencies, opts.IncludeDependents)
	leftDelim, rightDelim := tags.TemplateDelims(atmosConfig.Templates.Settings.Delimiters)
	result, err := dependencies.ResolveScopedClosure(describe, &dependencies.ScopeRequest{
		Components:       components,
		Tags:             opts.Tags,
		Labels:           labels,
		Direction:        direction,
		Depths:           depths,
		ProcessTemplates: opts.ProcessTemplates,
		ProcessFunctions: opts.ProcessFunctions,
		LeftDelim:        leftDelim,
		RightDelim:       rightDelim,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	// Restrict output to the FINAL closure's stacks before extraction.
	// result.Stacks can hold conservatively evaluated stacks that are not
	// closure members: the reverse direction evaluates unresolved dependency
	// sources so their edges materialize, and refineRoots can drop an
	// initially conservative root once its selector resolves — in both cases
	// the evaluated stack stays in result.Stacks. list components and
	// list instances already filter rows by closure membership; stacks must
	// too so the preview matches the execution set.
	closureStacks := filterStacksByNames(result.Stacks, dependencies.StackNames(result.Closure))
	stacks, err := extract.Stacks(closureStacks, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if opts.Component != "" {
		annotateStackRowsWithComponent(stacks, closureStacks, opts.Component)
	}
	return stacks, closureStacks, nil
}

// annotateStackRowsWithComponent mirrors extract.StacksForComponent's row
// shape on the closure path: rows for stacks that actually contain the
// selected component carry it under "component", so configured columns like
// `{{ .component }}` render identically with and without closure flags.
// Prerequisite stacks pulled in by the closure without the component keep no
// component key rather than claiming one they do not have.
func annotateStackRowsWithComponent(rows []map[string]any, stacksMap map[string]any, component string) {
	for _, row := range rows {
		stackName, _ := row["stack"].(string)
		if stackContainsComponent(stacksMap, stackName, component) {
			row["component"] = component
		}
	}
}

// stackContainsComponent reports whether the described stack holds the named
// component under any component type.
func stackContainsComponent(stacksMap map[string]any, stackName, component string) bool {
	stack, _ := stacksMap[stackName].(map[string]any)
	componentsSection, _ := stack["components"].(map[string]any)
	for _, typeValue := range componentsSection {
		typeSection, _ := typeValue.(map[string]any)
		if _, ok := typeSection[component]; ok {
			return true
		}
	}
	return false
}

// filterStacksByNames keeps only the named stacks from the describe map.
func filterStacksByNames(stacks map[string]any, names []string) map[string]any {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	filtered := make(map[string]any, len(names))
	for name, section := range stacks {
		if _, ok := allowed[name]; ok {
			filtered[name] = section
		}
	}
	return filtered
}

// renderStacksTable renders stacks in table format with filters, columns, and sorters.
func renderStacksTable(atmosConfig *schema.AtmosConfiguration, stacks []map[string]any, opts *StacksOptions) error {
	defer perf.Track(nil, "list.stacks.renderStacksTable")()

	filters := buildStackFilters(opts)
	columns := getStackColumns(atmosConfig, opts.Columns, opts.Component != "")

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	sorters, err := buildStackSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat, "")
	return renderWithPager(atmosConfig, "List Stacks", r, stacks)
}

// buildStackFilters creates filters based on command options.
func buildStackFilters(opts *StacksOptions) []filter.Filter {
	var filters []filter.Filter

	// Component and tags/labels filters are already handled by extraction
	// logic (extract.Stacks/extract.StacksForComponent), so the tree format
	// and structured outputs honor them too. Add any additional
	// renderer-level filters here in the future.

	return filters
}

// getStackColumns returns column configuration.
func getStackColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string, hasComponent bool) []column.Config {
	defer perf.Track(nil, "list.stacks.getStackColumns")()

	// If --columns flag is provided, parse it and return.
	if len(columnsFlag) > 0 {
		return parseColumnsFlag(columnsFlag)
	}

	// Check atmos.yaml for stacks.list.columns configuration.
	if len(atmosConfig.Stacks.List.Columns) > 0 {
		var configs []column.Config
		for _, col := range atmosConfig.Stacks.List.Columns {
			configs = append(configs, column.Config{
				Name:  col.Name,
				Value: col.Value,
				Width: col.Width,
			})
		}
		return configs
	}

	// Default columns for stacks.
	if hasComponent {
		// When filtering by component, show both stack and component.
		return []column.Config{
			{Name: "Stack", Value: "{{ .stack }}"},
			{Name: "Component", Value: "{{ .component }}"},
		}
	}

	// When showing all stacks, just show stack name.
	return []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
	}
}

// renderStacksTreeFormat handles the tree format output for stacks.
// It enables provenance tracking, re-processes stacks, and renders the import hierarchy.
func renderStacksTreeFormat(
	atmosConfig *schema.AtmosConfiguration,
	stacks []map[string]any,
	opts *StacksOptions,
	authManager auth.AuthManager,
	errOpts e.DescribeStacksErrorOptions,
) error {
	defer perf.Track(nil, "list.stacks.renderStacksTreeFormat")()

	log.Trace("Tree format detected, enabling provenance tracking")
	atmosConfig.TrackProvenance = true

	// Clear caches to ensure fresh processing with provenance enabled.
	e.ClearMergeContexts()
	e.ClearFindStacksMapCache()
	log.Trace("Caches cleared, re-processing with provenance")

	// Re-process stacks with provenance tracking enabled. Honor the
	// caller-supplied template/function flags so tree output is consistent with
	// non-tree runs of the same command invocation.
	skip := skipCredentialBackedYAMLFunctionsForInventory(opts.Skip, authManager)
	stacksMap, err := e.ExecuteDescribeStacksWithOptions(
		atmosConfig, "", nil, nil, nil,
		false, // ignoreMissingFiles
		opts.ProcessTemplates,
		opts.ProcessFunctions,
		false, // includeEmptyStacks
		skip,
		authManager,
		authManager == nil,
		errOpts,
	)
	if err != nil {
		return fmt.Errorf("error re-processing stacks with provenance: %w", err)
	}

	// Resolve import trees and filter to allowed stacks.
	importTrees, err := resolveAndFilterImportTrees(stacksMap, atmosConfig, stacks)
	if err != nil {
		return err
	}

	// Render and output the tree.
	output := format.RenderStacksTree(importTrees, opts.Provenance)
	_ = data.Writeln(output)
	return nil
}

// resolveAndFilterImportTrees resolves import trees from provenance and filters to allowed stacks.
func resolveAndFilterImportTrees(
	stacksMap map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	stacks []map[string]any,
) (map[string][]*tree.ImportNode, error) {
	defer perf.Track(nil, "list.stacks.resolveAndFilterImportTrees")()

	importTreesWithComponents, err := importresolver.ResolveImportTreeFromProvenance(stacksMap, atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("error resolving import tree from provenance: %w", err)
	}

	// Build a set of allowed stack names from the already-filtered stacks slice.
	allowedStacks := buildAllowedStacksSet(stacks)

	// Flatten component level - for stacks view, we just need stack → imports.
	// All components in a stack share the same import chain from the stack file.
	importTrees := make(map[string][]*tree.ImportNode)
	for stackName, componentImports := range importTreesWithComponents {
		if !allowedStacks[stackName] {
			continue
		}
		// Just take the first component's imports (they're all the same for a stack file).
		for _, imports := range componentImports {
			importTrees[stackName] = imports
			break
		}
	}

	return importTrees, nil
}

// buildAllowedStacksSet creates a set of stack names from a slice of stack maps.
func buildAllowedStacksSet(stacks []map[string]any) map[string]bool {
	defer perf.Track(nil, "list.stacks.buildAllowedStacksSet")()

	allowedStacks := make(map[string]bool)
	for _, stack := range stacks {
		if stackName, ok := stack["stack"].(string); ok {
			allowedStacks[stackName] = true
		}
	}
	return allowedStacks
}

// buildStackSorters creates sorters from sort specification.
func buildStackSorters(sortSpec string) ([]*listSort.Sorter, error) {
	defer perf.Track(nil, "list.stacks.buildStackSorters")()

	if sortSpec == "" {
		// Default sort: by stack ascending.
		return []*listSort.Sorter{
			listSort.NewSorter("Stack", listSort.Ascending),
		}, nil
	}

	return listSort.ParseSortSpec(sortSpec)
}
