package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const commandStr = "command"

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

// needProcessTemplatesAndYamlFunctions checks if a Terraform command requires the `Go` templates and Atmos YAML functions to be processed.
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
func ExecuteTerraformAffected(args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	var affectedList []schema.Affected
	var err error

	switch {
	case args.RepoPath != "":
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRepoPath(
			args.CLIConfig,
			args.RepoPath,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
		)
	case args.CloneTargetRef:
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRefClone(
			args.CLIConfig,
			args.Ref,
			args.SHA,
			args.SSHKeyPath,
			args.SSHKeyPassword,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
		)
	default:
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRefCheckout(
			args.CLIConfig,
			args.Ref,
			args.SHA,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
		)
	}
	if err != nil {
		return err
	}

	// Add dependent components for each directly affected component.
	if len(affectedList) > 0 {
		err = addDependentsToAffected(
			args.CLIConfig,
			&affectedList,
			args.IncludeSettings,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
		)
		if err != nil {
			return err
		}
	}

	affectedYaml, err := u.ConvertToYAML(affectedList)
	if err != nil {
		return err
	}
	log.Debug("Affected", "components", affectedYaml)

	for i := 0; i < len(affectedList); i++ {
		affected := &affectedList[i]
		// If the affected component is included in the dependencies of any other component, don't process it now;
		// it will be processed in the dependency order.
		if !affected.IncludedInDependents {
			err = executeTerraformAffectedComponentInDepOrder(
				info,
				affectedList,
				affected.Component,
				affected.Stack,
				"",
				"",
				affected.Dependents,
				args,
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
	affectedList []schema.Affected,
	affectedComponent string,
	affectedStack string,
	parentComponent string,
	parentStack string,
	dependents []schema.Dependent,
	args *DescribeAffectedCmdArgs,
) error {
	var logFunc func(msg any, keyvals ...any)
	if info.DryRun {
		logFunc = log.Info
	} else {
		logFunc = log.Debug
	}

	info.Component = affectedComponent
	info.ComponentFromArg = affectedComponent
	info.Stack = affectedStack

	command := fmt.Sprintf("atmos terraform %s %s -s %s", info.SubCommand, affectedComponent, affectedStack)

	if args.IncludeDependents && parentComponent != "" && parentStack != "" {
		logFunc("Executing", commandStr, command, "dependency of component", parentComponent, "in stack", parentStack)
	} else {
		logFunc("Executing", commandStr, command)
	}

	if !info.DryRun {
		// Execute the terraform command for the affected component
		err := ExecuteTerraform(*info)
		if err != nil {
			return err
		}
	}

	for i := 0; i < len(dependents); i++ {
		dep := &dependents[i]
		if args.IncludeDependents || isComponentInStackAffected(affectedList, dep.StackSlug) {
			if !dep.IncludedInDependents {
				err := executeTerraformAffectedComponentInDepOrder(
					info,
					affectedList,
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
	}

	return nil
}

// walkTerraformComponents iterates over all Terraform components in the provided stacks map.
// For each component it calls the provided function, stopping if the function returns an error.
func walkTerraformComponents(
	stacks map[string]any,
	fn func(stackName, componentName string, componentSection map[string]any) error,
) error {
	for stackName, stackSection := range stacks {
		stackSectionMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}

		componentsSection, ok := stackSectionMap[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}

		terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any)
		if !ok {
			continue
		}

		for componentName, compSection := range terraformSection {
			componentSection, ok := compSection.(map[string]any)
			if !ok {
				continue
			}

			if err := fn(stackName, componentName, componentSection); err != nil {
				return err
			}
		}
	}

	return nil
}

// processTerraformComponent performs filtering and execution logic for a single Terraform component.
func processTerraformComponent(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	stackName, componentName string,
	componentSection map[string]any,
	logFunc func(msg any, keyvals ...any),
) error {
	metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return nil
	}

	// Skip abstract components
	if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
		return nil
	}

	// Skip disabled components
	if !isComponentEnabled(metadataSection, componentName) {
		return nil
	}

	command := fmt.Sprintf("atmos terraform %s %s -s %s", info.SubCommand, componentName, stackName)

	if info.Query != "" {
		queryResult, err := u.EvaluateYqExpression(atmosConfig, componentSection, info.Query)
		if err != nil {
			return err
		}

		if queryPassed, ok := queryResult.(bool); !ok || !queryPassed {
			logFunc("Skipping the component because the query criteria not satisfied", commandStr, command, "query", info.Query)
			return nil
		}
	}

	logFunc("Executing", commandStr, command)

	if !info.DryRun {
		info.Component = componentName
		info.ComponentFromArg = componentName
		info.Stack = stackName
		info.StackFromArg = stackName

		if err := ExecuteTerraform(*info); err != nil {
			return err
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

	var logFunc func(msg any, keyvals ...any)
	if info.DryRun {
		logFunc = log.Info
	} else {
		logFunc = log.Debug
	}

	stacks, err := ExecuteDescribeStacks(
		&atmosConfig,
		info.Stack,
		info.Components,
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

	err = walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		return processTerraformComponent(&atmosConfig, info, stackName, componentName, componentSection, logFunc)
	})
	if err != nil {
		return err
	}

	return nil
}
