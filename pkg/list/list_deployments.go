package list

import (
	"fmt"
	"os"

	log "github.com/charmbracelet/log"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"golang.org/x/term"
)

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

	// Get all deployments
	deployments := []schema.Deployment{}
	for stackName, stackConfig := range stacksMap {
		stackConfigMap, ok := stackConfig.(map[string]any)
		if !ok {
			continue
		}

		// Get components from stack
		components, ok := stackConfigMap["components"].(map[string]any)
		if !ok {
			continue
		}

		// Process each component type (terraform, helmfile)
		for componentType, typeComponents := range components {
			typeComponentsMap, ok := typeComponents.(map[string]any)
			if !ok {
				continue
			}

			// Process each component in the stack
			for componentName, componentConfig := range typeComponentsMap {
				componentConfigMap, ok := componentConfig.(map[string]any)
				if !ok {
					continue
				}

				// Create deployment
				deployment := schema.Deployment{
					Component:     componentName,
					Stack:         stackName,
					ComponentType: componentType,
					Settings:      make(map[string]any),
					Vars:          make(map[string]any),
					Env:           make(map[string]any),
					Backend:       make(map[string]any),
					Metadata:      make(map[string]any),
				}

				// Copy component configuration
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
					continue
				}

				// Filter by drift detection if enabled
				if driftEnabled {
					// Check settings.pro.drift_detection.enabled
					if settings, ok := deployment.Settings["pro"].(map[string]any); ok {
						if driftDetection, ok := settings["drift_detection"].(map[string]any); ok {
							if enabled, ok := driftDetection["enabled"].(bool); ok && enabled {
								deployments = append(deployments, deployment)
							}
						}
					}
				} else {
					deployments = append(deployments, deployment)
				}
			}
		}
	}

	// Sort deployments by stack, then by component
	type deploymentRow struct {
		Component string
		Stack     string
	}
	rowsData := make([]deploymentRow, 0, len(deployments))
	for _, d := range deployments {
		rowsData = append(rowsData, deploymentRow{Component: d.Component, Stack: d.Stack})
	}
	// Sort
	slices.SortFunc(rowsData, func(a, b deploymentRow) int {
		if a.Stack < b.Stack {
			return -1
		} else if a.Stack > b.Stack {
			return 1
		}
		if a.Component < b.Component {
			return -1
		} else if a.Component > b.Component {
			return 1
		}
		return 0
	})

	// Convert to rows for formatter
	var rows []map[string]interface{}
	for _, row := range rowsData {
		rows = append(rows, map[string]interface{}{
			"Component": row.Component,
			"Stack":     row.Stack,
		})
	}

	// Create formatter options
	formatOpts := format.FormatOptions{
		Format:        format.FormatTable,
		TTY:           term.IsTerminal(int(os.Stdout.Fd())),
		CustomHeaders: []string{"Component", "Stack"},
		MaxColumns:    0,
	}

	// Format and print output
	output := format.CreateStyledTable(formatOpts.CustomHeaders, func() [][]string {
		var tableRows [][]string
		for _, row := range rows {
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%v", row["Component"]),
				fmt.Sprintf("%v", row["Stack"]),
			})
		}
		return tableRows
	}())
	fmt.Println(output)

	// Upload deployments if requested
	if upload {
		// Gather repo info
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

		// Get logger
		logger, err := logger.NewLoggerFromCliConfig(atmosConfig)
		if err != nil {
			log.Error("Failed to create logger", "error", err)
			return err
		}

		// Create API client
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

		if driftEnabled {
			err = apiClient.UploadDriftDetection(req)
			if err != nil {
				log.Error("Failed to upload deployments", "error", err)
				return err
			}
		} else {
			log.Info("Atmos Pro only supports uploading drift detection stacks at this time.\n\nTo upload drift detection stacks, use the --drift-enabled flag:\n  atmos list deployments --upload --drift-enabled")
			return nil
		}

		logger.Info("Successfully uploaded deployments to Atmos Pro API.")
	}

	return nil
}
