package list

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	term "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/list/format"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	componentHeader = "Component"
	stackHeader     = "Stack"
)

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

// formatInstances formats the instances for output.
func formatInstances(instances []schema.Instance) string {
	formatOpts := format.FormatOptions{
		TTY:           term.IsTTYSupportForStdout(),
		CustomHeaders: []string{componentHeader, stackHeader},
	}

	// If not in a TTY environment, output CSV.
	if !formatOpts.TTY {
		var output strings.Builder
		csvWriter := csv.NewWriter(&output)
		if err := csvWriter.Write([]string{componentHeader, stackHeader}); err != nil {
			return ""
		}
		for _, i := range instances {
			if err := csvWriter.Write([]string{i.Component, i.Stack}); err != nil {
				return ""
			}
		}
		csvWriter.Flush()
		if err := csvWriter.Error(); err != nil {
			log.Error(errUtils.ErrFailedToFinalizeCSVOutput.Error(), "error", err)
			return ""
		}
		return output.String()
	}

	// For TTY mode, create a styled table with only Component and Stack columns.
	tableRows := make([][]string, 0, len(instances))
	for _, i := range instances {
		row := []string{i.Component, i.Stack}
		tableRows = append(tableRows, row)
	}

	return format.CreateStyledTable(formatOpts.CustomHeaders, tableRows)
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
	authManager auth.AuthManager,
) ([]schema.Instance, error) {
	// Get all stacks with template processing enabled to render template variables.
	stacksMap, err := stacksProcessor.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, true, false, nil, authManager)
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
func processInstances(atmosConfig *schema.AtmosConfiguration, authManager auth.AuthManager) ([]schema.Instance, error) {
	return processInstancesWithDeps(atmosConfig, &e.DefaultStacksProcessor{}, authManager)
}

// ExecuteListInstancesCmd executes the list instances command.
func ExecuteListInstancesCmd(info *schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string, authManager auth.AuthManager) error {
	// Inline initializeConfig.
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		log.Error(errUtils.ErrFailedToInitConfig.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	// Get flags.
	upload, err := cmd.Flags().GetBool("upload")
	if err != nil {
		log.Error(errUtils.ErrParseFlag.Error(), "flag", "upload", "error", err)
		return errors.Join(errUtils.ErrParseFlag, err)
	}

	// Process instances.
	instances, err := processInstances(&atmosConfig, authManager)
	if err != nil {
		log.Error(errUtils.ErrProcessInstances.Error(), "error", err)
		return errors.Join(errUtils.ErrProcessInstances, err)
	}

	// Inline handleOutput.
	output := formatInstances(instances)
	fmt.Fprint(os.Stdout, output)

	// Handle upload if requested.
	if upload {
		proInstances := filterProEnabledInstances(instances)
		if len(proInstances) == 0 {
			u.PrintfMessageToTUI("No Atmos Pro-enabled instances found; nothing to upload.")
			return nil
		}
		return uploadInstances(proInstances)
	}

	return nil
}
