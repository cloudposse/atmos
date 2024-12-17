package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func checkTerraformConfig(cliConfig schema.CliConfiguration) error {
	if len(cliConfig.Components.Terraform.BasePath) < 1 {
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
func cleanTerraformWorkspace(cliConfig schema.CliConfiguration, componentPath string) {
	filePath := filepath.Join(componentPath, ".terraform", "environment")
	u.LogDebug(cliConfig, fmt.Sprintf("\nDeleting Terraform environment file:\n'%s'", filePath))
	_ = os.Remove(filePath)
}

func execTerraformOutput(cliConfig schema.CliConfiguration, component string, stack string, sections map[string]any) (map[string]any, error) {
	outputProcessed := map[string]any{}
	componentAbstract := false
	componentEnabled := true
	var err error

	metadataSection, ok := sections[cfg.MetadataSectionName]
	if ok {
		metadata, ok2 := metadataSection.(map[string]any)
		if ok2 {
			componentAbstract = IsComponentAbstract(metadata)
		}
	}

	varsSection, ok := sections[cfg.VarsSectionName]
	if ok {
		vars, ok2 := varsSection.(map[string]any)
		if ok2 {
			componentEnabled = IsComponentEnabled(vars)
		}
	}

	// Don't process Terraform output for disabled and abstract components
	if componentEnabled && !componentAbstract {
		executable, ok := sections[cfg.CommandSectionName].(string)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have 'command' (executable) defined", component, stack)
		}

		terraformWorkspace, ok := sections[cfg.WorkspaceSectionName].(string)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have Terraform/OpenTofu workspace defined", component, stack)
		}

		componentInfo, ok := sections["component_info"]
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have 'component_info' defined", component, stack)
		}

		componentInfoMap, ok := componentInfo.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'component_info' section", component, stack)
		}

		componentPath, ok := componentInfoMap["component_path"].(string)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'component_info.component_path' section", component, stack)
		}

		// Auto-generate backend file
		if cliConfig.Components.Terraform.AutoGenerateBackendFile {
			backendFileName := filepath.Join(componentPath, "backend.tf.json")

			u.LogTrace(cliConfig, "\nWriting the backend config to file:")
			u.LogTrace(cliConfig, backendFileName)

			backendTypeSection, ok := sections["backend_type"].(string)
			if !ok {
				return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'backend_type' section", component, stack)
			}

			backendSection, ok := sections["backend"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'backend' section", component, stack)
			}

			componentBackendConfig, err := generateComponentBackendConfig(backendTypeSection, backendSection, terraformWorkspace)
			if err != nil {
				return nil, err
			}

			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
			if err != nil {
				return nil, err
			}

			u.LogTrace(cliConfig, "\nWrote the backend config to file:")
			u.LogTrace(cliConfig, backendFileName)
		}

		// Generate `providers_override.tf.json` file if the `providers` section is configured
		providersSection, ok := sections["providers"].(map[string]any)

		if ok && len(providersSection) > 0 {
			providerOverrideFileName := filepath.Join(componentPath, "providers_override.tf.json")

			u.LogTrace(cliConfig, "\nWriting the provider overrides to file:")
			u.LogTrace(cliConfig, providerOverrideFileName)

			var providerOverrides = generateComponentProviderOverrides(providersSection)
			err = u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0644)
			if err != nil {
				return nil, err
			}

			u.LogTrace(cliConfig, "\nWrote the provider overrides to file:")
			u.LogTrace(cliConfig, providerOverrideFileName)
		}

		// Initialize Terraform/OpenTofu
		tf, err := tfexec.NewTerraform(componentPath, executable)
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
		defer cancel()

		// 'terraform init'
		// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory
		cleanTerraformWorkspace(cliConfig, componentPath)

		u.LogTrace(cliConfig, fmt.Sprintf("\nExecuting 'terraform init %s -s %s'", component, stack))
		err = tf.Init(ctx, tfexec.Upgrade(false))
		if err != nil {
			return nil, err
		}
		u.LogTrace(cliConfig, fmt.Sprintf("\nExecuted 'terraform init %s -s %s'", component, stack))

		// Terraform workspace
		u.LogTrace(cliConfig, fmt.Sprintf("\nExecuting 'terraform workspace new %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		err = tf.WorkspaceNew(ctx, terraformWorkspace)
		if err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("\nWorkspace exists. Executing 'terraform workspace select %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
			err = tf.WorkspaceSelect(ctx, terraformWorkspace)
			if err != nil {
				return nil, err
			}
			u.LogTrace(cliConfig, fmt.Sprintf("\nExecuted 'terraform workspace select %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		} else {
			u.LogTrace(cliConfig, fmt.Sprintf("\nExecuted 'terraform workspace new %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		}

		// Terraform output
		u.LogTrace(cliConfig, fmt.Sprintf("\nExecuting 'terraform output %s -s %s'", component, stack))
		outputMeta, err := tf.Output(ctx)
		if err != nil {
			return nil, err
		}
		u.LogTrace(cliConfig, fmt.Sprintf("\nExecuted 'terraform output %s -s %s'", component, stack))

		if cliConfig.Logs.Level == u.LogLevelTrace {
			y, err2 := u.ConvertToYAML(outputMeta)
			if err2 != nil {
				u.LogError(cliConfig, err2)
			} else {
				u.LogTrace(cliConfig, fmt.Sprintf("\nResult of 'terraform output %s -s %s' before processing it:\n%s\n", component, stack, y))
			}
		}

		outputProcessed = lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
			s := string(v.Value)
			u.LogTrace(cliConfig, fmt.Sprintf("Converting the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\n", k, s))

			d, err2 := u.ConvertFromJSON(s)

			if err2 != nil {
				u.LogError(cliConfig, fmt.Errorf("failed to convert output '%s': %w", k, err2))
				return k, nil
			} else {
				u.LogTrace(cliConfig, fmt.Sprintf("Converted the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\nResult: %v\n", k, s, d))
			}

			return k, d
		})
	} else {
		componentType := "disabled"
		if componentAbstract {
			componentType = "abstract"
		}
		u.LogTrace(cliConfig, fmt.Sprintf("\nNot executing 'terraform output %s -s %s' because the component is %s", component, stack, componentType))
	}

	return outputProcessed, nil
}
