package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Constants for map keys.
const (
	keyStack     = "stack"
	keyType      = "type"
	keyComponent = "component"
	keyFolder    = "folder"
	keyURI       = "uri"
	keyVersion   = "version"
)

// Package-level function variables for testing.
var (
	initCliConfigForSources         = config.InitCliConfig
	executeDescribeStacksForSources = e.ExecuteDescribeStacks
)

var sourcesParser *flags.StandardParser

// SourcesOptions contains parsed flags for the sources command.
type SourcesOptions struct {
	global.Flags
	Format      string
	Stack       string
	Component   string // From positional arg.
	AtmosConfig *schema.AtmosConfiguration
	AuthManager auth.AuthManager
}

// sourcesCmd lists components with source configuration.
var sourcesCmd = &cobra.Command{
	Use:   "sources [component]",
	Short: "List components with source configuration",
	Long: `List all components that have source configured in stacks.

If component is specified, shows source info for that component across stacks.
If --stack is specified, limits to that stack.
If neither specified, lists all components with source across all stacks.

This shows which components can be vendored using the source provisioner.
Lists sources for all component types (terraform, helmfile, packer).`,
	Example: `  # List all sources across all stacks
  atmos list sources

  # List sources in a specific stack
  atmos list sources --stack dev

  # List sources for a specific component across all stacks
  atmos list sources vpc

  # List sources for a specific component in a specific stack
  atmos list sources vpc --stack dev

  # Output as JSON
  atmos list sources --format json`,
	Args: cobra.RangeArgs(0, 1),
	RunE: executeListSources,
}

func init() {
	// Create parser for sources command.
	sourcesParser = NewListParser(
		WithFormatFlag,
		WithStackFlag,
	)

	// Register flags for sources command.
	sourcesParser.RegisterFlags(sourcesCmd)

	// Add stack completion.
	addStackCompletion(sourcesCmd)

	// Bind flags to Viper for environment variable support.
	if err := sourcesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListSources(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "list.sources.RunE")()

	opts, err := initSourcesCommand(cmd, args)
	if err != nil {
		return err
	}

	sources, err := fetchAndFilterSources(opts)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		printNoSourcesMessage(opts)
		return nil
	}

	return renderSourcesOutput(sources, opts)
}

// initSourcesCommand initializes the sources command, parsing flags and config.
func initSourcesCommand(cmd *cobra.Command, args []string) (*SourcesOptions, error) {
	v := viper.GetViper()

	if err := checkAtmosConfig(cmd, v); err != nil {
		return nil, err
	}

	if err := sourcesParser.BindFlagsToViper(cmd, v); err != nil {
		return nil, err
	}

	opts := &SourcesOptions{
		Flags:  flags.ParseGlobalFlags(cmd, v),
		Format: v.GetString("format"),
		Stack:  v.GetString("stack"),
	}

	if len(args) > 0 {
		opts.Component = args[0]
	}

	configInfo := buildConfigAndStacksInfo(&opts.Flags)
	configInfo.Stack = opts.Stack
	atmosConfig, err := initCliConfigForSources(configInfo, true)
	if err != nil {
		return nil, wrapSourcesConfigError(err, opts.Stack)
	}
	opts.AtmosConfig = &atmosConfig

	authManager, err := createAuthManagerForSources(cmd, opts.AtmosConfig)
	if err != nil {
		return nil, err
	}
	opts.AuthManager = authManager

	return opts, nil
}

// fetchAndFilterSources fetches stack data and extracts filtered sources.
func fetchAndFilterSources(opts *SourcesOptions) ([]map[string]any, error) {
	stacksMap, err := executeDescribeStacksForSources(
		opts.AtmosConfig,
		opts.Stack,
		nil, nil, nil,
		false, false, false, false,
		nil, opts.AuthManager,
	)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrExecuteDescribeStacks).
			WithCause(err).
			WithContext(keyStack, opts.Stack).
			Err()
	}

	var sources []map[string]any
	if opts.Stack != "" {
		sources = extractAllSourcesFromStack(stacksMap, opts.Stack)
	} else {
		sources = extractAllSourcesFromAllStacks(stacksMap)
	}

	return filterSourcesByComponent(sources, opts.Component), nil
}

// printNoSourcesMessage prints an appropriate message when no sources are found.
func printNoSourcesMessage(opts *SourcesOptions) {
	msg := "No components with source configured"
	if opts.Stack != "" {
		msg += fmt.Sprintf(" in stack %q", opts.Stack)
	}
	if opts.Component != "" {
		msg += fmt.Sprintf(" matching component %q", opts.Component)
	}
	ui.Info(msg)
}

// renderSourcesOutput renders the sources with dynamic columns and sorting.
func renderSourcesOutput(sources []map[string]any, opts *SourcesOptions) error {
	hasFolderDiff := checkSourcesFolderDiffers(sources)
	columns := getSourcesListColumnsForContext(opts.Stack != "", hasFolderDiff)

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithCause(err).
			WithExplanation("Failed to create column selector").
			Err()
	}

	sorters := buildSourcesSorters(opts.Stack == "")
	r := renderer.New(nil, selector, sorters, format.Format(opts.Format), "")
	return r.Render(sources)
}

// buildSourcesSorters builds the sorters for sources output.
func buildSourcesSorters(includeStack bool) []*listSort.Sorter {
	var sorters []*listSort.Sorter
	if includeStack {
		sorters = append(sorters, listSort.NewSorter("Stack", listSort.Ascending))
	}
	sorters = append(sorters,
		listSort.NewSorter("Type", listSort.Ascending),
		listSort.NewSorter("Component", listSort.Ascending),
	)
	return sorters
}

// getSourcesListColumnsForContext returns the column configuration based on context.
// HasStack indicates whether --stack was provided (single stack mode).
// HasFolderDiff indicates whether any component has folder != component name.
func getSourcesListColumnsForContext(hasStack, hasFolderDiff bool) []column.Config {
	var columns []column.Config

	// Stack column only when showing multiple stacks.
	if !hasStack {
		columns = append(columns, column.Config{Name: "Stack", Value: "{{ .stack }}"})
	}

	// Type is always shown for sources command (multiple component types).
	columns = append(columns, column.Config{Name: "Type", Value: "{{ .type }}"})

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

// checkSourcesFolderDiffers returns true if any source has folder != component.
func checkSourcesFolderDiffers(sources []map[string]any) bool {
	for _, s := range sources {
		if s[keyComponent] != s[keyFolder] {
			return true
		}
	}
	return false
}

// filterSourcesByComponent filters sources matching component name OR folder.
func filterSourcesByComponent(sources []map[string]any, componentFilter string) []map[string]any {
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

// extractAllSourcesFromStack extracts components with source from all component types in stack data.
func extractAllSourcesFromStack(stacksMap map[string]any, stack string) []map[string]any {
	defer perf.Track(nil, "list.sources.extractAllSourcesFromStack")()

	stackData, ok := stacksMap[stack].(map[string]any)
	if !ok {
		return nil
	}

	sources := extractSourcesFromStackData(stack, stackData)
	sortSourcesByTypeComponent(sources)
	return sources
}

// sortSourcesByTypeComponent sorts sources by type, then component.
func sortSourcesByTypeComponent(sources []map[string]any) {
	sort.Slice(sources, func(i, j int) bool {
		typeI, _ := sources[i][keyType].(string)
		typeJ, _ := sources[j][keyType].(string)
		if typeI != typeJ {
			return typeI < typeJ
		}
		compI, _ := sources[i][keyComponent].(string)
		compJ, _ := sources[j][keyComponent].(string)
		return compI < compJ
	})
}

// extractAllSourcesFromAllStacks extracts sources from ALL stacks in stacksMap.
func extractAllSourcesFromAllStacks(stacksMap map[string]any) []map[string]any {
	defer perf.Track(nil, "list.sources.extractAllSourcesFromAllStacks")()

	var sources []map[string]any

	for stackName, stackData := range stacksMap {
		stackSources := extractSourcesFromStackData(stackName, stackData)
		sources = append(sources, stackSources...)
	}

	// Sort by stack, type, then component for consistent output.
	sortSourcesByStackTypeComponent(sources)
	return sources
}

// extractSourcesFromStackData extracts sources from a single stack's data.
func extractSourcesFromStackData(stackName string, stackData any) []map[string]any {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil
	}

	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	var sources []map[string]any
	componentTypes := []string{"terraform", "helmfile", "packer"}

	for _, componentType := range componentTypes {
		typeSources := extractSourcesFromComponentType(stackName, componentType, components)
		sources = append(sources, typeSources...)
	}
	return sources
}

// extractSourcesFromComponentType extracts sources from a specific component type.
func extractSourcesFromComponentType(stackName, componentType string, components map[string]any) []map[string]any {
	typeComponents, ok := components[componentType].(map[string]any)
	if !ok {
		return nil
	}

	var sources []map[string]any
	for componentName, componentData := range typeComponents {
		sourceEntry := extractSourceEntry(stackName, componentType, componentName, componentData)
		if sourceEntry != nil {
			sources = append(sources, sourceEntry)
		}
	}
	return sources
}

// extractSourceEntry extracts a single source entry from component data.
func extractSourceEntry(stackName, componentType, componentName string, componentData any) map[string]any {
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

	folder := extractSourceComponentFolder(componentMap, componentName)

	return map[string]any{
		keyStack:     stackName,
		keyType:      componentType,
		keyComponent: componentName,
		keyFolder:    folder,
		keyURI:       sourceSpec.Uri,
		keyVersion:   sourceSpec.Version,
	}
}

// sortSourcesByStackTypeComponent sorts sources by stack, type, then component.
func sortSourcesByStackTypeComponent(sources []map[string]any) {
	sort.Slice(sources, func(i, j int) bool {
		stackI, _ := sources[i][keyStack].(string)
		stackJ, _ := sources[j][keyStack].(string)
		if stackI != stackJ {
			return stackI < stackJ
		}
		typeI, _ := sources[i][keyType].(string)
		typeJ, _ := sources[j][keyType].(string)
		if typeI != typeJ {
			return typeI < typeJ
		}
		compI, _ := sources[i][keyComponent].(string)
		compJ, _ := sources[j][keyComponent].(string)
		return compI < compJ
	})
}

// extractSourceComponentFolder extracts the component folder from metadata.component.
// Falls back to the component name if not set.
func extractSourceComponentFolder(componentMap map[string]any, componentName string) string {
	folder := componentName // Default to component name.
	if metadata, ok := componentMap["metadata"].(map[string]any); ok {
		if componentVal, ok := metadata[keyComponent].(string); ok && componentVal != "" {
			folder = componentVal
		}
	}
	return folder
}

// createAuthManagerForSources creates an auth manager for the sources command.
func createAuthManagerForSources(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) (auth.AuthManager, error) {
	defer perf.Track(atmosConfig, "list.sources.createAuthManagerForSources")()

	identity, _ := cmd.Flags().GetString("identity")
	if identity == "" {
		return nil, nil
	}

	authManager, err := auth.CreateAndAuthenticateManager(identity, &atmosConfig.Auth, config.IdentityFlagSelectValue)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithCause(err).
			WithContext("identity", identity).
			Err()
	}

	return authManager, nil
}

// wrapSourcesConfigError wraps configuration errors with user-friendly messages.
func wrapSourcesConfigError(err error, stack string) error {
	errMsg := err.Error()

	// Detect "no stacks found" pattern from import failures.
	// The context "paths" is automatically included from the source error.
	if strings.Contains(errMsg, "failed to find import") ||
		strings.Contains(errMsg, "no files match") {
		return errUtils.Build(errUtils.ErrNoStacksFound).
			WithCause(err).
			WithExplanation("No stack configuration files were found matching the configured import patterns").
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
