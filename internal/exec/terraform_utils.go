package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	l "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

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
		l.Debug("Terraform environment file found. Proceeding with deletion.", "file", filePath)

		if err := os.Remove(filePath); err != nil {
			l.Debug("Failed to delete Terraform environment file.", "file", filePath, "error", err)
		} else {
			l.Debug("Successfully deleted Terraform environment file.", "file", filePath)
		}
	} else if os.IsNotExist(err) {
		l.Debug("Terraform environment file not found. No action needed.", "file", filePath)
	} else {
		l.Debug("Error checking Terraform environment file.", "file", filePath, "error", err)
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

		l.Debug("Writing the backend config to file.", "file", backendFileName)

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

		l.Debug("Writing the provider overrides to file.", "file", providerOverrideFileName)

		if !info.DryRun {
			providerOverrides := generateComponentProviderOverrides(info.ComponentProvidersSection)
			err := u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0o600)
			return err
		}
	}
	return nil
}

// needProcessTemplatesAndYamlFunctions checks if a Terraform command
// requires the `Go` templates and Atmos YAML functions to be processed
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
			l.Warn("ignoring unsupported workspaces `enabled` setting for HTTP backend type.",
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

// ExecuteTerraformAffected executes `atmos terraform --affected`
func ExecuteTerraformAffected(cmd *cobra.Command, args []string, info schema.ConfigAndStacksInfo) error {
	// Add these flags here because `atmos describe affected` reads/needs them, but `atmos terraform --affected` does not define them
	cmd.PersistentFlags().String("file", "", "")
	cmd.PersistentFlags().String("format", "yaml", "")
	cmd.PersistentFlags().Bool("verbose", false, "")
	cmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "")
	cmd.PersistentFlags().Bool("include-settings", false, "")
	cmd.PersistentFlags().Bool("upload", false, "")
	cmd.PersistentFlags().StringP("query", "q", "", "")

	cliArgs, err := parseDescribeAffectedCliArgs(cmd, args)
	if err != nil {
		return err
	}

	cliArgs.IncludeSpaceliftAdminStacks = false
	cliArgs.IncludeSettings = false
	cliArgs.Upload = false
	cliArgs.OutputFile = ""
	cliArgs.Query = ""

	// https://atmos.tools/cli/commands/describe/affected
	affectedList, _, _, _, err := ExecuteDescribeAffected(cliArgs)
	if err != nil {
		return err
	}

	affectedYaml, err := u.ConvertToYAML(affectedList)
	if err != nil {
		return err
	}
	l.Debug("Affected components:\n" + affectedYaml)

	for _, affected := range affectedList {
		err = executeTerraformAffectedComponent(affected, info, "", "", cliArgs)
		if err != nil {
			return err
		}
	}

	return nil
}

// executeTerraformAffectedComponent recursively processes the affected components in the dependency order
func executeTerraformAffectedComponent(
	affected schema.Affected,
	info schema.ConfigAndStacksInfo,
	parentComponent string,
	parentStack string,
	args DescribeAffectedCmdArgs,
) error {
	// If the affected component is included as dependent in other components, don't process it now,
	// it will be processed in the dependency order
	if !affected.IncludedInDependents {
		info.Component = affected.Component
		info.ComponentFromArg = affected.Component
		info.Stack = affected.Stack

		if parentComponent != "" && parentStack != "" {
			l.Debug(fmt.Sprintf("Executing 'atmos terraform %s %s -s %s' as dependency of component '%s' in stack '%s'",
				info.SubCommand,
				affected.Component,
				affected.Stack,
				parentComponent,
				parentStack,
			))
		} else {
			l.Debug(fmt.Sprintf("Executing 'atmos terraform %s %s -s %s'",
				info.SubCommand,
				affected.Component,
				affected.Stack,
			))
		}

		// Execute the terraform command for the affected component
		//err := ExecuteTerraform(info)
		//if err != nil {
		//	return err
		//}
	} else if args.IncludeDependents {
		if parentComponent != "" && parentStack != "" {
			l.Debug(fmt.Sprintf("Skipping 'atmos terraform %s %s -s %s' because it's a dependency of component '%s' in stack '%s'",
				info.SubCommand,
				affected.Component,
				affected.Stack,
				parentComponent,
				parentStack,
			))
		} else {
			l.Debug(fmt.Sprintf("Skipping 'atmos terraform %s %s -s %s' because it's a dependency of another component",
				info.SubCommand,
				affected.Component,
				affected.Stack,
			))
		}
	}

	if args.IncludeDependents {
		for _, dep := range affected.Dependents {
			affectedDep := schema.Affected{
				Component:  dep.Component,
				Stack:      dep.Stack,
				Dependents: dep.Dependents,
			}
			err := executeTerraformAffectedComponent(affectedDep, info, affected.Component, affected.Stack, args)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ExecuteTerraformAll executes `atmos terraform --all`
func ExecuteTerraformAll(cmd *cobra.Command, args []string, info schema.ConfigAndStacksInfo) error {
	return nil
}
