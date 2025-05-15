package list

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/describe"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
)

// ExecuteListDeploymentsCmd executes the list deployments command
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
	stacksMap, err := describe.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false)
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

	// Print deployments in a pretty table format using lipgloss
	componentColWidth := 24
	stackColWidth := 18

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.ColorGreen)).Padding(0, 1)
	cellStyle := lipgloss.NewStyle().Padding(0, 1)

	tableBorder := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(theme.ColorCyan)).Margin(0, 1)

	header := headerStyle.Width(componentColWidth).Render("Component") + headerStyle.Width(stackColWidth).Render("Stack")
	rows := []string{header}
	for _, rowData := range rowsData {
		row := cellStyle.Width(componentColWidth).Render(rowData.Component) + cellStyle.Width(stackColWidth).Render(rowData.Stack)
		rows = append(rows, row)
	}

	table := tableBorder.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	fmt.Println(table)

	// Upload deployments if requested
	if upload {
		// Gather repo info
		repo, err := git.GetLocalRepo()
		if err != nil {
			return fmt.Errorf("failed to get local git repo: %w", err)
		}
		repoInfo, err := git.GetRepoInfo(repo)
		if err != nil {
			return fmt.Errorf("failed to get git repo info: %w", err)
		}

		// Get logger
		logger, err := logger.NewLoggerFromCliConfig(atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create logger: %w", err)
		}

		// Create API client
		apiClient, err := pro.NewAtmosProAPIClientFromEnv(logger)
		if err != nil {
			return fmt.Errorf("failed to create Atmos Pro API client: %w", err)
		}

		// TODO: Get the correct base SHA (for now, leave blank)
		req := pro.DriftDetectionUploadRequest{
			BaseSHA:   "",
			RepoURL:   repoInfo.RepoUrl,
			RepoName:  repoInfo.RepoName,
			RepoOwner: repoInfo.RepoOwner,
			RepoHost:  repoInfo.RepoHost,
			Stacks:    deployments,
		}

		err = apiClient.UploadDriftDetection(req)
		if err != nil {
			return fmt.Errorf("failed to upload deployments: %w", err)
		}

		logger.Info("Successfully uploaded deployments to Atmos Pro API.")
	}

	return nil
}
