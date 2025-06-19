package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func checkTerraformConfig(atmosConfig schema.AtmosConfiguration) error {
	if len(atmosConfig.Components.Terraform.BasePath) < 1 {
		return errors.New("Base path to terraform components must be provided in 'components.terraform.base_path' config or " +
			"'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV variable")
	}

	return nil
}

// cleanTerraformWorkspace deletes the `.terraform/environment` file from the component directory.
// The `.terraform/environment` file contains the name of the currently selected workspace,
// helping Terraform identify the active workspace context for managing your infrastructure.
// We delete the file to prevent the Terraform prompt asking to select the default or the
// previously used workspace. This happens when different backends are used for the same component.
func cleanTerraformWorkspace(atmosConfig schema.AtmosConfiguration, componentPath string) {
	// Get `TF_DATA_DIR` ENV variable, default to `.terraform` if not set
	tfDataDir := os.Getenv("TF_DATA_DIR")
	if tfDataDir == "" {
		tfDataDir = ".terraform"
	}

	// Convert relative path to absolute
	if !filepath.IsAbs(tfDataDir) {
		tfDataDir = filepath.Join(componentPath, tfDataDir)
	}

	// Ensure the path is cleaned properly
	tfDataDir = filepath.Clean(tfDataDir)

	// Construct the full file path
	filePath := filepath.Join(tfDataDir, "environment")

	// Check if the file exists before attempting deletion
	if _, err := os.Stat(filePath); err == nil {
		log.Debug("Terraform environment file found. Proceeding with deletion.", "file", filePath)

		if err := os.Remove(filePath); err != nil {
			log.Debug("Failed to delete Terraform environment file.", "file", filePath, "error", err)
		} else {
			log.Debug("Successfully deleted Terraform environment file.", "file", filePath)
		}
	} else if os.IsNotExist(err) {
		log.Debug("Terraform environment file not found. No action needed.", "file", filePath)
	} else {
		log.Debug("Error checking Terraform environment file.", "file", filePath, "error", err)
	}
}

func shouldProcessStacks(info *schema.ConfigAndStacksInfo) (bool, bool) {
	shouldProcessStacks := true
	shouldCheckStack := true

	if info.SubCommand == "clean" {
		if info.ComponentFromArg == "" {
			shouldProcessStacks = false
		}
		shouldCheckStack = info.Stack != ""

	}

	return shouldProcessStacks, shouldCheckStack
}

func generateBackendConfig(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	// Auto-generate backend file
	if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
		backendFileName := filepath.Join(workingDir, "backend.tf.json")

		log.Debug("Writing the backend config to file.", "file", backendFileName)

		if !info.DryRun {
			componentBackendConfig, err := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace)
			if err != nil {
				return err
			}

			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0o600)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func generateProviderOverrides(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	// Generate `providers_override.tf.json` file if the `providers` section is configured
	if len(info.ComponentProvidersSection) > 0 {
		providerOverrideFileName := filepath.Join(workingDir, "providers_override.tf.json")

		log.Debug("Writing the provider overrides to file.", "file", providerOverrideFileName)

		if !info.DryRun {
			providerOverrides := generateComponentProviderOverrides(info.ComponentProvidersSection)
			err := u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0o600)
			return err
		}
	}
	return nil
}

// needProcessTemplatesAndYamlFunctions checks if a Terraform command.
// requires the `Go` templates and Atmos YAML functions to be processed.
func needProcessTemplatesAndYamlFunctions(command string) bool {
	commandsThatNeedFuncProcessing := []string{
		"init",
		"plan",
		"apply",
		"deploy",
		"destroy",
		"generate",
		"output",
		"clean",
		"shell",
		"write",
		"force-unlock",
		"import",
		"refresh",
		"show",
		"taint",
		"untaint",
		"validate",
		"state list",
		"state mv",
		"state pull",
		"state push",
		"state replace-provider",
		"state rm",
		"state show",
	}
	return u.SliceContainsString(commandsThatNeedFuncProcessing, command)
}

// isWorkspacesEnabled checks if Terraform workspaces are enabled for a component.
// Workspaces are enabled by default except for:
// 1. When explicitly disabled via workspaces_enabled: false in `atmos.yaml`.
// 2. When using HTTP backend (which doesn't support workspaces).
func isWorkspacesEnabled(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	// Check if using HTTP backend first, as it doesn't support workspaces
	if info.ComponentBackendType == "http" {
		// If workspaces are explicitly enabled for HTTP backend, log a warning.
		if atmosConfig.Components.Terraform.WorkspacesEnabled != nil && *atmosConfig.Components.Terraform.WorkspacesEnabled {
			log.Warn("ignoring unsupported workspaces `enabled` setting for HTTP backend type.",
				"backend", "http",
				"component", info.Component)
		}
		return false
	}

	// Check if workspaces are explicitly disabled.
	if atmosConfig.Components.Terraform.WorkspacesEnabled != nil && !*atmosConfig.Components.Terraform.WorkspacesEnabled {
		return false
	}

	return true
}

// ExecuteTerraformAffected executes `atmos terraform <command> --affected`.
func ExecuteTerraformAffected(cmd *cobra.Command, args []string, info *schema.ConfigAndStacksInfo) error {
	// Add these flags because `atmos describe affected` needs them, but `atmos terraform --affected` does not define them
	cmd.PersistentFlags().String("file", "", "")
	cmd.PersistentFlags().String("format", "yaml", "")
	cmd.PersistentFlags().Bool("verbose", false, "")
	cmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "")
	cmd.PersistentFlags().Bool("include-settings", false, "")
	cmd.PersistentFlags().Bool("upload", false, "")

	a, err := ParseDescribeAffectedCliArgs(cmd, args)
	if err != nil {
		return err
	}

	a.IncludeSpaceliftAdminStacks = false
	a.IncludeSettings = false
	a.Upload = false
	a.OutputFile = ""
	a.Verbose = false

	var affectedList []schema.Affected

	switch {
	case a.RepoPath != "":
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRepoPath(
			a.CLIConfig,
			a.RepoPath,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	case a.CloneTargetRef:
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRefClone(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.SSHKeyPath,
			a.SSHKeyPassword,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	default:
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRefCheckout(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	}
	if err != nil {
		return err
	}

	// Add dependent components and stacks for each affected component
	if len(affectedList) > 0 && a.IncludeDependents {
		err = addDependentsToAffected(a.CLIConfig, &affectedList, a.IncludeSettings)
		if err != nil {
			return err
		}
	}

	affectedYaml, err := u.ConvertToYAML(affectedList)
	if err != nil {
		return err
	}
	log.Debug("Affected", "components", affectedYaml)

	for _, affected := range affectedList {
		// If the affected component is included in the dependencies of any other component, don't process it now,
		// it will be processed in the dependency order.
		if !affected.IncludedInDependents {
			err = executeTerraformAffectedComponentInDepOrder(info,
				affected.Component,
				affected.Stack,
				"",
				"",
				affected.Dependents,
				&a,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// executeTerraformAffectedComponentInDepOrder recursively processes the affected components in the dependency order.
func executeTerraformAffectedComponentInDepOrder(
	info *schema.ConfigAndStacksInfo,
	affectedComponent string,
	affectedStack string,
	parentComponent string,
	parentStack string,
	dependents []schema.Dependent,
	args *DescribeAffectedCmdArgs,
) error {
	var logFunc func(msg interface{}, keyvals ...interface{})
	if info.DryRun {
		logFunc = log.Info
	} else {
		logFunc = log.Debug
	}

	info.Component = affectedComponent
	info.ComponentFromArg = affectedComponent
	info.Stack = affectedStack

	command := fmt.Sprintf("atmos terraform %s %s -s %s", info.SubCommand, affectedComponent, affectedStack)

	if parentComponent != "" && parentStack != "" {
		logFunc("Executing", "command", command, "dependency of component", parentComponent, "in stack", parentStack)
	} else {
		logFunc("Executing", "command", command)
	}

	if !info.DryRun {
		// Execute the terraform command for the affected component
		// err := ExecuteTerraform(info)
		// if err != nil {
		//	return err
		// }
	}

	if args.IncludeDependents {
		for _, dep := range dependents {
			err := executeTerraformAffectedComponentInDepOrder(
				info,
				dep.Component,
				dep.Stack,
				affectedComponent,
				affectedStack,
				dep.Dependents,
				args,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ExecuteTerraformQuery executes `atmos terraform <command> --query <yq-expression --stack <stack>`.
func ExecuteTerraformQuery(info *schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	var stack string
	var components []string

	if !info.All {
		stack = info.Stack
		components = info.Components
	}

	var logFunc func(msg interface{}, keyvals ...interface{})
	if info.DryRun {
		logFunc = log.Info
	} else {
		logFunc = log.Debug
	}

	stacks, err := ExecuteDescribeStacks(
		atmosConfig,
		stack,
		components,
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
	)
	if err != nil {
		return err
	}

	for stackName, stackSection := range stacks {
		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap[cfg.ComponentsSectionName].(map[string]any); ok {
				if terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							if metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
								// Skip abstract components
								if metadataType, ok := metadataSection["type"].(string); ok {
									if metadataType == "abstract" {
										continue
									}
								}
								// Skip disabled components
								if !isComponentEnabled(metadataSection, componentName) {
									continue
								}

								command := fmt.Sprintf("atmos terraform %s %s -s %s", info.SubCommand, componentName, stackName)

								if info.Query != "" {
									queryResult, err := u.EvaluateYqExpression(&atmosConfig, componentSection, info.Query)
									if err != nil {
										return err
									}
									if queryPassed, ok := queryResult.(bool); !ok || !queryPassed {
										logFunc("Skipping the component because the query criteria not satisfied", "command", command, "query", info.Query)
										continue
									}
								}

								logFunc("Executing", "command", command)

								if !info.DryRun {
									// Execute the terraform command for the affected component
									// err := ExecuteTerraform(info)
									// if err != nil {
									//	return err
									// }
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}
