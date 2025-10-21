package exec

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// ErrFormatWithFile is the error format string for errors with file context.
	errFormatWithFile = "%w in file '%s'"
)

// ProcessStackConfig processes a stack configuration.
//
//nolint:gocognit,nestif,revive,cyclop,funlen // Core stack processing logic with complex configuration handling.
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
	globalAuthSection := map[string]any{}

	terraformVars := map[string]any{}
	terraformSettings := map[string]any{}
	terraformEnv := map[string]any{}
	terraformCommand := ""
	terraformProviders := map[string]any{}
	terraformHooks := map[string]any{}
	terraformAuth := map[string]any{}

	helmfileVars := map[string]any{}
	helmfileSettings := map[string]any{}
	helmfileEnv := map[string]any{}
	helmfileCommand := ""
	helmfileAuth := map[string]any{}

	packerVars := map[string]any{}
	packerSettings := map[string]any{}
	packerEnv := map[string]any{}
	packerCommand := ""
	packerAuth := map[string]any{}

	terraformComponents := map[string]any{}
	helmfileComponents := map[string]any{}
	packerComponents := map[string]any{}
	allComponents := map[string]any{}

	// Global sections.
	if i, ok := config[cfg.VarsSectionName]; ok {
		globalVarsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidVarsSection, stackName)
		}
	}

	if i, ok := config[cfg.HooksSectionName]; ok {
		globalHooksSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidHooksSection, stackName)
		}
	}

	if i, ok := config[cfg.SettingsSectionName]; ok {
		globalSettingsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidSettingsSection, stackName)
		}
	}

	if i, ok := config[cfg.EnvSectionName]; ok {
		globalEnvSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidEnvSection, stackName)
		}
	}

	if i, ok := config[cfg.TerraformSectionName]; ok {
		globalTerraformSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformSection, stackName)
		}
	}

	if i, ok := config[cfg.HelmfileSectionName]; ok {
		globalHelmfileSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidHelmfileSection, stackName)
		}
	}

	if i, ok := config[cfg.PackerSectionName]; ok {
		globalPackerSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidPackerSection, stackName)
		}
	}

	if i, ok := config[cfg.ComponentsSectionName]; ok {
		globalComponentsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidComponentsSection, stackName)
		}
	}

	if i, ok := config[cfg.AuthSectionName]; ok {
		globalAuthSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidAuthSection, stackName)
		}
	}

	// Terraform section.
	if i, ok := globalTerraformSection[cfg.CommandSectionName]; ok {
		terraformCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformCommand, stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.VarsSectionName]; ok {
		terraformVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformVars, stackName)
		}
	}

	globalAndTerraformVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, terraformVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.HooksSectionName]; ok {
		terraformHooks, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w '%s'", errUtils.ErrInvalidTerraformHooksSection, stackName)
		}
	}

	globalAndTerraformHooks, err := m.Merge(atmosConfig, []map[string]any{globalHooksSection, terraformHooks})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.SettingsSectionName]; ok {
		terraformSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformSettings, stackName)
		}
	}

	globalAndTerraformSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, terraformSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.EnvSectionName]; ok {
		terraformEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformEnv, stackName)
		}
	}

	globalAndTerraformEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, terraformEnv})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.ProvidersSectionName]; ok {
		terraformProviders, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformProviders, stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.AuthSectionName]; ok {
		terraformAuth, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformAuth, stackName)
		}
	}

	globalAndTerraformAuth, err := m.Merge(atmosConfig, []map[string]any{globalAuthSection, terraformAuth})
	if err != nil {
		return nil, err
	}

	// Global backend.
	globalBackendType := ""
	globalBackendSection := map[string]any{}

	if i, ok := globalTerraformSection[cfg.BackendTypeSectionName]; ok {
		globalBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformBackendType, stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.BackendSectionName]; ok {
		globalBackendSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformBackend, stackName)
		}
	}

	// Global remote state backend.
	globalRemoteStateBackendType := ""
	globalRemoteStateBackendSection := map[string]any{}

	if i, ok := globalTerraformSection[cfg.RemoteStateBackendTypeSectionName]; ok {
		globalRemoteStateBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformRemoteStateType, stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.RemoteStateBackendSectionName]; ok {
		globalRemoteStateBackendSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidTerraformRemoteStateSection, stackName)
		}
	}

	// Helmfile section.
	if i, ok := globalHelmfileSection[cfg.CommandSectionName]; ok {
		helmfileCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidHelmfileCommand, stackName)
		}
	}

	if i, ok := globalHelmfileSection[cfg.VarsSectionName]; ok {
		helmfileVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidHelmfileVars, stackName)
		}
	}

	globalAndHelmfileVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, helmfileVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection[cfg.SettingsSectionName]; ok {
		helmfileSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidHelmfileSettings, stackName)
		}
	}

	globalAndHelmfileSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, helmfileSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection[cfg.EnvSectionName]; ok {
		helmfileEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidHelmfileEnv, stackName)
		}
	}

	globalAndHelmfileEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, helmfileEnv})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection[cfg.AuthSectionName]; ok {
		helmfileAuth, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidHelmfileAuth, stackName)
		}
	}

	globalAndHelmfileAuth, err := m.Merge(atmosConfig, []map[string]any{globalAuthSection, helmfileAuth})
	if err != nil {
		return nil, err
	}

	// Packer section.
	if i, ok := globalPackerSection[cfg.CommandSectionName]; ok {
		packerCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidPackerCommand, stackName)
		}
	}

	if i, ok := globalPackerSection[cfg.VarsSectionName]; ok {
		packerVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidPackerVars, stackName)
		}
	}

	globalAndPackerVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, packerVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalPackerSection[cfg.SettingsSectionName]; ok {
		packerSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidPackerSettings, stackName)
		}
	}

	globalAndPackerSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, packerSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalPackerSection[cfg.EnvSectionName]; ok {
		packerEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidPackerEnv, stackName)
		}
	}

	globalAndPackerEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, packerEnv})
	if err != nil {
		return nil, err
	}

	if i, ok := globalPackerSection[cfg.AuthSectionName]; ok {
		packerAuth, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidPackerAuth, stackName)
		}
	}

	globalAndPackerAuth, err := m.Merge(atmosConfig, []map[string]any{globalAuthSection, packerAuth})
	if err != nil {
		return nil, err
	}

	// Convert atmosConfig.Auth struct to map[string]any once before parallel processing.
	// This prevents race conditions when processAuthConfig is called from multiple goroutines.
	// Use JSON marshaling for deep conversion of nested structs to maps.
	var atmosAuthConfig map[string]any
	if atmosConfig.Auth.Providers != nil || atmosConfig.Auth.Identities != nil {
		jsonBytes, err := json.Marshal(atmosConfig.Auth)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to marshal global auth config: %v", errUtils.ErrInvalidAuthConfig, err)
		}
		if err := json.Unmarshal(jsonBytes, &atmosAuthConfig); err != nil {
			return nil, fmt.Errorf("%w: failed to unmarshal global auth config: %v", errUtils.ErrInvalidAuthConfig, err)
		}
	} else {
		atmosAuthConfig = map[string]any{}
	}

	// Process all Terraform components in parallel.
	if componentTypeFilter == "" || componentTypeFilter == cfg.TerraformComponentType {
		if allTerraformComponents, ok := globalComponentsSection[cfg.TerraformComponentType]; ok {
			allTerraformComponentsMap, ok := allTerraformComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidComponentsTerraform, stackName)
			}

			// Build options for each Terraform component.
			buildTerraformOpts := func(component string, componentMap map[string]any) (*ComponentProcessorOptions, error) {
				return &ComponentProcessorOptions{
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
					GlobalAuth:                      globalAndTerraformAuth,
					GlobalCommand:                   terraformCommand,
					AtmosGlobalAuthMap:              atmosAuthConfig,
					TerraformProviders:              terraformProviders,
					GlobalAndTerraformHooks:         globalAndTerraformHooks,
					GlobalBackendType:               globalBackendType,
					GlobalBackendSection:            globalBackendSection,
					GlobalRemoteStateBackendType:    globalRemoteStateBackendType,
					GlobalRemoteStateBackendSection: globalRemoteStateBackendSection,
					AtmosConfig:                     atmosConfig,
				}, nil
			}

			var err error
			terraformComponents, err = processComponentsInParallel(atmosConfig, allTerraformComponentsMap, buildTerraformOpts)
			if err != nil {
				return nil, err
			}
		}
	}

	// Process all Helmfile components in parallel.
	if componentTypeFilter == "" || componentTypeFilter == cfg.HelmfileComponentType {
		if allHelmfileComponents, ok := globalComponentsSection[cfg.HelmfileComponentType]; ok {
			allHelmfileComponentsMap, ok := allHelmfileComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidComponentsHelmfile, stackName)
			}

			// Build options for each Helmfile component.
			buildHelmfileOpts := func(component string, componentMap map[string]any) (*ComponentProcessorOptions, error) {
				return &ComponentProcessorOptions{
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
					GlobalAuth:               globalAndHelmfileAuth,
					GlobalCommand:            helmfileCommand,
					AtmosGlobalAuthMap:       atmosAuthConfig,
					AtmosConfig:              atmosConfig,
				}, nil
			}

			var err error
			helmfileComponents, err = processComponentsInParallel(atmosConfig, allHelmfileComponentsMap, buildHelmfileOpts)
			if err != nil {
				return nil, err
			}
		}
	}

	// Process all Packer components in parallel.
	if componentTypeFilter == "" || componentTypeFilter == cfg.PackerComponentType {
		if allPackerComponents, ok := globalComponentsSection[cfg.PackerComponentType]; ok {
			allPackerComponentsMap, ok := allPackerComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf(errFormatWithFile, errUtils.ErrInvalidComponentsPacker, stackName)
			}

			// Build options for each Packer component.
			buildPackerOpts := func(component string, componentMap map[string]any) (*ComponentProcessorOptions, error) {
				return &ComponentProcessorOptions{
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
					GlobalAuth:               globalAndPackerAuth,
					GlobalCommand:            packerCommand,
					AtmosGlobalAuthMap:       atmosAuthConfig,
					AtmosConfig:              atmosConfig,
				}, nil
			}

			var err error
			packerComponents, err = processComponentsInParallel(atmosConfig, allPackerComponentsMap, buildPackerOpts)
			if err != nil {
				return nil, err
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

// componentProcessResult holds the result of processing a single component in parallel.
type componentProcessResult struct {
	component string
	comp      map[string]any
	err       error
}

// componentWork holds the component name and its processing options.
type componentWork struct {
	component string
	opts      *ComponentProcessorOptions
}

// buildComponentWork pre-builds all component options before parallel processing.
// This ensures each goroutine gets isolated copies of global configurations,
// preventing race conditions in the merge library.
func buildComponentWork(
	componentsMap map[string]any,
	optsBuilder func(component string, componentMap map[string]any) (*ComponentProcessorOptions, error),
) ([]componentWork, error) {
	work := make([]componentWork, 0, len(componentsMap))

	for cmp, v := range componentsMap {
		componentMap, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w for component %s", errUtils.ErrInvalidComponentMapType, cmp)
		}

		opts, err := optsBuilder(cmp, componentMap)
		if err != nil {
			return nil, err
		}

		work = append(work, componentWork{component: cmp, opts: opts})
	}

	return work, nil
}

// processComponentsInParallel processes all components of a specific type in parallel.
// This function parallelizes the expensive component processing work while ensuring
// all errors are properly captured and returned.
//
// IMPORTANT: To avoid race conditions during parallel processing, we must ensure that
// each goroutine gets its own copy of shared data structures. The optsBuilder function
// is responsible for creating independent copies of global configuration maps.
func processComponentsInParallel(
	atmosConfig *schema.AtmosConfiguration,
	componentsMap map[string]any,
	optsBuilder func(component string, componentMap map[string]any) (*ComponentProcessorOptions, error),
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.processComponentsInParallel")()

	if len(componentsMap) == 0 {
		return map[string]any{}, nil
	}

	// Pre-build all component options before starting parallel processing.
	work, err := buildComponentWork(componentsMap, optsBuilder)
	if err != nil {
		return nil, err
	}

	// Create channels for results.
	results := make(chan componentProcessResult, len(work))
	var wg sync.WaitGroup

	// Process each component in parallel.
	for _, w := range work {
		wg.Add(1)
		go func(component string, opts *ComponentProcessorOptions) {
			defer wg.Done()

			// Process the component (expensive: inheritance, backend, settings, etc.).
			result, err := processComponent(opts)
			if err != nil {
				results <- componentProcessResult{component: component, err: err}
				return
			}

			// Merge component configurations.
			comp, err := mergeComponentConfigurations(atmosConfig, opts, result)
			results <- componentProcessResult{component: component, comp: comp, err: err}
		}(w.component, w.opts)
	}

	// Close results channel when all goroutines are done.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results from all goroutines.
	processedComponents := make(map[string]any, len(work))
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		processedComponents[result.component] = result.comp
	}

	return processedComponents, nil
}
