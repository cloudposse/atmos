package exec

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	terraformOutputsCache = sync.Map{}
)

func execTerraformOutput(atmosConfig *schema.AtmosConfiguration, component string, stack string, sections map[string]any) (map[string]any, error) {
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
		if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
			backendFileName := filepath.Join(componentPath, "backend.tf.json")

			u.LogTrace(*atmosConfig, "\nWriting the backend config to file:")
			u.LogTrace(*atmosConfig, backendFileName)

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

			u.LogTrace(*atmosConfig, "\nWrote the backend config to file:")
			u.LogTrace(*atmosConfig, backendFileName)
		}

		// Generate `providers_override.tf.json` file if the `providers` section is configured
		providersSection, ok := sections["providers"].(map[string]any)

		if ok && len(providersSection) > 0 {
			providerOverrideFileName := filepath.Join(componentPath, "providers_override.tf.json")

			u.LogTrace(*atmosConfig, "\nWriting the provider overrides to file:")
			u.LogTrace(*atmosConfig, providerOverrideFileName)

			var providerOverrides = generateComponentProviderOverrides(providersSection)
			err = u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0644)
			if err != nil {
				return nil, err
			}

			u.LogTrace(*atmosConfig, "\nWrote the provider overrides to file:")
			u.LogTrace(*atmosConfig, providerOverrideFileName)
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
		cleanTerraformWorkspace(*atmosConfig, componentPath)

		u.LogTrace(*atmosConfig, fmt.Sprintf("\nExecuting 'terraform init %s -s %s'", component, stack))
		err = tf.Init(ctx, tfexec.Upgrade(false))
		if err != nil {
			return nil, err
		}
		u.LogTrace(*atmosConfig, fmt.Sprintf("\nExecuted 'terraform init %s -s %s'", component, stack))

		// Terraform workspace
		u.LogTrace(*atmosConfig, fmt.Sprintf("\nExecuting 'terraform workspace new %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		err = tf.WorkspaceNew(ctx, terraformWorkspace)
		if err != nil {
			u.LogTrace(*atmosConfig, fmt.Sprintf("\nWorkspace exists. Executing 'terraform workspace select %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
			err = tf.WorkspaceSelect(ctx, terraformWorkspace)
			if err != nil {
				return nil, err
			}
			u.LogTrace(*atmosConfig, fmt.Sprintf("\nExecuted 'terraform workspace select %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		} else {
			u.LogTrace(*atmosConfig, fmt.Sprintf("\nExecuted 'terraform workspace new %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		}

		// Terraform output
		u.LogTrace(*atmosConfig, fmt.Sprintf("\nExecuting 'terraform output %s -s %s'", component, stack))
		outputMeta, err := tf.Output(ctx)
		if err != nil {
			return nil, err
		}
		u.LogTrace(*atmosConfig, fmt.Sprintf("\nExecuted 'terraform output %s -s %s'", component, stack))

		if atmosConfig.Logs.Level == u.LogLevelTrace {
			y, err2 := u.ConvertToYAML(outputMeta)
			if err2 != nil {
				u.LogError(*atmosConfig, err2)
			} else {
				u.LogTrace(*atmosConfig, fmt.Sprintf("\nResult of 'terraform output %s -s %s' before processing it:\n%s\n", component, stack, y))
			}
		}

		outputProcessed = lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
			s := string(v.Value)
			u.LogTrace(*atmosConfig, fmt.Sprintf("Converting the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\n", k, s))

			d, err2 := u.ConvertFromJSON(s)

			if err2 != nil {
				u.LogError(*atmosConfig, fmt.Errorf("failed to convert output '%s': %w", k, err2))
				return k, nil
			} else {
				u.LogTrace(*atmosConfig, fmt.Sprintf("Converted the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\nResult: %v\n", k, s, d))
			}

			return k, d
		})
	} else {
		componentType := "disabled"
		if componentAbstract {
			componentType = "abstract"
		}
		u.LogTrace(*atmosConfig, fmt.Sprintf("\nNot executing 'terraform output %s -s %s' because the component is %s", component, stack, componentType))
	}

	return outputProcessed, nil
}

func GetTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
) any {

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	if !skipCache {
		cachedOutputs, found := terraformOutputsCache.Load(stackSlug)
		if found && cachedOutputs != nil {
			u.LogTrace(*atmosConfig, fmt.Sprintf("Found the result of the Atmos YAML function '!terraform.output %s %s %s' in the cache", component, stack, output))
			return getTerraformOutput(atmosConfig, component, stack, cachedOutputs.(map[string]any), output)
		}
	}

	sections, err := ExecuteDescribeComponent(component, stack, true)
	if err != nil {
		u.LogErrorAndExit(*atmosConfig, err)
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs, err := GetComponentRemoteStateBackendStaticType(sections)
	if err != nil {
		u.LogErrorAndExit(*atmosConfig, err)
	}

	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformOutputsCache.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		return getStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateBackendStaticTypeOutputs, output)
	} else {
		// Execute `terraform output`
		terraformOutputs, err := execTerraformOutput(atmosConfig, component, stack, sections)
		if err != nil {
			u.LogErrorAndExit(*atmosConfig, err)
		}

		// Cache the result
		terraformOutputsCache.Store(stackSlug, terraformOutputs)
		return getTerraformOutput(atmosConfig, component, stack, terraformOutputs, output)
	}
}

func getTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	outputs map[string]any,
	output string,
) any {
	if u.MapKeyExists(outputs, output) {
		return outputs[output]
	}

	u.LogErrorAndExit(*atmosConfig, fmt.Errorf("invalid terrform output lookup: the component '%s' in the stack '%s' does not have the output '%s'",
		component,
		stack,
		output,
	))

	return nil
}

func getStaticRemoteStateOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) any {
	if u.MapKeyExists(remoteStateSection, output) {
		return remoteStateSection[output]
	}

	u.LogErrorAndExit(*atmosConfig, fmt.Errorf("invalid terrform output lookup: the component '%s' in the stack '%s' "+
		"is configured with the 'static' remote state backend, but the remote state backend does not have the output '%s'",
		component,
		stack,
		output,
	))

	return nil
}
