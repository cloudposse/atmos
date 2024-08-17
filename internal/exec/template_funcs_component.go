package exec

import (
	"context"
	"fmt"
	"path"
	"sync"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	componentFuncSyncMap = sync.Map{}
)

func componentFunc(cliConfig schema.CliConfiguration, component string, stack string) (any, error) {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing template function 'atmos.Component(%s, %s)'", component, stack))

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	existingSections, found := componentFuncSyncMap.Load(stackSlug)
	if found && existingSections != nil {
		if cliConfig.Logs.Level == u.LogLevelTrace {
			u.LogTrace(cliConfig, fmt.Sprintf("Found the result of the template function 'atmos.Component(%s, %s)' in the cache", component, stack))

			if outputsSection, ok := existingSections.(map[string]any)["outputs"]; ok {
				u.LogTrace(cliConfig, "'outputs' section:")
				y, err2 := u.ConvertToYAML(outputsSection)
				if err2 != nil {
					u.LogError(err2)
				} else {
					u.LogTrace(cliConfig, y)
				}
			}
		}

		return existingSections, nil
	}

	sections, err := ExecuteDescribeComponent(component, stack, true)
	if err != nil {
		return nil, err
	}

	executable, ok := sections["command"].(string)
	if !ok {
		return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have 'command' (executable) defined", component, stack)
	}

	terraformWorkspace, ok := sections["workspace"].(string)
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
		backendFileName := path.Join(componentPath, "backend.tf.json")

		u.LogTrace(cliConfig, "\nWriting the backend config to file:")
		u.LogTrace(cliConfig, backendFileName)

		backendTypeSection, ok := sections["backend_type"].(string)
		if !ok {
			return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'backend_type' section", component, stack)
		}

		backendSection, ok := sections["backend"].(map[any]any)
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
	providersSection, ok := sections["providers"].(map[any]any)

	if ok && len(providersSection) > 0 {
		providerOverrideFileName := path.Join(componentPath, "providers_override.tf.json")

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

	ctx := context.Background()

	// 'terraform init'
	err = tf.Init(ctx, tfexec.Upgrade(false))
	if err != nil {
		return nil, err
	}

	// Terraform workspace
	err = tf.WorkspaceNew(ctx, terraformWorkspace)
	if err != nil {
		err = tf.WorkspaceSelect(ctx, terraformWorkspace)
		if err != nil {
			return nil, err
		}
	}

	// Terraform output
	outputMeta, err := tf.Output(ctx)
	if err != nil {
		return nil, err
	}

	if cliConfig.Logs.Level == u.LogLevelTrace {
		y, err2 := u.ConvertToYAML(outputMeta)
		if err2 != nil {
			u.LogError(err2)
		} else {
			u.LogTrace(cliConfig, fmt.Sprintf("\nResult of 'atmos terraform output %s -s %s' before processing it:\n%s\n", component, stack, y))
		}
	}

	outputMetaProcessed := lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
		s := string(v.Value)
		u.LogTrace(cliConfig, fmt.Sprintf("Converting the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\n", k, s))

		d, err2 := u.ConvertFromJSON(s)

		if err2 != nil {
			u.LogError(err2)
		} else {
			u.LogTrace(cliConfig, fmt.Sprintf("Converted the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\nResult: %v\n", k, s, d))
		}

		return k, d
	})

	outputs := map[string]any{
		"outputs": outputMetaProcessed,
	}

	sections = lo.Assign(sections, outputs)

	// Cache the result
	componentFuncSyncMap.Store(stackSlug, sections)

	if cliConfig.Logs.Level == u.LogLevelTrace {
		u.LogTrace(cliConfig, fmt.Sprintf("Executed template function 'atmos.Component(%s, %s)'\n\n'outputs' section:", component, stack))
		y, err2 := u.ConvertToYAML(outputMetaProcessed)
		if err2 != nil {
			u.LogError(err2)
		} else {
			u.LogTrace(cliConfig, y)
		}
	}

	return sections, nil
}
