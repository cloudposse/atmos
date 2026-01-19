package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Constants for map keys used in source data.
const (
	keyStack     = "stack"
	keyComponent = "component"
	keyFolder    = "folder"
	keyURI       = "uri"
	keyVersion   = "version"
)

// ListCommand creates a list command for the given component type.
func ListCommand(cfg *Config) *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithStringFlag("format", "f", "table", "Output format: table, json, yaml, csv, tsv"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)

	cmd := &cobra.Command{
		Use:   "list [component]",
		Short: fmt.Sprintf("List %s components with source configuration", cfg.TypeLabel),
		Long: fmt.Sprintf(`List all %s components that have source configured.

If component is specified, shows source info for that component across stacks.
If --stack is specified, limits to that stack.
If neither specified, lists all components with source across all stacks.

This command shows which components can be vendored using the source provisioner.`, cfg.TypeLabel),
		Example: fmt.Sprintf(`  # List all sources in a stack
  atmos %s source list --stack dev

  # List sources for a specific component across all stacks
  atmos %s source list vpc

  # List source for a specific component in a specific stack
  atmos %s source list vpc --stack dev

  # List all sources across all stacks
  atmos %s source list

  # Output as JSON
  atmos %s source list --format json`, cfg.ComponentType, cfg.ComponentType, cfg.ComponentType, cfg.ComponentType, cfg.ComponentType),
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeList(cmd, args, cfg, parser)
		},
	}

	cmd.DisableFlagParsing = false
	parser.RegisterFlags(cmd)

	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	return cmd
}

// listOptions holds the parsed options for the list command.
type listOptions struct {
	stack         string
	formatStr     string
	component     string
	typeLabel     string
	componentType string
}

func executeList(cmd *cobra.Command, args []string, cfg *Config, parser *flags.StandardParser) error {
	defer perf.Track(nil, fmt.Sprintf("source.%s.list.RunE", cfg.ComponentType))()

	opts, atmosConfig, err := initListCommand(cmd, args, cfg, parser)
	if err != nil {
		return err
	}

	sources, err := fetchAndFilterListSources(opts, atmosConfig)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		printNoListSourcesMessage(opts)
		return nil
	}

	return renderListOutput(sources, opts)
}

// initListCommand initializes the list command, parsing flags and config.
func initListCommand(cmd *cobra.Command, args []string, cfg *Config, parser *flags.StandardParser) (*listOptions, *schema.AtmosConfiguration, error) {
	v := viper.GetViper()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return nil, nil, err
	}

	opts := &listOptions{
		stack:         v.GetString("stack"),
		formatStr:     v.GetString("format"),
		typeLabel:     cfg.TypeLabel,
		componentType: cfg.ComponentType,
	}

	if len(args) > 0 {
		opts.component = args[0]
	}

	configInfo := schema.ConfigAndStacksInfo{Stack: opts.stack}
	atmosConfig, err := initCliConfigForPrompt(configInfo, true)
	if err != nil {
		return nil, nil, wrapConfigError(err, opts.stack)
	}

	return opts, &atmosConfig, nil
}

// wrapConfigError wraps configuration errors with user-friendly messages.
func wrapConfigError(err error, stack string) error {
	errMsg := err.Error()

	// Detect "no stacks found" pattern from import failures.
	if strings.Contains(errMsg, "failed to find import") ||
		strings.Contains(errMsg, "no files match") {
		builder := errUtils.Build(errUtils.ErrNoStacksFound).
			WithCause(err).
			WithExplanation("No stack configuration files were found matching the configured import patterns")

		// Extract and show the searched path if available.
		if path := extractSearchedPath(errMsg); path != "" {
			builder = builder.WithContext("searched", path)
		}

		return builder.
			WithHint("Ensure your `stacks` directory contains valid YAML files and check `atmos.yaml` configuration").
			WithHint("See https://atmos.tools/learn/stacks for stack configuration details").
			Err()
	}

	// Detect missing atmos.yaml or stacks directory.
	if strings.Contains(errMsg, "stacks directory does not exist") ||
		strings.Contains(errMsg, "atmos.yaml") {
		return errUtils.Build(errUtils.ErrMissingAtmosConfig).
			WithCause(err).
			WithExplanation("The Atmos configuration or stacks directory could not be found").
			WithHint("Run `atmos` from a directory containing `atmos.yaml`, or set `ATMOS_BASE_PATH`").
			WithHint("See https://atmos.tools/cli/configuration for configuration details").
			Err()
	}

	// Default: generic config error.
	builder := errUtils.Build(errUtils.ErrFailedToInitConfig).
		WithCause(err)
	if stack != "" {
		builder = builder.WithContext(keyStack, stack)
	}
	return builder.Err()
}

// extractSearchedPath extracts the file path from error messages.
func extractSearchedPath(errMsg string) string {
	// Pattern: "failed to find import: '/path/to/file'"
	if idx := strings.Index(errMsg, "failed to find import: '"); idx != -1 {
		start := idx + len("failed to find import: '")
		if end := strings.Index(errMsg[start:], "'"); end != -1 {
			return errMsg[start : start+end]
		}
	}
	return ""
}

// fetchAndFilterListSources fetches stack data and extracts filtered sources.
func fetchAndFilterListSources(opts *listOptions, atmosConfig *schema.AtmosConfiguration) ([]map[string]any, error) {
	stacksMap, err := executeDescribeStacksFunc(
		atmosConfig,
		opts.stack,
		nil,
		[]string{opts.componentType},
		nil,
		false, false, false, false,
		nil, nil,
	)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrExecuteDescribeStacks).
			WithCause(err).
			WithContext("stack", opts.stack).
			Err()
	}

	var sources []map[string]any
	if opts.stack != "" {
		sources = extractSourcesFromStack(stacksMap, opts.stack, opts.componentType)
	} else {
		sources = extractSourcesFromAllStacks(stacksMap, opts.componentType)
	}

	return filterByComponent(sources, opts.component), nil
}

// printNoListSourcesMessage prints an appropriate message when no sources are found.
func printNoListSourcesMessage(opts *listOptions) {
	msg := fmt.Sprintf("No %s components with source configured", opts.typeLabel)
	if opts.stack != "" {
		msg += fmt.Sprintf(" in stack %q", opts.stack)
	}
	if opts.component != "" {
		msg += fmt.Sprintf(" matching component %q", opts.component)
	}
	ui.Info(msg)
}

// renderListOutput renders the sources with dynamic columns and sorting.
func renderListOutput(sources []map[string]any, opts *listOptions) error {
	hasFolderDiff := checkFolderDiffers(sources)
	columns := getSourceListColumnsForContext(opts.stack != "", hasFolderDiff)

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithCause(err).
			WithExplanation("Failed to create column selector").
			Err()
	}

	sorters := buildListSorters(opts.stack == "")
	r := renderer.New(nil, selector, sorters, format.Format(opts.formatStr), "")
	return r.Render(sources)
}

// buildListSorters builds the sorters for list output.
func buildListSorters(includeStack bool) []*listSort.Sorter {
	var sorters []*listSort.Sorter
	if includeStack {
		sorters = append(sorters, listSort.NewSorter("Stack", listSort.Ascending))
	}
	sorters = append(sorters, listSort.NewSorter("Component", listSort.Ascending))
	return sorters
}

// getSourceListColumnsForContext returns the column configuration based on context.
// HasStack indicates whether --stack was provided (single stack mode).
// HasFolderDiff indicates whether any component has folder != component name.
func getSourceListColumnsForContext(hasStack, hasFolderDiff bool) []column.Config {
	var columns []column.Config

	// Stack column only when showing multiple stacks.
	if !hasStack {
		columns = append(columns, column.Config{Name: "Stack", Value: "{{ .stack }}"})
	}

	// Component (instance name) always shown.
	columns = append(columns, column.Config{Name: "Component", Value: "{{ .component }}"})

	// Folder only if it differs from component for any row.
	if hasFolderDiff {
		columns = append(columns, column.Config{Name: "Folder", Value: "{{ .folder }}"})
	}

	// URI and Version always shown.
	columns = append(columns,
		column.Config{Name: "URI", Value: "{{ .uri }}"},
		column.Config{Name: "Version", Value: "{{ .version }}"},
	)

	return columns
}

// checkFolderDiffers returns true if any source has folder != component.
func checkFolderDiffers(sources []map[string]any) bool {
	for _, s := range sources {
		if s[keyComponent] != s[keyFolder] {
			return true
		}
	}
	return false
}

// filterByComponent filters sources matching component name OR folder.
func filterByComponent(sources []map[string]any, componentFilter string) []map[string]any {
	if componentFilter == "" {
		return sources
	}
	var filtered []map[string]any
	for _, s := range sources {
		component, _ := s[keyComponent].(string)
		folder, _ := s[keyFolder].(string)
		// Match if component name OR folder matches the filter.
		if component == componentFilter || folder == componentFilter {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// extractSourcesFromStack extracts components with source configuration from a single stack.
func extractSourcesFromStack(stacksMap map[string]any, stack, componentType string) []map[string]any {
	defer perf.Track(nil, "source.cmd.extractSourcesFromStack")()

	stackData, ok := stacksMap[stack].(map[string]any)
	if !ok {
		return nil
	}

	components, ok := stackData["components"].(map[string]any)
	if !ok {
		return nil
	}

	typeComponents, ok := components[componentType].(map[string]any)
	if !ok {
		return nil
	}

	var sources []map[string]any
	for componentName, componentData := range typeComponents {
		componentMap, ok := componentData.(map[string]any)
		if !ok {
			continue
		}

		if !source.HasSource(componentMap) {
			continue
		}

		sourceSpec, err := source.ExtractSource(componentMap)
		if err != nil || sourceSpec == nil {
			continue
		}

		// Extract component folder from metadata.component.
		folder := extractComponentFolder(componentMap, componentName)

		sources = append(sources, map[string]any{
			keyStack:     stack,
			keyComponent: componentName,
			keyFolder:    folder,
			keyURI:       sourceSpec.Uri,
			keyVersion:   sourceSpec.Version,
		})
	}

	// Sort by component name for consistent output.
	sort.Slice(sources, func(i, j int) bool {
		return sources[i][keyComponent].(string) < sources[j][keyComponent].(string)
	})

	return sources
}

// extractSourcesFromAllStacks extracts sources from ALL stacks in stacksMap.
func extractSourcesFromAllStacks(stacksMap map[string]any, componentType string) []map[string]any {
	defer perf.Track(nil, "source.cmd.extractSourcesFromAllStacks")()

	var sources []map[string]any

	for stackName, stackData := range stacksMap {
		stackSources := extractSourcesFromSingleStackData(stackName, stackData, componentType)
		sources = append(sources, stackSources...)
	}

	sortSourcesByStackComponent(sources)
	return sources
}

// extractSourcesFromSingleStackData extracts sources from a single stack's data.
func extractSourcesFromSingleStackData(stackName string, stackData any, componentType string) []map[string]any {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil
	}

	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	typeComponents, ok := components[componentType].(map[string]any)
	if !ok {
		return nil
	}

	var sources []map[string]any
	for componentName, componentData := range typeComponents {
		entry := extractSingleSourceEntry(stackName, componentName, componentData)
		if entry != nil {
			sources = append(sources, entry)
		}
	}
	return sources
}

// extractSingleSourceEntry extracts a single source entry from component data.
func extractSingleSourceEntry(stackName, componentName string, componentData any) map[string]any {
	componentMap, ok := componentData.(map[string]any)
	if !ok {
		return nil
	}

	if !source.HasSource(componentMap) {
		return nil
	}

	sourceSpec, err := source.ExtractSource(componentMap)
	if err != nil || sourceSpec == nil {
		return nil
	}

	folder := extractComponentFolder(componentMap, componentName)

	return map[string]any{
		keyStack:     stackName,
		keyComponent: componentName,
		keyFolder:    folder,
		keyURI:       sourceSpec.Uri,
		keyVersion:   sourceSpec.Version,
	}
}

// sortSourcesByStackComponent sorts sources by stack, then component.
func sortSourcesByStackComponent(sources []map[string]any) {
	sort.Slice(sources, func(i, j int) bool {
		stackI := sources[i][keyStack].(string)
		stackJ := sources[j][keyStack].(string)
		if stackI != stackJ {
			return stackI < stackJ
		}
		return sources[i][keyComponent].(string) < sources[j][keyComponent].(string)
	})
}

// extractComponentFolder extracts the component folder from metadata.component.
// Falls back to the component name if not set.
func extractComponentFolder(componentMap map[string]any, componentName string) string {
	folder := componentName // Default to component name.
	if metadata, ok := componentMap["metadata"].(map[string]any); ok {
		if componentVal, ok := metadata[keyComponent].(string); ok && componentVal != "" {
			folder = componentVal
		}
	}
	return folder
}
