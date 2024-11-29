package exec

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/config"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	terraformOutputFuncSyncMap = sync.Map{}
)

func processTagTerraformOutput(cliConfig schema.CliConfiguration, input string) any {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	part := strings.TrimPrefix(input, config.AtmosYamlFuncTerraformOutput)
	part = strings.TrimSpace(part)

	if part == "" {
		err := errors.New(fmt.Sprintf("invalid Atmos YAML function: %s\nthree parameters are required: component, stack, output", input))
		u.LogErrorAndExit(cliConfig, err)
	}

	parts := strings.Split(part, " ")

	if len(parts) != 3 {
		err := errors.New(fmt.Sprintf("invalid Atmos YAML function: %s\nthree parameters are required: component, stack, output", input))
		u.LogErrorAndExit(cliConfig, err)
	}

	component := strings.TrimSpace(parts[0])
	stack := strings.TrimSpace(parts[1])
	output := strings.TrimSpace(parts[2])

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	cachedOutputs, found := terraformOutputFuncSyncMap.Load(stackSlug)
	if found && cachedOutputs != nil {
		u.LogTrace(cliConfig, fmt.Sprintf("Found the result of the Atmos YAML function '!terraform.output %s %s %s' in the cache", component, stack, output))
		return getTerraformOutput(cliConfig, input, component, stack, cachedOutputs.(map[string]any), output)
	}

	sections, err := ExecuteDescribeComponent(component, stack, true)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	outputProcessed := map[string]any{}

	executable, ok := sections[cfg.CommandSectionName].(string)
	if !ok {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' does not have 'command' (executable) defined",
			input,
			component,
			stack,
		))
	}

	terraformWorkspace, ok := sections[cfg.WorkspaceSectionName].(string)
	if !ok {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' does not have Terraform/OpenTofu workspace defined",
			input,
			component,
			stack,
		))
	}

	componentInfo, ok := sections["component_info"]
	if !ok {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' does not have 'component_info' defined",
			input,
			component,
			stack,
		))
	}

	componentInfoMap, ok := componentInfo.(map[string]any)
	if !ok {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' has an invalid 'component_info' section",
			input,
			component,
			stack,
		))
	}

	componentPath, ok := componentInfoMap["component_path"].(string)
	if !ok {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' has an invalid 'component_info.component_path' section",
			input,
			component,
			stack,
		))
	}

	// Auto-generate backend file
	if cliConfig.Components.Terraform.AutoGenerateBackendFile {
		backendFileName := path.Join(componentPath, "backend.tf.json")

		u.LogTrace(cliConfig, "\nWriting the backend config to file:")
		u.LogTrace(cliConfig, backendFileName)

		backendTypeSection, ok := sections["backend_type"].(string)
		if !ok {
			u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' has an invalid 'backend_type' section",
				input,
				component,
				stack,
			))
		}

		backendSection, ok := sections["backend"].(map[string]any)
		if !ok {
			u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' has an invalid 'backend' section",
				input,
				component,
				stack,
			))
		}

		componentBackendConfig, err := generateComponentBackendConfig(backendTypeSection, backendSection, terraformWorkspace)
		if err != nil {
			u.LogErrorAndExit(cliConfig, err)
		}

		err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
		if err != nil {
			u.LogErrorAndExit(cliConfig, err)
		}

		u.LogTrace(cliConfig, "\nWrote the backend config to file:")
		u.LogTrace(cliConfig, backendFileName)
	}

	// Generate `providers_override.tf.json` file if the `providers` section is configured
	providersSection, ok := sections["providers"].(map[string]any)

	if ok && len(providersSection) > 0 {
		providerOverrideFileName := path.Join(componentPath, "providers_override.tf.json")

		u.LogTrace(cliConfig, "\nWriting the provider overrides to file:")
		u.LogTrace(cliConfig, providerOverrideFileName)

		var providerOverrides = generateComponentProviderOverrides(providersSection)
		err = u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0644)
		if err != nil {
			u.LogErrorAndExit(cliConfig, err)
		}

		u.LogTrace(cliConfig, "\nWrote the provider overrides to file:")
		u.LogTrace(cliConfig, providerOverrideFileName)
	}

	// Initialize Terraform/OpenTofu
	tf, err := tfexec.NewTerraform(componentPath, executable)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	ctx := context.Background()

	// 'terraform init'
	// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory
	cleanTerraformWorkspace(cliConfig, componentPath)

	u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nexecuting 'terraform init %s -s %s'", input, component, stack))
	err = tf.Init(ctx, tfexec.Upgrade(false))
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}
	u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nexecuted 'terraform init %s -s %s'", input, component, stack))

	// Terraform workspace
	u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nexecuting 'terraform workspace new %s' for component '%s' in stack '%s'", input, terraformWorkspace, component, stack))

	err = tf.WorkspaceNew(ctx, terraformWorkspace)
	if err != nil {
		u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nterraform workspace exists. Executing 'terraform workspace select %s' for component '%s' in stack '%s'", input, terraformWorkspace, component, stack))
		err = tf.WorkspaceSelect(ctx, terraformWorkspace)
		if err != nil {
			u.LogErrorAndExit(cliConfig, err)
		}
		u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nexecuted 'terraform workspace select %s' for component '%s' in stack '%s'", input, terraformWorkspace, component, stack))
	} else {
		u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nexecuted 'terraform workspace new %s' for component '%s' in stack '%s'", input, terraformWorkspace, component, stack))
	}

	// Terraform output
	u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nexecuting 'terraform output %s -s %s'", input, component, stack))

	outputMeta, err := tf.Output(ctx)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}
	u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nexecuted 'terraform output %s -s %s'", input, component, stack))

	if cliConfig.Logs.Level == u.LogLevelTrace {
		y, err2 := u.ConvertToYAML(outputMeta)
		if err2 != nil {
			u.LogError(cliConfig, err2)
		} else {
			u.LogTrace(cliConfig, fmt.Sprintf("\nAtmos YAML function: %s\nresult of 'terraform output %s -s %s' before processing it:\n%s\n", input, component, stack, y))
		}
	}

	outputProcessed = lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
		s := string(v.Value)
		u.LogTrace(cliConfig, fmt.Sprintf("Atmos YAML function: %s\nconverting the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\n", input, k, s))

		d, err2 := u.ConvertFromJSON(s)

		if err2 != nil {
			u.LogError(cliConfig, err2)
		} else {
			u.LogTrace(cliConfig, fmt.Sprintf("Atmos YAML function: %s\nconverted the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\nResult: %v\n", input, k, s, d))
		}

		return k, d
	})

	// Cache the result
	terraformOutputFuncSyncMap.Store(stackSlug, outputProcessed)

	return getTerraformOutput(cliConfig, input, component, stack, outputProcessed, output)
}

func getTerraformOutput(
	cliConfig schema.CliConfiguration,
	funcDef string,
	component string,
	stack string,
	outputs map[string]any,
	output string,
) any {
	if u.MapKeyExists(outputs, output) {
		return outputs[output]
	}

	u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' does not have the output '%s'",
		funcDef,
		component,
		stack,
		output,
	))

	return nil
}
