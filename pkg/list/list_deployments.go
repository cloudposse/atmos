package list

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	componentHeader = "Component"
	stackHeader     = "Stack"
)

// Static error definitions for deployment operations.
var (
	ErrGetLocalRepo          = errors.New("failed to get local repo")
	ErrGetRepoInfo           = errors.New("failed to get repo info")
	ErrInitCliConfig         = errors.New("failed to initialize CLI config")
	ErrCreateAPIClient       = errors.New("failed to create API client")
	ErrUploadDeployments     = errors.New("failed to upload deployments")
	ErrExecuteDescribeStacks = errors.New("failed to execute describe stacks")
	ErrProcessDeployments    = errors.New("failed to process deployments")
	ErrParseFlag             = errors.New("failed to parse flag")
)

// processComponentConfig processes a single component configuration and returns a deployment if valid.
func processComponentConfig(stackName, componentName, componentType string, componentConfig interface{}) *schema.Deployment {
	componentConfigMap, ok := componentConfig.(map[string]any)
	if !ok {
		return nil
	}
	return createDeployment(stackName, componentName, componentType, componentConfigMap)
}

// processComponentType processes all components of a specific type in a stack.
func processComponentType(stackName, componentType string, typeComponents interface{}) []schema.Deployment {
	typeComponentsMap, ok := typeComponents.(map[string]any)
	if !ok {
		return nil
	}

	var deployments []schema.Deployment
	for componentName, componentConfig := range typeComponentsMap {
		if deployment := processComponentConfig(stackName, componentName, componentType, componentConfig); deployment != nil {
			deployments = append(deployments, *deployment)
		}
	}
	return deployments
}

// processStackComponents processes all components in a stack.
func processStackComponents(stackName string, stackConfig interface{}) []schema.Deployment {
	stackConfigMap, ok := stackConfig.(map[string]any)
	if !ok {
		return nil
	}

	components, ok := stackConfigMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	var deployments []schema.Deployment
	for componentType, typeComponents := range components {
		if typeDeployments := processComponentType(stackName, componentType, typeComponents); typeDeployments != nil {
			deployments = append(deployments, typeDeployments...)
		}
	}
	return deployments
}

// collectDeployments collects all deployments from the stacks map.
func collectDeployments(stacksMap map[string]interface{}) []schema.Deployment {
	var deployments []schema.Deployment
	for stackName, stackConfig := range stacksMap {
		if stackDeployments := processStackComponents(stackName, stackConfig); stackDeployments != nil {
			deployments = append(deployments, stackDeployments...)
		}
	}
	return deployments
}

// createDeployment creates a deployment from the component configuration.
func createDeployment(stackName, componentName, componentType string, componentConfigMap map[string]any) *schema.Deployment {
	deployment := &schema.Deployment{
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
		deployment.Settings = settings
	}
	if vars, ok := componentConfigMap["vars"].(map[string]any); ok {
		deployment.Vars = vars
	}
	if env, ok := componentConfigMap["env"].(map[string]any); ok {
		deployment.Env = env
	}
	if backend, ok := componentConfigMap["backend"].(map[string]any); ok {
		deployment.Backend = backend
	}
	if metadata, ok := componentConfigMap["metadata"].(map[string]any); ok {
		deployment.Metadata = metadata
	}

	// Skip abstract components.
	if metadataType, ok := deployment.Metadata["type"].(string); ok && metadataType == "abstract" {
		return nil
	}

	return deployment
}

// isProDriftDetectionEnabled checks if a deployment has Atmos Pro drift detection enabled.
// Returns true if settings.pro.drift_detection.enabled == true and settings.pro.enabled != false.
func isProDriftDetectionEnabled(deployment *schema.Deployment) bool {
	proSettings, ok := deployment.Settings["pro"].(map[string]any)
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

// filterProEnabledDeployments returns only deployments that have Atmos Pro drift detection explicitly enabled.
// via settings.pro.drift_detection.enabled == true, but excludes deployments where settings.pro.enabled == false.
func filterProEnabledDeployments(deployments []schema.Deployment) []schema.Deployment {
	filtered := make([]schema.Deployment, 0, len(deployments))
	for _, deployment := range deployments {
		if isProDriftDetectionEnabled(&deployment) {
			filtered = append(filtered, deployment)
		}
	}
	return filtered
}

// sortDeployments sorts deployments by stack and component.
func sortDeployments(deployments []schema.Deployment) []schema.Deployment {
	sort.SliceStable(deployments, func(i, j int) bool {
		if deployments[i].Stack != deployments[j].Stack {
			return deployments[i].Stack < deployments[j].Stack
		}
		return deployments[i].Component < deployments[j].Component
	})
	return deployments
}

// formatDeployments formats the deployments for output.
func formatDeployments(deployments []schema.Deployment) string {
	formatOpts := format.FormatOptions{
		TTY:           term.IsTerminal(int(os.Stdout.Fd())),
		CustomHeaders: []string{componentHeader, stackHeader},
	}

	// If not in a TTY environment, output CSV.
	if !formatOpts.TTY {
		var output strings.Builder
		csvWriter := csv.NewWriter(&output)
		if err := csvWriter.Write([]string{componentHeader, stackHeader}); err != nil {
			return ""
		}
		for _, d := range deployments {
			if err := csvWriter.Write([]string{d.Component, d.Stack}); err != nil {
				return ""
			}
		}
		csvWriter.Flush()
		return output.String()
	}

	// For TTY mode, create a styled table with only Component and Stack columns.
	var tableRows [][]string
	for _, d := range deployments {
		row := []string{d.Component, d.Stack}
		tableRows = append(tableRows, row)
	}

	return format.CreateStyledTable(formatOpts.CustomHeaders, tableRows)
}

// uploadDeployments uploads deployments to Atmos Pro API.
func uploadDeployments(deployments []schema.Deployment) error {
	repo, err := git.GetLocalRepo()
	if err != nil {
		log.Error(ErrGetLocalRepo.Error(), "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrGetLocalRepo, err)
	}
	repoInfo, err := git.GetRepoInfo(repo)
	if err != nil {
		log.Error(ErrGetRepoInfo.Error(), "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrGetRepoInfo, err)
	}

	// Initialize CLI config for API client.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		log.Error(ErrInitCliConfig.Error(), "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrInitCliConfig, err)
	}

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(&atmosConfig)
	if err != nil {
		log.Error(ErrCreateAPIClient.Error(), "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrCreateAPIClient, err)
	}

	req := dtos.DeploymentsUploadRequest{
		RepoURL:     repoInfo.RepoUrl,
		RepoName:    repoInfo.RepoName,
		RepoOwner:   repoInfo.RepoOwner,
		RepoHost:    repoInfo.RepoHost,
		Deployments: deployments,
	}

	err = apiClient.UploadDeployments(&req)
	if err != nil {
		log.Error(ErrUploadDeployments.Error(), "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrUploadDeployments, err)
	}

	log.Info("Successfully uploaded deployments to Atmos Pro API.")
	return nil
}

// processDeployments collects, filters, and sorts deployments.
func processDeployments(atmosConfig *schema.AtmosConfiguration) ([]schema.Deployment, error) {
	// Get all stacks with template processing enabled to render template variables.
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, true, false, nil)
	if err != nil {
		log.Error(ErrExecuteDescribeStacks.Error(), "error", err)
		return nil, fmt.Errorf(errUtils.ErrWrappingFormat, ErrExecuteDescribeStacks, err)
	}

	// Collect deployments.
	deployments := collectDeployments(stacksMap)

	// Sort deployments.
	deployments = sortDeployments(deployments)

	return deployments, nil
}

// ExecuteListDeploymentsCmd executes the list deployments command.
func ExecuteListDeploymentsCmd(info *schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string) error {
	// Inline initializeConfig.
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		log.Error(ErrInitCliConfig.Error(), "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrInitCliConfig, err)
	}

	// Get flags.
	upload, err := cmd.Flags().GetBool("upload")
	if err != nil {
		log.Error(ErrParseFlag.Error(), "flag", "upload", "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrParseFlag, err)
	}

	// Process deployments.
	deployments, err := processDeployments(&atmosConfig)
	if err != nil {
		log.Error(ErrProcessDeployments.Error(), "error", err)
		return fmt.Errorf(errUtils.ErrWrappingFormat, ErrProcessDeployments, err)
	}

	// Inline handleOutput.
	output := formatDeployments(deployments)
	fmt.Fprint(os.Stdout, output)

	// Handle upload if requested.
	if upload {
		proDeployments := filterProEnabledDeployments(deployments)
		if len(proDeployments) == 0 {
			log.Info("No Atmos Pro-enabled deployments found; nothing to upload.")
			return nil
		}
		return uploadDeployments(proDeployments)
	}

	return nil
}
