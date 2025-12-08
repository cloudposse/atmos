package list

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/importresolver"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Default columns for list instances if not specified in atmos.yaml.
var defaultInstanceColumns = []column.Config{
	{Name: "Component", Value: "{{ .component }}"},
	{Name: "Stack", Value: "{{ .stack }}"},
}

// InstancesCommandOptions contains options for the list instances command.
type InstancesCommandOptions struct {
	Info        *schema.ConfigAndStacksInfo
	Cmd         *cobra.Command
	Args        []string
	ShowImports bool
	ColumnsFlag []string
	FilterSpec  string
	SortSpec    string
	Delimiter   string
	Query       string
}

// parseColumnsFlag parses column specifications from CLI flag.
// Each flag value should be in the format: "Name=TemplateExpression"
// Example: --columns "Component={{ .component }}" --columns "Stack={{ .stack }}"
// Returns error if any column specification is invalid.
func parseColumnsFlag(columnsFlag []string) ([]column.Config, error) {
	if len(columnsFlag) == 0 {
		return defaultInstanceColumns, nil
	}

	columns := make([]column.Config, 0, len(columnsFlag))
	for i, spec := range columnsFlag {
		// Split on first '=' to separate name from template
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: column spec %d must be in format 'Name=Template', got: %q",
				errUtils.ErrInvalidConfig, i+1, spec)
		}

		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if name == "" {
			return nil, fmt.Errorf("%w: column spec %d has empty name", errUtils.ErrInvalidConfig, i+1)
		}
		if value == "" {
			return nil, fmt.Errorf("%w: column spec %d has empty template", errUtils.ErrInvalidConfig, i+1)
		}

		columns = append(columns, column.Config{
			Name:  name,
			Value: value,
		})
	}

	return columns, nil
}

// processComponentConfig processes a single component configuration and returns an instance if valid.
func processComponentConfig(stackName, componentName, componentType string, componentConfig interface{}) *schema.Instance {
	componentConfigMap, ok := componentConfig.(map[string]any)
	if !ok {
		return nil
	}
	return createInstance(stackName, componentName, componentType, componentConfigMap)
}

// processComponentType processes all components of a specific type in a stack.
func processComponentType(stackName, componentType string, typeComponents interface{}) []schema.Instance {
	typeComponentsMap, ok := typeComponents.(map[string]any)
	if !ok {
		return nil
	}

	var instances []schema.Instance
	for componentName, componentConfig := range typeComponentsMap {
		if instance := processComponentConfig(stackName, componentName, componentType, componentConfig); instance != nil {
			instances = append(instances, *instance)
		}
	}
	return instances
}

// processStackComponents processes all components in a stack.
func processStackComponents(stackName string, stackConfig interface{}) []schema.Instance {
	stackConfigMap, ok := stackConfig.(map[string]any)
	if !ok {
		return nil
	}

	components, ok := stackConfigMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	var instances []schema.Instance
	for componentType, typeComponents := range components {
		if typeInstances := processComponentType(stackName, componentType, typeComponents); typeInstances != nil {
			instances = append(instances, typeInstances...)
		}
	}
	return instances
}

// collectInstances collects all instances from the stacks map.
func collectInstances(stacksMap map[string]interface{}) []schema.Instance {
	var instances []schema.Instance
	for stackName, stackConfig := range stacksMap {
		if stackInstances := processStackComponents(stackName, stackConfig); stackInstances != nil {
			instances = append(instances, stackInstances...)
		}
	}
	return instances
}

// createInstance creates an instance from the component configuration.
func createInstance(stackName, componentName, componentType string, componentConfigMap map[string]any) *schema.Instance {
	instance := &schema.Instance{
		Component:     componentName,
		Stack:         stackName,
		ComponentType: componentType,
		Settings:      make(map[string]any),
		Vars:          make(map[string]any),
		Env:           make(map[string]any),
		Backend:       make(map[string]any),
		Metadata:      make(map[string]any),
	}

	if settings, ok := componentConfigMap["settings"].(map[string]any); ok {
		instance.Settings = settings
	}
	if vars, ok := componentConfigMap["vars"].(map[string]any); ok {
		instance.Vars = vars
	}
	if env, ok := componentConfigMap["env"].(map[string]any); ok {
		instance.Env = env
	}
	if backend, ok := componentConfigMap["backend"].(map[string]any); ok {
		instance.Backend = backend
	}
	if metadata, ok := componentConfigMap["metadata"].(map[string]any); ok {
		instance.Metadata = metadata
	}

	// Skip abstract components.
	if metadataType, ok := instance.Metadata["type"].(string); ok && metadataType == "abstract" {
		return nil
	}

	return instance
}

// isProDriftDetectionEnabled checks if an instance has Atmos Pro drift detection enabled.
// Returns true if settings.pro.drift_detection.enabled == true and settings.pro.enabled != false.
func isProDriftDetectionEnabled(instance *schema.Instance) bool {
	proSettings, ok := instance.Settings["pro"].(map[string]any)
	if !ok {
		return false
	}

	// Skip if pro is explicitly disabled
	if proEnabled, ok := proSettings["enabled"].(bool); ok && !proEnabled {
		return false
	}

	driftDetection, ok := proSettings["drift_detection"].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := driftDetection["enabled"].(bool)
	return ok && enabled
}

// filterProEnabledInstances returns only instances that have Atmos Pro drift detection explicitly enabled
// via settings.pro.drift_detection.enabled == true, but excludes instances where settings.pro.enabled == false.
func filterProEnabledInstances(instances []schema.Instance) []schema.Instance {
	filtered := make([]schema.Instance, 0, len(instances))
	for i := range instances {
		if isProDriftDetectionEnabled(&instances[i]) {
			filtered = append(filtered, instances[i])
		}
	}
	return filtered
}

// sortInstances sorts instances by stack and component.
func sortInstances(instances []schema.Instance) []schema.Instance {
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].Stack != instances[j].Stack {
			return instances[i].Stack < instances[j].Stack
		}
		return instances[i].Component < instances[j].Component
	})
	return instances
}

// getInstanceColumns returns column configuration from CLI flag, atmos.yaml, or defaults.
// Returns error if CLI flag parsing fails.
func getInstanceColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string) ([]column.Config, error) {
	// If --columns flag is provided, parse it and return.
	if len(columnsFlag) > 0 {
		columns, err := parseColumnsFlag(columnsFlag)
		if err != nil {
			return nil, err
		}
		return columns, nil
	}

	// Check if custom columns are configured in atmos.yaml.
	if len(atmosConfig.Components.List.Columns) > 0 {
		columns := make([]column.Config, len(atmosConfig.Components.List.Columns))
		for i, col := range atmosConfig.Components.List.Columns {
			columns[i] = column.Config{
				Name:  col.Name,
				Value: col.Value,
				Width: col.Width,
			}
		}
		return columns, nil
	}

	// Return default columns.
	return defaultInstanceColumns, nil
}

// uploadInstancesWithDeps uploads instances to Atmos Pro API using injected dependencies.
// This function is testable via mocks. Use uploadInstances() for production code.
func uploadInstancesWithDeps(
	instances []schema.Instance,
	gitOps git.RepositoryOperations,
	configLoader cfg.Loader,
	clientFactory pro.ClientFactory,
) error {
	repo, err := gitOps.GetLocalRepo()
	if err != nil {
		log.Error(errUtils.ErrFailedToGetLocalRepo.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToGetLocalRepo, err)
	}

	repoInfo, err := gitOps.GetRepoInfo(repo)
	if err != nil {
		log.Error(errUtils.ErrFailedToGetRepoInfo.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToGetRepoInfo, err)
	}

	if repoInfo.RepoUrl == "" || repoInfo.RepoName == "" || repoInfo.RepoOwner == "" || repoInfo.RepoHost == "" {
		log.Warn("Git repo info is incomplete; upload may be rejected.", "repo_url", repoInfo.RepoUrl, "repo_name", repoInfo.RepoName, "repo_owner", repoInfo.RepoOwner, "repo_host", repoInfo.RepoHost)
	}

	// Initialize CLI config for API client.
	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := configLoader.InitCliConfig(&configInfo, false)
	if err != nil {
		log.Error(errUtils.ErrFailedToInitConfig.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	apiClient, err := clientFactory.NewClient(&atmosConfig)
	if err != nil {
		log.Error(errUtils.ErrFailedToCreateAPIClient.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToCreateAPIClient, err)
	}

	req := dtos.InstancesUploadRequest{
		RepoURL:   repoInfo.RepoUrl,
		RepoName:  repoInfo.RepoName,
		RepoOwner: repoInfo.RepoOwner,
		RepoHost:  repoInfo.RepoHost,
		Instances: instances,
	}

	err = apiClient.UploadInstances(&req)
	if err != nil {
		log.Error(errUtils.ErrFailedToUploadInstances.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToUploadInstances, err)
	}

	u.PrintfMessageToTUI("Successfully uploaded instances to Atmos Pro API.")
	return nil
}

// uploadInstances uploads instances to Atmos Pro API.
// This is a convenience wrapper around uploadInstancesWithDeps() for production use.
func uploadInstances(instances []schema.Instance) error {
	return uploadInstancesWithDeps(
		instances,
		&git.DefaultRepositoryOperations{},
		&cfg.DefaultLoader{},
		&pro.DefaultClientFactory{},
	)
}

// processInstancesWithDeps collects, filters, and sorts instances using injected dependencies.
// This function is testable via mocks. Use processInstances() for production code.
func processInstancesWithDeps(
	atmosConfig *schema.AtmosConfiguration,
	stacksProcessor e.StacksProcessor,
) ([]schema.Instance, error) {
	// Get all stacks with template processing enabled to render template variables.
	stacksMap, err := stacksProcessor.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, true, false, nil, nil)
	if err != nil {
		log.Error(errUtils.ErrExecuteDescribeStacks.Error(), "error", err)
		return nil, errors.Join(errUtils.ErrExecuteDescribeStacks, err)
	}

	// Collect instances.
	instances := collectInstances(stacksMap)

	// Sort instances.
	instances = sortInstances(instances)

	return instances, nil
}

// processInstances collects, filters, and sorts instances.
// This is a convenience wrapper around processInstancesWithDeps() for production use.
func processInstances(atmosConfig *schema.AtmosConfiguration) ([]schema.Instance, error) {
	return processInstancesWithDeps(atmosConfig, &e.DefaultStacksProcessor{})
}

// ExecuteListInstancesCmd executes the list instances command.
//
//nolint:revive,cyclop,funlen // Complexity and length from format branching and upload handling (unavoidable pattern).
func ExecuteListInstancesCmd(opts *InstancesCommandOptions) error {
	log.Trace("ExecuteListInstancesCmd starting")
	// Initialize CLI config.
	atmosConfig, err := cfg.InitCliConfig(*opts.Info, true)
	if err != nil {
		log.Error(errUtils.ErrFailedToInitConfig.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	// Get flags.
	upload, err := opts.Cmd.Flags().GetBool("upload")
	if err != nil {
		log.Error(errUtils.ErrParseFlag.Error(), "flag", "upload", "error", err)
		return errors.Join(errUtils.ErrParseFlag, err)
	}

	formatFlag, err := opts.Cmd.Flags().GetString("format")
	if err != nil {
		log.Error(errUtils.ErrParseFlag.Error(), "flag", "format", "error", err)
		return errors.Join(errUtils.ErrParseFlag, err)
	}

	// Handle tree format specially - branch before calling processInstances to avoid double processing.
	log.Trace("Checking format flag", "format_flag", formatFlag, "format_tree", format.FormatTree, "match", formatFlag == string(format.FormatTree))
	if formatFlag == string(format.FormatTree) {
		// Tree format does not support --upload.
		if upload {
			return fmt.Errorf("%w: --upload is not supported with --format=tree", errUtils.ErrInvalidFlag)
		}

		// Enable provenance tracking to capture import chains.
		atmosConfig.TrackProvenance = true

		// Clear caches to ensure fresh processing with provenance enabled.
		e.ClearMergeContexts()
		e.ClearFindStacksMapCache()

		// Get all stacks for provenance-based import resolution (single call).
		stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
		if err != nil {
			log.Error(errUtils.ErrExecuteDescribeStacks.Error(), "error", err)
			return errors.Join(errUtils.ErrExecuteDescribeStacks, err)
		}

		// Resolve import trees using provenance system.
		importTrees, err := importresolver.ResolveImportTreeFromProvenance(stacksMap, &atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to resolve import trees: %w", err)
		}

		// Render tree view.
		// Use showImports parameter from --provenance flag.
		output := format.RenderInstancesTree(importTrees, opts.ShowImports)
		fmt.Println(output)
		return nil
	}

	// For non-tree formats, process instances normally.
	instances, err := processInstances(&atmosConfig)
	if err != nil {
		log.Error(errUtils.ErrProcessInstances.Error(), "error", err)
		return errors.Join(errUtils.ErrProcessInstances, err)
	}

	// Extract instances into renderer-compatible format with metadata fields.
	data := extract.Metadata(instances)

	// Get column configuration.
	columns, err := getInstanceColumns(&atmosConfig, opts.ColumnsFlag)
	if err != nil {
		log.Error("failed to get columns", "error", err)
		return errors.Join(errUtils.ErrInvalidConfig, err)
	}

	// Create column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("failed to create column selector: %w", err)
	}

	// Build filters from filter specification.
	filters, err := buildInstanceFilters(opts.FilterSpec)
	if err != nil {
		return fmt.Errorf("failed to build filters: %w", err)
	}

	// Build sorters from sort specification.
	// Pass columns to allow smart default sorting based on available columns.
	sorters, err := buildInstanceSorters(opts.SortSpec, columns)
	if err != nil {
		return fmt.Errorf("failed to build sorters: %w", err)
	}

	// Create renderer.
	r := renderer.New(filters, selector, sorters, format.Format(formatFlag), opts.Delimiter)

	// Render output.
	if err := r.Render(data); err != nil {
		return fmt.Errorf("failed to render instances: %w", err)
	}

	// Handle upload if requested.
	if upload {
		proInstances := filterProEnabledInstances(instances)
		if len(proInstances) == 0 {
			_ = ui.Info("No Atmos Pro-enabled instances found; nothing to upload.")
			return nil
		}
		return uploadInstances(proInstances)
	}

	return nil
}

// buildInstanceFilters creates filters from filter specification.
// The filter spec format is currently undefined for instances,
// so this returns an empty filter list for now.
func buildInstanceFilters(filterSpec string) ([]filter.Filter, error) {
	// TODO: Implement filter parsing when filter spec format is defined.
	// For now, return empty filter list.
	return nil, nil
}

// buildInstanceSorters creates sorters from sort specification.
// When sortSpec is empty and columns contain default "Component" and "Stack",
// applies default sorting. Otherwise returns empty sorters (natural order).
func buildInstanceSorters(sortSpec string, columns []column.Config) ([]*listSort.Sorter, error) {
	// If user provided explicit sort spec, use it.
	if sortSpec != "" {
		return listSort.ParseSortSpec(sortSpec)
	}

	// Build map of available column names.
	columnNames := make(map[string]bool)
	for _, col := range columns {
		columnNames[col.Name] = true
	}

	// Only apply default sort if both Component and Stack columns exist.
	if columnNames["Component"] && columnNames["Stack"] {
		return []*listSort.Sorter{
			listSort.NewSorter("Component", listSort.Ascending),
			listSort.NewSorter("Stack", listSort.Ascending),
		}, nil
	}

	// No default sort for custom columns - return empty sorters (natural order).
	return nil, nil
}
