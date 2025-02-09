package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	l "github.com/charmbracelet/log"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var terraformOutputsCache = sync.Map{}

func execTerraformOutput(atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	sections map[string]any,
) (map[string]any, error) {
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
		// Detect and set ENV variables from the `env` section
		envSection, ok := sections[cfg.EnvSectionName]
		if ok {
			envMap, ok2 := envSection.(map[string]any)
			if ok2 && len(envMap) > 0 {
				for k, v := range envMap {
					if v != nil {
						vstr := fmt.Sprintf("%v", v)
						l.Debug("Setting environment variable %s=%v for component %s in stack %s", k, vstr, component, stack)
						err := os.Setenv(k, vstr)
						if err != nil {
							return nil, err
						}
					}
				}
			}
		}

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

			l.Debug("Writing the backend config to file:", "file", backendFileName)

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

			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0o644)
			if err != nil {
				return nil, err
			}

			l.Debug("Wrote the backend config to file:", "file", backendFileName)
		}

		// Generate `providers_override.tf.json` file if the `providers` section is configured
		providersSection, ok := sections[cfg.ProvidersSectionName].(map[string]any)

		if ok && len(providersSection) > 0 {
			providerOverrideFileName := filepath.Join(componentPath, "providers_override.tf.json")

			l.Debug("Writing the provider overrides to file:", "file", providerOverrideFileName)

			providerOverrides := generateComponentProviderOverrides(providersSection)
			err = u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0o644)
			if err != nil {
				return nil, err
			}

			l.Debug("Wrote the provider overrides to file:", "file", providerOverrideFileName)
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

		l.Debug(fmt.Sprintf("Executing 'terraform init %s -s %s'", component, stack))

		var initOptions []tfexec.InitOption
		initOptions = append(initOptions, tfexec.Upgrade(false))
		// If `components.terraform.init_run_reconfigure` is set to `true` in atmos.yaml, add the `-reconfigure` flag to `terraform init`
		if atmosConfig.Components.Terraform.InitRunReconfigure {
			initOptions = append(initOptions, tfexec.Reconfigure(true))
		}
		err = tf.Init(ctx, initOptions...)
		if err != nil {
			return nil, err
		}

		l.Debug(fmt.Sprintf("Executed 'terraform init %s -s %s'", component, stack))

		// Terraform workspace
		l.Debug(fmt.Sprintf("Executing 'terraform workspace new %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		err = tf.WorkspaceNew(ctx, terraformWorkspace)
		if err != nil {
			l.Debug(fmt.Sprintf("Workspace exists. Executing 'terraform workspace select %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
			err = tf.WorkspaceSelect(ctx, terraformWorkspace)
			if err != nil {
				return nil, err
			}
			l.Debug(fmt.Sprintf("Executed 'terraform workspace select %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		} else {
			l.Debug(fmt.Sprintf("Executed 'terraform workspace new %s' for component '%s' in stack '%s'", terraformWorkspace, component, stack))
		}

		// Terraform output
		l.Debug(fmt.Sprintf("Executing 'terraform output %s -s %s'", component, stack))
		outputMeta, err := tf.Output(ctx)
		if err != nil {
			return nil, err
		}
		l.Debug(fmt.Sprintf("Executed 'terraform output %s -s %s'", component, stack))

		if atmosConfig.Logs.Level == u.LogLevelTrace {
			y, err2 := u.ConvertToYAML(outputMeta)
			if err2 != nil {
				l.Error("Error converting output to YAML:", "error", err2)
			} else {
				l.Debug(fmt.Sprintf("Result of 'terraform output %s -s %s' before processing it:\n%s\n", component, stack, y))
			}
		}

		outputProcessed = lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
			s := string(v.Value)
			l.Debug(fmt.Sprintf("Converting the variable '%s' with the value\n%s\nfrom JSON to 'Go' data type\n", k, s))

			d, err2 := u.ConvertFromJSON(s)

			if err2 != nil {
				l.Error("failed to convert output”, “output”, "error", err2)
				return k, nil
			} else {
				l.Debug("Converted the variable from JSON to Go data type", “key”, k, “value”, s, “result”, d))
			}

			return k, d
		})
	} else {
		componentStatus := "disabled"
		if componentAbstract {
			componentStatus = "abstract"
		}
		l.Debug(fmt.Sprintf("Not executing 'terraform output %s -s %s' because the component is %s", component, stack, componentStatus))
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
	message := fmt.Sprintf("Fetching %s output from %s in %s", output, component, stack)

	// If the result for the component in the stack already exists in the cache, return it
	if !skipCache {
		cachedOutputs, found := terraformOutputsCache.Load(stackSlug)
		if found && cachedOutputs != nil {
			l.Debug("Found the result of the Atmos YAML function '!terraform.output %s %s %s' in the cache", component, stack, output)
			return getTerraformOutputVariable(atmosConfig, component, stack, cachedOutputs.(map[string]any), output)
		}
	}

	// Initialize spinner
	s := spinner.New()
	s.Style = theme.Styles.Link

	var opts []tea.ProgramOption
	if !CheckTTYSupport() {
		// set tea.WithInput(nil) workaround tea program not run on not TTY mod issue
		opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
		l.Debug("No TTY detected. Falling back to basic output. This can happen when no terminal is attached or when commands are pipelined.")
		fmt.Println(message)
	}

	p := tea.NewProgram(modelSpinner{
		spinner: s,
		message: message,
	}, opts...)

	spinnerDone := make(chan struct{})
	go func() {
		if _, err := p.Run(); err != nil {
			// If there's any error running the spinner, just print the message
			fmt.Println(message)
			l.Error("Failed to run spinner:", "error", err)
		}
		close(spinnerDone)
	}()

	sections, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	if err != nil {
		p.Quit()
		<-spinnerDone
		fmt.Printf("\r✗ %s\n", message)
		l.Error("Failed to describe the component", "component", component, "stack", stack, "error", err)
		os.Exit(1)
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs, err := GetComponentRemoteStateBackendStaticType(sections)
	if err != nil {
		p.Quit()
		<-spinnerDone
		fmt.Printf("\r✗ %s\n", message)
		l.Error("Failed to get remote state backend static type outputs", "error", err)
		os.Exit(1)
	}

	var result any
	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformOutputsCache.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		result = getStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateBackendStaticTypeOutputs, output)
	} else {
		// Execute `terraform output`
		terraformOutputs, err := execTerraformOutput(atmosConfig, component, stack, sections)
		if err != nil {
			p.Quit()
			<-spinnerDone
			fmt.Printf("\r✗ %s\n", message)
			l.Error("Failed to execute terraform output", "component", component, "stack", stack, "error", err)
			os.Exit(1)
		}

		// Cache the result
		terraformOutputsCache.Store(stackSlug, terraformOutputs)
		result = getTerraformOutputVariable(atmosConfig, component, stack, terraformOutputs, output)
	}

	// Stop spinner and show success
	p.Quit()
	<-spinnerDone
	fmt.Printf("\r✓ %s\n", message)

	return result
}

func getTerraformOutputVariable(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	outputs map[string]any,
	output string,
) any {
	val := output
	if !strings.HasPrefix(output, ".") {
		val = "." + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, outputs, val)
	if err != nil {
		l.Error("Error evaluating terraform output", "output", output, "component", component, "stack", stack, "error", err)
		os.Exit(1)
	}

	return res
}

func getStaticRemoteStateOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) any {
	val := output
	if !strings.HasPrefix(output, ".") {
		val = "." + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, remoteStateSection, val)
	if err != nil {
		l.Error("Error evaluating the 'static' remote state backend output", "output", output, "component", component, "stack", stack, "error", err)
		os.Exit(1)
	}

	return res
}
