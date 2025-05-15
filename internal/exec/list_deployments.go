package exec

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
)

// ExecuteListDeploymentsCmd executes the list deployments command
func ExecuteListDeploymentsCmd(cmd *cobra.Command, args []string) error {
	// Process CLI arguments
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	// Initialize CLI config
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	// Get stack filter
	stackFilter := info.StackFromArg

	// Get drift detection filter
	driftEnabled := cmd.Flags().Changed("drift-enabled")

	// Get upload flag
	upload := cmd.Flags().Changed("upload")

	// Get all stacks
	stacksMap, err := ExecuteDescribeStacks(atmosConfig, stackFilter, nil, nil, nil, false, false, false, false, nil)
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

	// Print deployments in the format: component stack
	for _, deployment := range deployments {
		fmt.Printf("%s %s\n", deployment.Component, deployment.Stack)
	}

	// Upload deployments if requested
	if upload {
		// TODO: Implement upload to pro API
		fmt.Println("Upload functionality not implemented yet...")
	}

	return nil
}
