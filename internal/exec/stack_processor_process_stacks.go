package exec

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ProcessStackConfig processes a stack configuration.
func ProcessStackConfig(
	atmosConfig *schema.AtmosConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	packerComponentsBasePath string,
	stack string,
	config map[string]any,
	processStackDeps bool,
	processComponentDeps bool,
	componentTypeFilter string,
	componentStackMap map[string]map[string][]string,
	importsConfig map[string]map[string]any,
	checkBaseComponentExists bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStackConfig")()

	stackName := strings.TrimSuffix(
		strings.TrimSuffix(
			u.TrimBasePathFromPath(stacksBasePath+"/", stack),
			u.DefaultStackConfigFileExtension),
		".yml",
	)

	globalVarsSection := map[string]any{}
	globalHooksSection := map[string]any{}
	globalSettingsSection := map[string]any{}
	globalEnvSection := map[string]any{}
	globalTerraformSection := map[string]any{}
	globalHelmfileSection := map[string]any{}
	globalPackerSection := map[string]any{}
	globalComponentsSection := map[string]any{}

	terraformVars := map[string]any{}
	terraformSettings := map[string]any{}
	terraformEnv := map[string]any{}
	terraformCommand := ""
	terraformProviders := map[string]any{}
	terraformHooks := map[string]any{}

	helmfileVars := map[string]any{}
	helmfileSettings := map[string]any{}
	helmfileEnv := map[string]any{}
	helmfileCommand := ""

	packerVars := map[string]any{}
	packerSettings := map[string]any{}
	packerEnv := map[string]any{}
	packerCommand := ""

	terraformComponents := map[string]any{}
	helmfileComponents := map[string]any{}
	packerComponents := map[string]any{}
	allComponents := map[string]any{}

	// Global sections.
	if i, ok := config[cfg.VarsSectionName]; ok {
		globalVarsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'vars' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.HooksSectionName]; ok {
		globalHooksSection, ok = i.(map[string]any)
		if !ok {
			return nil, errors.Wrapf(errUtils.ErrInvalidHooksSection, " '%s'", stackName)
		}
	}

	if i, ok := config[cfg.SettingsSectionName]; ok {
		globalSettingsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'settings' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.EnvSectionName]; ok {
		globalEnvSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'env' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.TerraformSectionName]; ok {
		globalTerraformSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.HelmfileSectionName]; ok {
		globalHelmfileSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.PackerSectionName]; ok {
		globalPackerSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.ComponentsSectionName]; ok {
		globalComponentsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'components' section in the file '%s'", stackName)
		}
	}

	// Terraform section.
	if i, ok := globalTerraformSection[cfg.CommandSectionName]; ok {
		terraformCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.VarsSectionName]; ok {
		terraformVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.vars' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.HooksSectionName]; ok {
		terraformHooks, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w '%s'", errUtils.ErrInvalidTerraformHooksSection, stackName)
		}
	}

	globalAndTerraformVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, terraformVars})
	if err != nil {
		return nil, err
	}

	globalAndTerraformHooks, err := m.Merge(atmosConfig, []map[string]any{globalHooksSection, terraformHooks})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.SettingsSectionName]; ok {
		terraformSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.settings' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, terraformSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.EnvSectionName]; ok {
		terraformEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.env' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, terraformEnv})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.ProvidersSectionName]; ok {
		terraformProviders, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.providers' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.HooksSectionName]; ok {
		terraformHooks, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.hooks' section in the file '%s'", stackName)
		}
	}

	// Global backend.
	globalBackendType := ""
	globalBackendSection := map[string]any{}

	if i, ok := globalTerraformSection[cfg.BackendTypeSectionName]; ok {
		globalBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.BackendSectionName]; ok {
		globalBackendSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend' section in the file '%s'", stackName)
		}
	}

	// Global remote state backend.
	globalRemoteStateBackendType := ""
	globalRemoteStateBackendSection := map[string]any{}

	if i, ok := globalTerraformSection[cfg.RemoteStateBackendTypeSectionName]; ok {
		globalRemoteStateBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.remote_state_backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.RemoteStateBackendSectionName]; ok {
		globalRemoteStateBackendSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.remote_state_backend' section in the file '%s'", stackName)
		}
	}

	// Helmfile section.
	if i, ok := globalHelmfileSection[cfg.CommandSectionName]; ok {
		helmfileCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalHelmfileSection[cfg.VarsSectionName]; ok {
		helmfileVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.vars' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, helmfileVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection[cfg.SettingsSectionName]; ok {
		helmfileSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.settings' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, helmfileSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection[cfg.EnvSectionName]; ok {
		helmfileEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.env' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, helmfileEnv})
	if err != nil {
		return nil, err
	}

	// Packer section.
	if i, ok := globalPackerSection[cfg.CommandSectionName]; ok {
		packerCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalPackerSection[cfg.VarsSectionName]; ok {
		packerVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.vars' section in the file '%s'", stackName)
		}
	}

	globalAndPackerVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, packerVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalPackerSection[cfg.SettingsSectionName]; ok {
		packerSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.settings' section in the file '%s'", stackName)
		}
	}

	globalAndPackerSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, packerSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalPackerSection[cfg.EnvSectionName]; ok {
		packerEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.env' section in the file '%s'", stackName)
		}
	}

	globalAndPackerEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, packerEnv})
	if err != nil {
		return nil, err
	}

	// Process all Terraform components.
	if componentTypeFilter == "" || componentTypeFilter == cfg.TerraformComponentType {
		if allTerraformComponents, ok := globalComponentsSection[cfg.TerraformComponentType]; ok {

			allTerraformComponentsMap, ok := allTerraformComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform' section in the file '%s'", stackName)
			}

			for cmp, v := range allTerraformComponentsMap {
				component := cmp

				componentMap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", component, stackName)
				}

				// Process component using helper function.
				opts := ComponentProcessorOptions{
					ComponentType:                   cfg.TerraformComponentType,
					Component:                       component,
					Stack:                           stack,
					StackName:                       stackName,
					ComponentMap:                    componentMap,
					AllComponentsMap:                allTerraformComponentsMap,
					ComponentsBasePath:              terraformComponentsBasePath,
					CheckBaseComponentExists:        checkBaseComponentExists,
					GlobalVars:                      globalAndTerraformVars,
					GlobalSettings:                  globalAndTerraformSettings,
					GlobalEnv:                       globalAndTerraformEnv,
					GlobalCommand:                   terraformCommand,
					TerraformProviders:              terraformProviders,
					GlobalAndTerraformHooks:         globalAndTerraformHooks,
					GlobalBackendType:               globalBackendType,
					GlobalBackendSection:            globalBackendSection,
					GlobalRemoteStateBackendType:    globalRemoteStateBackendType,
					GlobalRemoteStateBackendSection: globalRemoteStateBackendSection,
					AtmosConfig:                     atmosConfig,
				}

				result, err := processComponent(&opts)
				if err != nil {
					return nil, err
				}

				// Merge component configurations.
				comp, err := mergeComponentConfigurations(atmosConfig, opts, result)
				if err != nil {
					return nil, err
				}

				terraformComponents[component] = comp
			}
		}
	}

	// Process all Helmfile components.
	if componentTypeFilter == "" || componentTypeFilter == cfg.HelmfileComponentType {
		if allHelmfileComponents, ok := globalComponentsSection[cfg.HelmfileComponentType]; ok {

			allHelmfileComponentsMap, ok := allHelmfileComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.helmfile' section in the file '%s'", stackName)
			}

			for cmp, v := range allHelmfileComponentsMap {
				component := cmp

				componentMap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.helmfile.%s' section in the file '%s'", component, stackName)
				}

				// Process component using helper function.
				opts := ComponentProcessorOptions{
					ComponentType:            cfg.HelmfileComponentType,
					Component:                component,
					Stack:                    stack,
					StackName:                stackName,
					ComponentMap:             componentMap,
					AllComponentsMap:         allHelmfileComponentsMap,
					ComponentsBasePath:       helmfileComponentsBasePath,
					CheckBaseComponentExists: checkBaseComponentExists,
					GlobalVars:               globalAndHelmfileVars,
					GlobalSettings:           globalAndHelmfileSettings,
					GlobalEnv:                globalAndHelmfileEnv,
					GlobalCommand:            helmfileCommand,
					AtmosConfig:              atmosConfig,
				}

				result, err := processComponent(&opts)
				if err != nil {
					return nil, err
				}

				// Merge component configurations.
				comp, err := mergeComponentConfigurations(atmosConfig, opts, result)
				if err != nil {
					return nil, err
				}

				helmfileComponents[component] = comp
			}
		}
	}

	// Process all Packer components.
	if componentTypeFilter == "" || componentTypeFilter == cfg.PackerComponentType {
		if allPackerComponents, ok := globalComponentsSection[cfg.PackerComponentType]; ok {

			allPackerComponentsMap, ok := allPackerComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.packer' section in the file '%s'", stackName)
			}

			for cmp, v := range allPackerComponentsMap {
				component := cmp

				componentMap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.packer.%s' section in the file '%s'", component, stackName)
				}

				// Process component using helper function.
				opts := ComponentProcessorOptions{
					ComponentType:            cfg.PackerComponentType,
					Component:                component,
					Stack:                    stack,
					StackName:                stackName,
					ComponentMap:             componentMap,
					AllComponentsMap:         allPackerComponentsMap,
					ComponentsBasePath:       packerComponentsBasePath,
					CheckBaseComponentExists: checkBaseComponentExists,
					GlobalVars:               globalAndPackerVars,
					GlobalSettings:           globalAndPackerSettings,
					GlobalEnv:                globalAndPackerEnv,
					GlobalCommand:            packerCommand,
					AtmosConfig:              atmosConfig,
				}

				result, err := processComponent(&opts)
				if err != nil {
					return nil, err
				}

				// Merge component configurations.
				comp, err := mergeComponentConfigurations(atmosConfig, opts, result)
				if err != nil {
					return nil, err
				}

				packerComponents[component] = comp
			}
		}
	}

	allComponents[cfg.TerraformComponentType] = terraformComponents
	allComponents[cfg.HelmfileComponentType] = helmfileComponents
	allComponents[cfg.PackerComponentType] = packerComponents

	result := map[string]any{
		cfg.ComponentsSectionName: allComponents,
	}

	return result, nil
}
