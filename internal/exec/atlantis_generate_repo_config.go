package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
	"path/filepath"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteAtlantisGenerateRepoConfigCmd executes `atlantis generate repo-config` command
func ExecuteAtlantisGenerateRepoConfigCmd(cmd *cobra.Command, args []string) error {
	cliConfig, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	outputPath, err := flags.GetString("output-path")
	if err != nil {
		return err
	}

	configTemplateName, err := flags.GetString("config-template")
	if err != nil {
		return err
	}

	projectTemplateName, err := flags.GetString("project-template")
	if err != nil {
		return err
	}

	workflowTemplateName, err := flags.GetString("workflow-template")
	if err != nil {
		return err
	}

	stacksCsv, err := flags.GetString("stacks")
	if err != nil {
		return err
	}
	var stacks []string
	if stacksCsv != "" {
		stacks = strings.Split(stacksCsv, ",")
	}

	componentsCsv, err := flags.GetString("components")
	if err != nil {
		return err
	}
	var components []string
	if componentsCsv != "" {
		components = strings.Split(componentsCsv, ",")
	}

	return ExecuteAtlantisGenerateRepoConfig(cliConfig, outputPath, configTemplateName, projectTemplateName, workflowTemplateName, stacks, components)
}

// ExecuteAtlantisGenerateRepoConfig generates repository configuration for Atlantis
func ExecuteAtlantisGenerateRepoConfig(
	cliConfig cfg.CliConfiguration,
	outputPath string,
	configTemplateName string,
	projectTemplateName string,
	workflowTemplateName string,
	stacks []string,
	components []string) error {

	stacksMap, err := FindStacksMap(cliConfig)
	if err != nil {
		return err
	}

	var configTemplate cfg.AtlantisRepoConfig
	var projectTemplate cfg.AtlantisProjectConfig
	var workflowTemplate any
	var ok bool

	if configTemplate, ok = cliConfig.Integrations.Atlantis.ConfigTemplates[configTemplateName]; !ok {
		return errors.Errorf("atlantis config template '%s' is not defined in 'integrations.atlantis.config_templates' in atmos.yaml", configTemplateName)
	}

	if projectTemplate, ok = cliConfig.Integrations.Atlantis.ProjectTemplates[projectTemplateName]; !ok {
		return errors.Errorf("atlantis project template '%s' is not defined in 'integrations.atlantis.project_templates' in atmos.yaml", projectTemplateName)
	}

	if workflowTemplate, ok = cliConfig.Integrations.Atlantis.WorkflowTemplates[workflowTemplateName]; !ok {
		return errors.Errorf("atlantis workflow template '%s' is not defined in 'integrations.atlantis.workflow_templates' in atmos.yaml", workflowTemplateName)
	}

	var atlantisProjects []cfg.AtlantisProjectConfig
	var componentsSection map[string]any
	var terraformSection map[string]any
	var componentSection map[string]any
	var varsSection map[any]any

	// Iterate over all components in all stacks and generate atlantis projects
	// Iterate not over the map itself, but over the sorted map keys since Go iterates over maps in random order
	stacksMapSortedKeys := u.StringKeysFromMap(stacksMap)

	for _, stackConfigFileName := range stacksMapSortedKeys {
		stackSection := stacksMap[stackConfigFileName]

		if componentsSection, ok = stackSection.(map[any]any)["components"].(map[string]any); !ok {
			continue
		}

		if terraformSection, ok = componentsSection["terraform"].(map[string]any); !ok {
			continue
		}

		// Iterate not over the map itself, but over the sorted map keys since Go iterates over maps in random order
		componentMapSortedKeys := u.StringKeysFromMap(terraformSection)

		for _, componentName := range componentMapSortedKeys {
			compSection := terraformSection[componentName]

			if componentSection, ok = compSection.(map[string]any); !ok {
				continue
			}

			// Check if `components` filter is provided
			if len(components) == 0 ||
				u.SliceContainsString(components, componentName) {

				// Component vars
				if varsSection, ok = componentSection["vars"].(map[any]any); !ok {
					continue
				}

				// Component metadata
				metadataSection := map[any]any{}
				if metadataSection, ok = componentSection["metadata"].(map[any]any); ok {
					if componentType, ok := metadataSection["type"].(string); ok {
						// Don't include abstract components
						if componentType == "abstract" {
							continue
						}
					}
				}

				// Find the terraform component
				// If `component` attribute is present, it's the terraform component
				// Otherwise, the YAML component name is the terraform component (by default)
				terraformComponent := componentName
				if componentAttribute, ok := componentSection["component"].(string); ok {
					terraformComponent = componentAttribute
				}

				// Absolute path to the terraform component
				terraformComponentPath := path.Join(
					cliConfig.BasePath,
					cliConfig.Components.Terraform.BasePath,
					terraformComponent,
				)

				// Context
				context := cfg.GetContextFromVars(varsSection)
				context.Component = strings.Replace(componentName, "/", "-", -1)
				context.ComponentPath = terraformComponentPath
				contextPrefix, err := cfg.GetContextPrefix(stackConfigFileName, context, cliConfig.Stacks.NamePattern, stackConfigFileName)
				if err != nil {
					return err
				}

				// Terraform workspace
				workspace, err := BuildTerraformWorkspace(
					stackConfigFileName,
					cliConfig.Stacks.NamePattern,
					metadataSection,
					context,
				)
				if err != nil {
					return err
				}

				context.Workspace = workspace

				// Check if `stacks` filter is provided
				if len(stacks) == 0 ||
					// `stacks` filter can contain the names of the top-level stack config files:
					// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
					u.SliceContainsString(stacks, stackConfigFileName) ||
					// `stacks` filter can also contain the logical stack names (derived from the context vars):
					// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
					u.SliceContainsString(stacks, contextPrefix) {

					// Generate an atlantis project for the component in the stack
					// Replace the context tokens
					var whenModified []string

					for _, item := range projectTemplate.Autoplan.WhenModified {
						processedItem := cfg.ReplaceContextTokens(context, item)
						whenModified = append(whenModified, processedItem)
					}

					atlantisProjectAutoplanConfig := cfg.AtlantisProjectAutoplanConfig{
						Enabled:      projectTemplate.Autoplan.Enabled,
						WhenModified: whenModified,
					}

					atlantisProjectName := cfg.ReplaceContextTokens(context, projectTemplate.Name)

					atlantisProject := cfg.AtlantisProjectConfig{
						Name:                      atlantisProjectName,
						Workspace:                 cfg.ReplaceContextTokens(context, projectTemplate.Workspace),
						Workflow:                  workflowTemplateName,
						Dir:                       cfg.ReplaceContextTokens(context, projectTemplate.Dir),
						TerraformVersion:          projectTemplate.TerraformVersion,
						DeleteSourceBranchOnMerge: projectTemplate.DeleteSourceBranchOnMerge,
						Autoplan:                  atlantisProjectAutoplanConfig,
						ApplyRequirements:         projectTemplate.ApplyRequirements,
					}

					atlantisProjects = append(atlantisProjects, atlantisProject)
				}
			}
		}
	}

	// Workflows
	atlantisWorkflows := map[string]any{
		workflowTemplateName: workflowTemplate,
	}

	// Final atlantis config
	atlantisYaml := cfg.AtlantisConfigOutput{}
	atlantisYaml.Version = configTemplate.Version
	atlantisYaml.Automerge = configTemplate.Automerge
	atlantisYaml.DeleteSourceBranchOnMerge = configTemplate.DeleteSourceBranchOnMerge
	atlantisYaml.ParallelPlan = configTemplate.ParallelPlan
	atlantisYaml.ParallelApply = configTemplate.ParallelApply
	atlantisYaml.AllowedRegexpPrefixes = configTemplate.AllowedRegexpPrefixes
	atlantisYaml.Projects = atlantisProjects
	atlantisYaml.Workflows = atlantisWorkflows

	// Write the atlantis config to a file at the specified path
	// Check the command line argument `--output-path` first
	// Then check the `atlantis.path` setting in `atmos.yaml`
	fileName := outputPath
	if fileName == "" {
		fileName = cliConfig.Integrations.Atlantis.Path
		u.PrintInfo(fmt.Sprintf("Using 'atlantis.path: %s' from atmos.yaml", fileName))
	} else {
		u.PrintInfo(fmt.Sprintf("Using '--output-path %s' command-line argument", fileName))
	}

	// If the path is empty, dump to `stdout`
	if fileName != "" {
		u.PrintInfo(fmt.Sprintf("Writing atlantis repo config file to '%s'", fileName))

		fileAbsolutePath, err := filepath.Abs(fileName)
		if err != nil {
			return err
		}

		// Create all the intermediate subdirectories
		err = u.EnsureDir(fileAbsolutePath)
		if err != nil {
			return err
		}

		err = u.WriteToFileAsYAML(fileAbsolutePath, atlantisYaml, 0644)
		if err != nil {
			return err
		}
	} else {
		err = u.PrintAsYAML(atlantisYaml)
		if err != nil {
			return err
		}
	}

	return nil
}
