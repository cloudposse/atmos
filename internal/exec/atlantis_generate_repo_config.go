package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
	"path/filepath"
	"strings"
)

// ExecuteAtlantisGenerateRepoConfigCmd executes `atlantis generate repo-config` command
func ExecuteAtlantisGenerateRepoConfigCmd(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	configTemplateName, err := flags.GetString("config-template")
	if err != nil {
		return err
	}

	projectTemplateName, err := flags.GetString("project-template")
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

	return ExecuteAtlantisGenerateRepoConfig(configTemplateName, projectTemplateName, stacks, components)
}

// ExecuteAtlantisGenerateRepoConfig generates repository configuration for Atlantis
func ExecuteAtlantisGenerateRepoConfig(configTemplateName string, projectTemplateName string, stacks []string, components []string) error {
	var configAndStacksInfo c.ConfigAndStacksInfo
	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	var configTemplate c.AtlantisRepoConfig
	var projectTemplate c.AtlantisProjectConfig
	var ok bool

	if configTemplate, ok = c.Config.Integrations.Atlantis.ConfigTemplates[configTemplateName]; !ok {
		return errors.Errorf("atlantis config template '%s' is not defined in 'integrations.atlantis.config_templates' in atmos.yaml", configTemplateName)
	}

	if projectTemplate, ok = c.Config.Integrations.Atlantis.ProjectTemplates[projectTemplateName]; !ok {
		return errors.Errorf("atlantis project template '%s' is not defined in 'integrations.atlantis.project_templates' in atmos.yaml", projectTemplateName)
	}

	var atlantisProjects []c.AtlantisProjectConfig

	// Iterate over all components in all stacks and generate atlantis projects
	for stackConfigFileName, stackSection := range stacksMap {
		if componentsSection, ok := stackSection.(map[any]any)["components"].(map[string]any); ok {
			if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
				for componentName, compSection := range terraformSection {
					if componentSection, ok := compSection.(map[string]any); ok {
						// Find all derived components of the provided components
						derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackConfigFileName, terraformSection, components)
						if err != nil {
							return err
						}

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if varsSection, ok := componentSection["vars"].(map[any]any); ok {
								// Find terraform component.
								// If `component` attribute is present, it's the terraform component.
								// Otherwise, the YAML component name is the terraform component.
								terraformComponent := componentName
								if componentAttribute, ok := componentSection["component"].(string); ok {
									terraformComponent = componentAttribute
								}

								// Absolute path to the terraform component
								terraformComponentPath := path.Join(
									c.Config.BasePath,
									c.Config.Components.Terraform.BasePath,
									terraformComponent,
								)

								// Component metadata
								metadata := map[any]any{}
								if metadataSection, ok := componentSection["metadata"].(map[any]any); ok {
									metadata = metadataSection
								}

								context := c.GetContextFromVars(varsSection)
								context.Component = strings.Replace(componentName, "/", "-", -1)
								context.ComponentPath = terraformComponentPath
								contextPrefix, err := c.GetContextPrefix(stackConfigFileName, context, c.Config.Stacks.NamePattern, stackConfigFileName)
								if err != nil {
									return err
								}

								// Terraform workspace
								workspace, err := BuildTerraformWorkspace(
									stackConfigFileName,
									c.Config.Stacks.NamePattern,
									metadata,
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

									// Generate atlantis project for the component in the stack
									var whenModified []string

									for _, item := range projectTemplate.Autoplan.WhenModified {
										processedItem := c.ReplaceContextTokens(context, item)
										whenModified = append(whenModified, processedItem)
									}

									atlantisProjectAutoplanConfig := c.AtlantisProjectAutoplanConfig{
										Enabled:           projectTemplate.Autoplan.Enabled,
										ApplyRequirements: projectTemplate.Autoplan.ApplyRequirements,
										WhenModified:      whenModified,
									}

									atlantisProject := c.AtlantisProjectConfig{
										Name:                      c.ReplaceContextTokens(context, projectTemplate.Name),
										Workspace:                 c.ReplaceContextTokens(context, projectTemplate.Workspace),
										Workflow:                  c.ReplaceContextTokens(context, projectTemplate.Workflow),
										Dir:                       c.ReplaceContextTokens(context, projectTemplate.Dir),
										TerraformVersion:          c.ReplaceContextTokens(context, projectTemplate.TerraformVersion),
										DeleteSourceBranchOnMerge: projectTemplate.DeleteSourceBranchOnMerge,
										Autoplan:                  atlantisProjectAutoplanConfig,
									}

									atlantisProjects = append(atlantisProjects, atlantisProject)
								}
							}
						}
					}
				}
			}
		}
	}

	// atlantis config
	atlantisYaml := c.AtlantisConfigOutput{}
	atlantisYaml.Version = configTemplate.Version
	atlantisYaml.Automerge = configTemplate.Automerge
	atlantisYaml.DeleteSourceBranchOnMerge = configTemplate.DeleteSourceBranchOnMerge
	atlantisYaml.ParallelPlan = configTemplate.ParallelPlan
	atlantisYaml.ParallelApply = configTemplate.ParallelApply
	atlantisYaml.Workflows = configTemplate.Workflows
	atlantisYaml.AllowedRegexpPrefixes = configTemplate.AllowedRegexpPrefixes
	atlantisYaml.Projects = atlantisProjects

	// Write atlantis config to a file at the specified path
	fileName := c.Config.Integrations.Atlantis.Path
	u.PrintInfo(fmt.Sprintf("Writing atlantis config to file '%s'", fileName))

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

	return nil
}
