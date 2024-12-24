package exec

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteAtlantisGenerateRepoConfigCmd executes 'atlantis generate repo-config' command
func ExecuteAtlantisGenerateRepoConfigCmd(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
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

	affectedOnly, err := flags.GetBool("affected-only")
	if err != nil {
		return err
	}

	ref, err := flags.GetString("ref")
	if err != nil {
		return err
	}

	sha, err := flags.GetString("sha")
	if err != nil {
		return err
	}

	repoPath, err := flags.GetString("repo-path")
	if err != nil {
		return err
	}

	sshKeyPath, err := flags.GetString("ssh-key")
	if err != nil {
		return err
	}

	sshKeyPassword, err := flags.GetString("ssh-key-password")
	if err != nil {
		return err
	}

	verbose, err := flags.GetBool("verbose")
	if err != nil {
		return err
	}

	// If the flag `--affected-only=true` is passed, find the affected components and stacks
	if affectedOnly {
		cloneTargetRef, err := flags.GetBool("clone-target-ref")
		if err != nil {
			return err
		}

		return ExecuteAtlantisGenerateRepoConfigAffectedOnly(
			atmosConfig,
			outputPath,
			configTemplateName,
			projectTemplateName,
			ref,
			sha,
			repoPath,
			sshKeyPath,
			sshKeyPassword,
			verbose,
			cloneTargetRef,
			"",
		)
	}

	return ExecuteAtlantisGenerateRepoConfig(
		atmosConfig,
		outputPath,
		configTemplateName,
		projectTemplateName,
		stacks,
		components,
	)
}

// ExecuteAtlantisGenerateRepoConfigAffectedOnly generates repository configuration for Atlantis only for the affected components and stacks
func ExecuteAtlantisGenerateRepoConfigAffectedOnly(
	atmosConfig schema.AtmosConfiguration,
	outputPath string,
	configTemplateName string,
	projectTemplateName string,
	ref string,
	sha string,
	repoPath string,
	sshKeyPath string,
	sshKeyPassword string,
	verbose bool,
	cloneTargetRef bool,
	stack string,
) error {
	if repoPath != "" && (ref != "" || sha != "" || sshKeyPath != "" || sshKeyPassword != "") {
		return errors.New("if the '--repo-path' flag is specified, the '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")
	}

	var affected []schema.Affected
	var err error

	if repoPath != "" {
		affected, _, _, _, err = ExecuteDescribeAffectedWithTargetRepoPath(
			atmosConfig,
			repoPath,
			verbose,
			false,
			false,
			stack,
		)
	} else if cloneTargetRef {
		affected, _, _, _, err = ExecuteDescribeAffectedWithTargetRefClone(
			atmosConfig,
			ref,
			sha,
			sshKeyPath,
			sshKeyPassword,
			verbose,
			false,
			false,
			stack,
		)
	} else {
		affected, _, _, _, err = ExecuteDescribeAffectedWithTargetRefCheckout(
			atmosConfig,
			ref,
			sha,
			verbose,
			false,
			false,
			stack,
		)
	}

	if err != nil {
		return err
	}

	if len(affected) == 0 {
		return nil
	}

	affectedComponents := lo.FilterMap[schema.Affected, string](affected, func(x schema.Affected, _ int) (string, bool) {
		if x.ComponentType == "terraform" {
			return x.Component, true
		}
		return "", false
	})

	affectedStacks := lo.FilterMap[schema.Affected, string](affected, func(x schema.Affected, _ int) (string, bool) {
		if x.ComponentType == "terraform" {
			return x.Stack, true
		}
		return "", false
	})

	return ExecuteAtlantisGenerateRepoConfig(
		atmosConfig,
		outputPath,
		configTemplateName,
		projectTemplateName,
		affectedStacks,
		affectedComponents,
	)
}

// ExecuteAtlantisGenerateRepoConfig generates repository configuration for Atlantis
func ExecuteAtlantisGenerateRepoConfig(
	atmosConfig schema.AtmosConfiguration,
	outputPath string,
	configTemplateNameArg string,
	projectTemplateNameArg string,
	stacks []string,
	components []string,
) error {

	stacksMap, _, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return err
	}

	var configTemplate schema.AtlantisRepoConfig
	var projectTemplate schema.AtlantisProjectConfig
	var ok bool
	var atlantisProjects []schema.AtlantisProjectConfig
	var componentsSection map[string]any
	var terraformSection map[string]any
	var componentSection map[string]any
	var varsSection map[string]any
	var settingsSection map[string]any

	if projectTemplateNameArg != "" {
		if projectTemplate, ok = atmosConfig.Integrations.Atlantis.ProjectTemplates[projectTemplateNameArg]; !ok {
			return errors.Errorf("atlantis project template '%s' is not defined in 'integrations.atlantis.project_templates' in 'atmos.yaml'", projectTemplateNameArg)
		}
	}

	// Iterate over all components in all stacks and generate atlantis projects
	// Iterate not over the map itself, but over the sorted map keys since Go iterates over maps in random order
	stacksMapSortedKeys := u.StringKeysFromMap(stacksMap)

	for _, stackConfigFileName := range stacksMapSortedKeys {
		stackSection := stacksMap[stackConfigFileName]

		if componentsSection, ok = stackSection.(map[string]any)["components"].(map[string]any); !ok {
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

			// Check if 'components' filter is provided
			if len(components) == 0 ||
				u.SliceContainsString(components, componentName) {

				// Component vars
				if varsSection, ok = componentSection["vars"].(map[string]any); !ok {
					continue
				}

				// Component metadata
				metadataSection := map[string]any{}
				if metadataSection, ok = componentSection["metadata"].(map[string]any); ok {
					if componentType, ok := metadataSection["type"].(string); ok {
						// Don't include abstract components
						if componentType == "abstract" {
							continue
						}
					}
				}

				// If the project template is not passes on the command line, find and process it in the component project template in the 'settings.atlantis' section
				if projectTemplateNameArg == "" {
					if settingsSection, ok = componentSection["settings"].(map[string]any); ok {
						if settingsAtlantisSection, ok := settingsSection["atlantis"].(map[string]any); ok {
							// 'settings.atlantis.project_template' has higher priority than 'settings.atlantis.project_template_name'
							if settingsAtlantisProjectTemplate, ok := settingsAtlantisSection["project_template"].(map[string]any); ok {
								err = mapstructure.Decode(settingsAtlantisProjectTemplate, &projectTemplate)
								if err != nil {
									return err
								}
							} else if settingsAtlantisProjectTemplateName, ok := settingsAtlantisSection["project_template_name"].(string); ok && settingsAtlantisProjectTemplateName != "" {
								if projectTemplate, ok = atmosConfig.Integrations.Atlantis.ProjectTemplates[settingsAtlantisProjectTemplateName]; !ok {
									return errors.Errorf(
										"the component '%s' in the stack config file '%s' "+
											"specifies the atlantis project template name '%s' "+
											"in the 'settings.atlantis.project_template_name' section, "+
											"but this atlantis project template is not defined in 'integrations.atlantis.project_templates' in 'atmos.yaml'",
										componentName, stackConfigFileName, settingsAtlantisProjectTemplateName)
								}
							}
						}
					}
				}

				// https://www.golinuxcloud.com/golang-check-if-struct-is-empty/
				if reflect.ValueOf(projectTemplate).IsZero() {
					return errors.Errorf(
						"atlantis project template is not specified for the component '%s'. "+
							"In needs to be defined in one of these places: 'settings.atlantis.project_template_name' stack config section, "+
							"'settings.atlantis.project_template' stack config section, "+
							"or passed on the command line using the '--project-template' flag to select a project template from the "+
							"collection of templates defined in the 'integrations.atlantis.project_templates' section in 'atmos.yaml'",
						componentName)
				}

				// Find the terraform component
				// If 'component' attribute is present, it's the terraform component
				// Otherwise, the Atmos component name is the terraform component (by default)
				terraformComponent := componentName
				if componentAttribute, ok := componentSection[cfg.ComponentSectionName].(string); ok {
					terraformComponent = componentAttribute
				}

				// Absolute path to the terraform component
				terraformComponentPath := path.Join(
					atmosConfig.BasePath,
					atmosConfig.Components.Terraform.BasePath,
					terraformComponent,
				)

				// Context
				context := cfg.GetContextFromVars(varsSection)
				context.Component = strings.Replace(componentName, "/", "-", -1)
				context.ComponentPath = terraformComponentPath
				contextPrefix, err := cfg.GetContextPrefix(stackConfigFileName, context, GetStackNamePattern(atmosConfig), stackConfigFileName)
				if err != nil {
					return err
				}

				// Base component is required to calculate terraform workspace for derived components
				if terraformComponent != componentName {
					context.BaseComponent = terraformComponent
				}

				configAndStacksInfo := schema.ConfigAndStacksInfo{
					ComponentFromArg:         componentName,
					Stack:                    stackConfigFileName,
					ComponentMetadataSection: metadataSection,
					ComponentSettingsSection: settingsSection,
					ComponentVarsSection:     varsSection,
					Context:                  context,
					ComponentSection: map[string]any{
						cfg.VarsSectionName:     varsSection,
						cfg.SettingsSectionName: settingsSection,
						cfg.MetadataSectionName: metadataSection,
					},
				}

				// Calculate terraform workspace
				workspace, err := BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
				if err != nil {
					return err
				}

				context.Workspace = workspace

				// Check if 'stacks' filter is provided
				if len(stacks) == 0 ||
					// 'stacks' filter can contain the names of the top-level stack config files:
					// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
					u.SliceContainsString(stacks, stackConfigFileName) ||
					// 'stacks' filter can also contain the logical stack names (derived from the context vars):
					// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
					u.SliceContainsString(stacks, contextPrefix) {

					// Generate an atlantis project for the component in the stack
					// Replace the context tokens
					var whenModified []string

					for _, item := range projectTemplate.Autoplan.WhenModified {
						processedItem := cfg.ReplaceContextTokens(context, item)
						whenModified = append(whenModified, processedItem)
					}

					atlantisProjectAutoplanConfig := schema.AtlantisProjectAutoplanConfig{
						Enabled:      projectTemplate.Autoplan.Enabled,
						WhenModified: whenModified,
					}

					atlantisProjectName := cfg.ReplaceContextTokens(context, projectTemplate.Name)

					atlantisProject := schema.AtlantisProjectConfig{
						Name:                      atlantisProjectName,
						Workspace:                 cfg.ReplaceContextTokens(context, projectTemplate.Workspace),
						Dir:                       cfg.ReplaceContextTokens(context, projectTemplate.Dir),
						TerraformVersion:          projectTemplate.TerraformVersion,
						DeleteSourceBranchOnMerge: projectTemplate.DeleteSourceBranchOnMerge,
						Autoplan:                  atlantisProjectAutoplanConfig,
						ApplyRequirements:         projectTemplate.ApplyRequirements,
						Workflow:                  projectTemplate.Workflow,
					}

					atlantisProjects = append(atlantisProjects, atlantisProject)
				}
			}
		}
	}

	// If the config template is not passes on the command line, find and process it in the component project template in the 'settings.atlantis' section
	if configTemplateNameArg == "" {
		if settingsSection, ok = componentSection["settings"].(map[string]any); ok {
			if settingsAtlantisSection, ok := settingsSection["atlantis"].(map[string]any); ok {
				// 'settings.atlantis.config_template' has higher priority than 'settings.atlantis.config_template_name'
				if settingsAtlantisConfigTemplate, ok := settingsAtlantisSection["config_template"].(map[string]any); ok {
					err = mapstructure.Decode(settingsAtlantisConfigTemplate, &configTemplate)
					if err != nil {
						return err
					}
				} else if settingsAtlantisConfigTemplateName, ok := settingsAtlantisSection["config_template_name"].(string); ok && settingsAtlantisConfigTemplateName != "" {
					if configTemplate, ok = atmosConfig.Integrations.Atlantis.ConfigTemplates[settingsAtlantisConfigTemplateName]; !ok {
						return errors.Errorf(
							"atlantis config template name '%s' is specified "+
								"in the 'settings.atlantis.config_template_name' section, "+
								"but this atlantis config template is not defined in 'integrations.atlantis.config_templates' in 'atmos.yaml'",
							settingsAtlantisConfigTemplateName)
					}
				}
			}
		}
	} else {
		if configTemplate, ok = atmosConfig.Integrations.Atlantis.ConfigTemplates[configTemplateNameArg]; !ok {
			return errors.Errorf("atlantis config template '%s' is not defined in 'integrations.atlantis.config_templates' in 'atmos.yaml'", configTemplateNameArg)
		}
	}

	if reflect.ValueOf(configTemplate).IsZero() {
		return errors.Errorf(
			"atlantis config template is not specified. " +
				"In needs to be defined in one of these places: 'settings.atlantis.config_template_name' stack config section, " +
				"'settings.atlantis.config_template' stack config section, " +
				"or passed on the command line using the '--config-template' flag to select a config template from the " +
				"collection of templates defined in the 'integrations.atlantis.config_templates' section in 'atmos.yaml'")
	}

	// Final atlantis config
	atlantisYaml := schema.AtlantisConfigOutput{}
	atlantisYaml.Version = configTemplate.Version
	atlantisYaml.Automerge = configTemplate.Automerge
	atlantisYaml.DeleteSourceBranchOnMerge = configTemplate.DeleteSourceBranchOnMerge
	atlantisYaml.ParallelPlan = configTemplate.ParallelPlan
	atlantisYaml.ParallelApply = configTemplate.ParallelApply
	atlantisYaml.AllowedRegexpPrefixes = configTemplate.AllowedRegexpPrefixes
	atlantisYaml.Projects = atlantisProjects

	// Workflows
	if settingsSection, ok = componentSection["settings"].(map[string]any); ok {
		if settingsAtlantisSection, ok := settingsSection["atlantis"].(map[string]any); ok {
			if settingsAtlantisWorkflowTemplates, ok := settingsAtlantisSection["workflow_templates"].(map[string]any); ok {
				atlantisYaml.Workflows = settingsAtlantisWorkflowTemplates
			}
		}
	}

	if reflect.ValueOf(atlantisYaml.Workflows).IsZero() && !reflect.ValueOf(atmosConfig.Integrations.Atlantis.WorkflowTemplates).IsZero() {
		atlantisYaml.Workflows = atmosConfig.Integrations.Atlantis.WorkflowTemplates
	}

	// Write the atlantis config to a file at the specified path
	// Check the command line argument '--output-path' first
	// Then check the 'atlantis.path' setting in 'atmos.yaml'
	fileName := outputPath
	if fileName == "" {
		fileName = atmosConfig.Integrations.Atlantis.Path
		u.LogDebug(atmosConfig, fmt.Sprintf("Using 'atlantis.path: %s' from 'atmos.yaml'", fileName))
	} else {
		u.LogDebug(atmosConfig, fmt.Sprintf("Using '--output-path %s' command-line argument", fileName))
	}

	// If the path is empty, dump to 'stdout'
	if fileName != "" {
		u.LogDebug(atmosConfig, fmt.Sprintf("Writing atlantis repo config file to '%s'\n", fileName))

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
