package list

import (
	"fmt"
	"os"
	"sort"

	log "github.com/charmbracelet/log"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// collectDeployments collects all deployments from the stacks map.
func collectDeployments(stacksMap map[string]interface{}) []schema.Deployment {
	deployments := []schema.Deployment{}
	for stackName, stackConfig := range stacksMap {
		stackConfigMap, ok := stackConfig.(map[string]any)
		if !ok {
			continue
		}

		components, ok := stackConfigMap["components"].(map[string]any)
		if !ok {
			continue
		}

		for componentType, typeComponents := range components {
			typeComponentsMap, ok := typeComponents.(map[string]any)
			if !ok {
				continue
			}

			for componentName, componentConfig := range typeComponentsMap {
				componentConfigMap, ok := componentConfig.(map[string]any)
				if !ok {
					continue
				}

				deployment := createDeployment(stackName, componentName, componentType, componentConfigMap)
				if deployment != nil {
					deployments = append(deployments, *deployment)
				}
			}
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

	// Skip abstract components
	if componentType, ok := deployment.Metadata["type"].(string); ok && componentType == "abstract" {
		return nil
	}

	return deployment
}

// filterDeployments filters deployments based on drift detection.
func filterDeployments(deployments []schema.Deployment, driftEnabled bool) []schema.Deployment {
	if !driftEnabled {
		return deployments
	}

	filtered := []schema.Deployment{}
	for _, deployment := range deployments {
		if settings, ok := deployment.Settings["pro"].(map[string]any); ok {
			if driftDetection, ok := settings["drift_detection"].(map[string]any); ok {
				if enabled, ok := driftDetection["enabled"].(bool); ok && enabled {
					filtered = append(filtered, deployment)
				}
			}
		}
	}
	return filtered
}

// sortDeployments sorts deployments by stack and component.
func sortDeployments(deployments []schema.Deployment) []schema.Deployment {
	type deploymentRow struct {
		Component string
		Stack     string
		Index     int // Add index to track original position
	}
	rowsData := make([]deploymentRow, 0, len(deployments))
	for i, d := range deployments {
		rowsData = append(rowsData, deploymentRow{Component: d.Component, Stack: d.Stack, Index: i})
	}
	sort.Slice(rowsData, func(i, j int) bool {
		if rowsData[i].Stack != rowsData[j].Stack {
			return rowsData[i].Stack < rowsData[j].Stack
		}
		return rowsData[i].Component < rowsData[j].Component
	})

	// Create a new slice with sorted deployments
	sortedDeployments := make([]schema.Deployment, len(deployments))
	for i, row := range rowsData {
		sortedDeployments[i] = deployments[row.Index]
	}
	return sortedDeployments
}

// formatDeployments formats the deployments for output.
func formatDeployments(deployments []schema.Deployment) string {
	var rows []map[string]interface{}
	for _, d := range deployments {
		rows = append(rows, map[string]interface{}{
			"Component": d.Component,
			"Stack":     d.Stack,
		})
	}

	formatOpts := format.FormatOptions{
		Format:        format.FormatTable,
		TTY:           term.IsTerminal(int(os.Stdout.Fd())),
		CustomHeaders: []string{"Component", "Stack"},
		MaxColumns:    0,
	}

	return format.CreateStyledTable(formatOpts.CustomHeaders, func() [][]string {
		var tableRows [][]string
		for _, row := range rows {
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%v", row["Component"]),
				fmt.Sprintf("%v", row["Stack"]),
			})
		}
		return tableRows
	}())
}

// uploadDeployments uploads deployments to Atmos Pro API.
func uploadDeployments(deployments []schema.Deployment, atmosConfig schema.AtmosConfiguration) error {
	repo, err := git.GetLocalRepo()
	if err != nil {
		log.Error("Failed to get local git repo", "error", err)
		return err
	}
	repoInfo, err := git.GetRepoInfo(repo)
	if err != nil {
		log.Error("Failed to get git repo info", "error", err)
		return err
	}

	logger, err := logger.NewLoggerFromCliConfig(atmosConfig)
	if err != nil {
		log.Error("Failed to create logger", "error", err)
		return err
	}

	apiClient, err := pro.NewAtmosProAPIClientFromEnv(logger)
	if err != nil {
		log.Error("Failed to create Atmos Pro API client", "error", err)
		return err
	}

	req := pro.DriftDetectionUploadRequest{
		RepoURL:   repoInfo.RepoUrl,
		RepoName:  repoInfo.RepoName,
		RepoOwner: repoInfo.RepoOwner,
		RepoHost:  repoInfo.RepoHost,
		Stacks:    deployments,
	}

	err = apiClient.UploadDriftDetection(&req)
	if err != nil {
		log.Error("Failed to upload deployments", "error", err)
		return err
	}

	logger.Info("Successfully uploaded deployments to Atmos Pro API.")
	return nil
}

// ExecuteListDeploymentsCmd executes the list deployments command.
func ExecuteListDeploymentsCmd(info schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string) error {
	// Initialize CLI config
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	// Get drift detection filter
	driftEnabled := cmd.Flags().Changed("drift-enabled")

	// Get upload flag
	upload := cmd.Flags().Changed("upload")

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return err
	}

	// Collect deployments
	deployments := collectDeployments(stacksMap)

	// Filter deployments if drift detection is enabled
	deployments = filterDeployments(deployments, driftEnabled)

	// Sort deployments
	deployments = sortDeployments(deployments)

	// Format and print output
	output := formatDeployments(deployments)
	fmt.Println(output)

	// Upload deployments if requested
	if !upload {
		return nil
	}

	if !driftEnabled {
		log.Info("Atmos Pro only supports uploading drift detection stacks at this time.\n\nTo upload drift detection stacks, use the --drift-enabled flag:\n  atmos list deployments --upload --drift-enabled")
		return nil
	}

	if err := uploadDeployments(deployments, atmosConfig); err != nil {
		return err
	}

	return nil
}
