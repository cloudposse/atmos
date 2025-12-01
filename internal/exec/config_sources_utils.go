package exec

import (
	"fmt"
	"reflect"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessConfigSources processes the sources (files) for all sections for a component in a stack.
func ProcessConfigSources(
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
) (schema.ConfigSources, error) {
	defer perf.Track(nil, "exec.ProcessConfigSources")()

	result := schema.ConfigSources{}

	// `vars` section
	vars := map[string]schema.ConfigSourcesItem{}
	result["vars"] = vars

	for k, v := range configAndStacksInfo.ComponentVarsSection {
		name := k
		obj := schema.ConfigSourcesItem{}
		obj.Name = name
		obj.FinalValue = v
		obj.StackDependencies = processSectionValueInStacks(configAndStacksInfo, rawStackConfigs, "vars", "", name)
		vars[name] = obj
	}

	// `env` section
	env := map[string]schema.ConfigSourcesItem{}
	result["env"] = env

	for k, v := range configAndStacksInfo.ComponentEnvSection {
		name := k
		obj := schema.ConfigSourcesItem{}
		obj.Name = name
		obj.FinalValue = v
		obj.StackDependencies = processSectionValueInStacks(configAndStacksInfo, rawStackConfigs, "env", "", name)
		env[name] = obj
	}

	// `settings` section
	settings := map[string]schema.ConfigSourcesItem{}
	result["settings"] = settings

	for k, v := range configAndStacksInfo.ComponentSettingsSection {
		name := k
		obj := schema.ConfigSourcesItem{}
		obj.Name = name
		obj.FinalValue = v
		obj.StackDependencies = processSectionValueInStacks(configAndStacksInfo, rawStackConfigs, "settings", "", name)
		settings[name] = obj
	}

	// `backend` section
	backend := map[string]schema.ConfigSourcesItem{}
	result["backend"] = backend

	for k, v := range configAndStacksInfo.ComponentBackendSection {
		name := k
		obj := schema.ConfigSourcesItem{}
		obj.Name = name
		obj.FinalValue = v
		obj.StackDependencies = processSectionValueInStacks(configAndStacksInfo, rawStackConfigs, "backend", configAndStacksInfo.ComponentBackendType, name)
		backend[name] = obj
	}

	return result, nil
}

func processSectionValueInStacks(
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	section string,
	subsection string,
	value string,
) schema.ConfigSourcesStackDependencies {
	result := schema.ConfigSourcesStackDependencies{}

	// Process the value for the component in the stack
	// Because we want to show the value dependencies from higher to lower priority,
	// the order of processing is the reverse order from what Atmos follows when calculating the final variables in the `vars` section
	processComponentSectionValueInStack(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		section,
		subsection,
		value,
	)

	processComponentSectionValueInStackImports(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		section,
		subsection,
		value,
	)

	// Process the value for all the base components in the stack from the inheritance chain
	for _, baseComponent := range configAndStacksInfo.ComponentInheritanceChain {
		processComponentSectionValueInStack(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			section,
			subsection,
			value,
		)

		processComponentSectionValueInStackImports(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			section,
			subsection,
			value,
		)
	}

	processComponentTypeSectionValueInStack(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		section,
		subsection,
		value,
	)

	processGlobalSectionValueInStack(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		rawStackConfigs,
		section,
		subsection,
		value,
	)

	for _, baseComponent := range configAndStacksInfo.ComponentInheritanceChain {
		processComponentTypeSectionValueInStack(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			section,
			subsection,
			value,
		)

		processGlobalSectionValueInStack(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			rawStackConfigs,
			section,
			subsection,
			value,
		)
	}

	processComponentTypeSectionValueInStackImports(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		section,
		subsection,
		value,
	)

	processGlobalSectionValueInStackImports(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		rawStackConfigs,
		section,
		subsection,
		value,
	)

	for _, baseComponent := range configAndStacksInfo.ComponentInheritanceChain {
		processComponentTypeSectionValueInStackImports(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			section,
			subsection,
			value,
		)

		processGlobalSectionValueInStackImports(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			rawStackConfigs,
			section,
			subsection,
			value,
		)
	}

	return result
}

// https://medium.com/swlh/golang-tips-why-pointers-to-slices-are-useful-and-how-ignoring-them-can-lead-to-tricky-bugs-cac90f72e77b
func processComponentSectionValueInStack(
	component string,
	stackFile string,
	result *schema.ConfigSourcesStackDependencies,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	section string,
	subsection string,
	value string,
) *schema.ConfigSourcesStackDependencies {
	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStack, ok := rawStackConfig["stack"]
	if !ok {
		return result
	}

	rawStackMap, ok := rawStack.(map[string]any)
	if !ok {
		return result
	}

	rawStackComponentsSection, ok := rawStackMap["components"]
	if !ok {
		return result
	}

	rawStackComponentsSectionMap, ok := rawStackComponentsSection.(map[string]any)
	if !ok {
		return result
	}

	rawStackComponentTypeSection, ok := rawStackComponentsSectionMap[configAndStacksInfo.ComponentType]
	if !ok {
		return result
	}

	rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[string]any)
	if !ok {
		return result
	}

	rawStackComponentSection, ok := rawStackComponentTypeSectionMap[component]
	if !ok {
		return result
	}

	rawStackComponentSectionMap, ok := rawStackComponentSection.(map[string]any)
	if !ok {
		return result
	}

	rawStackVars, ok := rawStackComponentSectionMap[section]
	if !ok {
		return result
	}

	rawStackVarsMap, ok := rawStackVars.(map[string]any)
	if !ok {
		return result
	}

	if subsection != "" {
		rawStackVarsMapSubsection, ok := rawStackVarsMap[subsection].(map[string]any)
		if !ok {
			return result
		}
		rawStackVarsMap = rawStackVarsMapSubsection
	}

	rawStackVarVal, ok := rawStackVarsMap[value]
	if !ok {
		return result
	}

	stackFileSection := fmt.Sprintf("components.%s.%s", configAndStacksInfo.ComponentType, section)
	if subsection != "" {
		stackFileSection = fmt.Sprintf("components.%s.%s.%s", configAndStacksInfo.ComponentType, section, subsection)
	}

	val := schema.ConfigSourcesStackDependency{
		StackFile:        stackFile,
		StackFileSection: stackFileSection,
		VariableValue:    rawStackVarVal,
		DependencyType:   "inline",
	}

	appendSectionValue(result, val)

	return result
}

func processComponentTypeSectionValueInStack(
	component string,
	stackFile string,
	result *schema.ConfigSourcesStackDependencies,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	section string,
	subsection string,
	value string,
) *schema.ConfigSourcesStackDependencies {
	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStack, ok := rawStackConfig["stack"]
	if !ok {
		return result
	}

	rawStackMap, ok := rawStack.(map[string]any)
	if !ok {
		return result
	}

	rawStackComponentTypeSection, ok := rawStackMap[configAndStacksInfo.ComponentType]
	if !ok {
		return result
	}

	rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[string]any)
	if !ok {
		return result
	}

	rawStackVars, ok := rawStackComponentTypeSectionMap[section]
	if !ok {
		return result
	}

	rawStackVarsMap, ok := rawStackVars.(map[string]any)
	if !ok {
		return result
	}

	if subsection != "" {
		rawStackVarsMapSubsection, ok := rawStackVarsMap[subsection].(map[string]any)
		if !ok {
			return result
		}
		rawStackVarsMap = rawStackVarsMapSubsection
	}

	rawStackVarVal, ok := rawStackVarsMap[value]
	if !ok {
		return result
	}

	stackFileSection := fmt.Sprintf("%s.%s", configAndStacksInfo.ComponentType, section)
	if subsection != "" {
		stackFileSection = fmt.Sprintf("%s.%s.%s", configAndStacksInfo.ComponentType, section, subsection)
	}

	val := schema.ConfigSourcesStackDependency{
		StackFile:        stackFile,
		StackFileSection: stackFileSection,
		VariableValue:    rawStackVarVal,
		DependencyType:   "inline",
	}

	appendSectionValue(result, val)

	return result
}

func processGlobalSectionValueInStack(
	component string,
	stackFile string,
	result *schema.ConfigSourcesStackDependencies,
	rawStackConfigs map[string]map[string]any,
	section string,
	subsection string,
	value string,
) *schema.ConfigSourcesStackDependencies {
	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStack, ok := rawStackConfig["stack"]
	if !ok {
		return result
	}

	rawStackMap, ok := rawStack.(map[string]any)
	if !ok {
		return result
	}

	rawStackVars, ok := rawStackMap[section]
	if !ok {
		return result
	}

	rawStackVarsMap, ok := rawStackVars.(map[string]any)
	if !ok {
		return result
	}

	if subsection != "" {
		rawStackVarsMapSubsection, ok := rawStackVarsMap[subsection].(map[string]any)
		if !ok {
			return result
		}
		rawStackVarsMap = rawStackVarsMapSubsection
	}

	rawStackVarVal, ok := rawStackVarsMap[value]
	if !ok {
		return result
	}

	stackFileSection := section
	if subsection != "" {
		stackFileSection = fmt.Sprintf("%s.%s", section, subsection)
	}

	val := schema.ConfigSourcesStackDependency{
		StackFile:        stackFile,
		StackFileSection: stackFileSection,
		VariableValue:    rawStackVarVal,
		DependencyType:   "inline",
	}

	appendSectionValue(result, val)

	return result
}

func processComponentSectionValueInStackImports(
	component string,
	stackFile string,
	result *schema.ConfigSourcesStackDependencies,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	section string,
	subsection string,
	value string,
) *schema.ConfigSourcesStackDependencies {
	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStackImports, ok := rawStackConfig["imports"]
	if !ok {
		return result
	}

	rawStackImportsMap, ok := rawStackImports.(map[string]map[string]any)
	if !ok {
		return result
	}

	rawStackImportFiles, ok := rawStackConfig["import_files"]
	if !ok {
		return result
	}

	rawStackImportFilesList, ok := rawStackImportFiles.([]string)
	if !ok {
		return result
	}

	for _, impKey := range rawStackImportFilesList {
		impVal := rawStackImportsMap[impKey]

		rawStackComponentsSection, ok := impVal["components"]
		if !ok {
			continue
		}

		rawStackComponentsSectionMap, ok := rawStackComponentsSection.(map[string]any)
		if !ok {
			continue
		}

		rawStackComponentTypeSection, ok := rawStackComponentsSectionMap[configAndStacksInfo.ComponentType]
		if !ok {
			continue
		}

		rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[string]any)
		if !ok {
			continue
		}

		rawStackComponentSection, ok := rawStackComponentTypeSectionMap[component]
		if !ok {
			continue
		}

		rawStackComponentSectionMap, ok := rawStackComponentSection.(map[string]any)
		if !ok {
			continue
		}

		rawStackVars, ok := rawStackComponentSectionMap[section]
		if !ok {
			continue
		}

		rawStackVarsMap, ok := rawStackVars.(map[string]any)
		if !ok {
			continue
		}

		if subsection != "" {
			rawStackVarsMapSubsection, ok := rawStackVarsMap[subsection].(map[string]any)
			if !ok {
				return result
			}
			rawStackVarsMap = rawStackVarsMapSubsection
		}

		rawStackVarVal, ok := rawStackVarsMap[value]
		if !ok {
			continue
		}

		stackFileSection := fmt.Sprintf("components.%s.%s", configAndStacksInfo.ComponentType, section)
		if subsection != "" {
			stackFileSection = fmt.Sprintf("components.%s.%s.%s", configAndStacksInfo.ComponentType, section, subsection)
		}

		val := schema.ConfigSourcesStackDependency{
			StackFile:        impKey,
			StackFileSection: stackFileSection,
			VariableValue:    rawStackVarVal,
			DependencyType:   "import",
		}

		appendSectionValue(result, val)
	}

	return result
}

func processComponentTypeSectionValueInStackImports(
	component string,
	stackFile string,
	result *schema.ConfigSourcesStackDependencies,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	section string,
	subsection string,
	value string,
) *schema.ConfigSourcesStackDependencies {
	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStackImports, ok := rawStackConfig["imports"]
	if !ok {
		return result
	}

	rawStackImportsMap, ok := rawStackImports.(map[string]map[string]any)
	if !ok {
		return result
	}

	rawStackImportFiles, ok := rawStackConfig["import_files"]
	if !ok {
		return result
	}

	rawStackImportFilesList, ok := rawStackImportFiles.([]string)
	if !ok {
		return result
	}

	for _, impKey := range rawStackImportFilesList {
		impVal := rawStackImportsMap[impKey]

		rawStackComponentTypeSection, ok := impVal[configAndStacksInfo.ComponentType]
		if !ok {
			continue
		}

		rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[string]any)
		if !ok {
			continue
		}

		rawStackVars, ok := rawStackComponentTypeSectionMap[section]
		if !ok {
			continue
		}

		rawStackVarsMap, ok := rawStackVars.(map[string]any)
		if !ok {
			continue
		}

		if subsection != "" {
			rawStackVarsMapSubsection, ok := rawStackVarsMap[subsection].(map[string]any)
			if !ok {
				return result
			}
			rawStackVarsMap = rawStackVarsMapSubsection
		}

		rawStackVarVal, ok := rawStackVarsMap[value]
		if !ok {
			continue
		}

		stackFileSection := fmt.Sprintf("%s.%s", configAndStacksInfo.ComponentType, section)
		if subsection != "" {
			stackFileSection = fmt.Sprintf("%s.%s.%s", configAndStacksInfo.ComponentType, section, subsection)
		}

		val := schema.ConfigSourcesStackDependency{
			StackFile:        impKey,
			StackFileSection: stackFileSection,
			VariableValue:    rawStackVarVal,
			DependencyType:   "import",
		}

		appendSectionValue(result, val)
	}

	return result
}

func processGlobalSectionValueInStackImports(
	component string,
	stackFile string,
	result *schema.ConfigSourcesStackDependencies,
	rawStackConfigs map[string]map[string]any,
	section string,
	subsection string,
	value string,
) *schema.ConfigSourcesStackDependencies {
	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStackImports, ok := rawStackConfig["imports"]
	if !ok {
		return result
	}

	rawStackImportsMap, ok := rawStackImports.(map[string]map[string]any)
	if !ok {
		return result
	}

	rawStackImportFiles, ok := rawStackConfig["import_files"]
	if !ok {
		return result
	}

	rawStackImportFilesList, ok := rawStackImportFiles.([]string)
	if !ok {
		return result
	}

	for _, impKey := range rawStackImportFilesList {
		impVal := rawStackImportsMap[impKey]

		rawStackVars, ok := impVal[section]
		if !ok {
			continue
		}

		rawStackVarsMap, ok := rawStackVars.(map[string]any)
		if !ok {
			continue
		}

		if subsection != "" {
			rawStackVarsMapSubsection, ok := rawStackVarsMap[subsection].(map[string]any)
			if !ok {
				return result
			}
			rawStackVarsMap = rawStackVarsMapSubsection
		}

		rawStackVarVal, ok := rawStackVarsMap[value]
		if !ok {
			continue
		}

		stackFileSection := section
		if subsection != "" {
			stackFileSection = fmt.Sprintf("%s.%s", section, subsection)
		}

		val := schema.ConfigSourcesStackDependency{
			StackFile:        impKey,
			StackFileSection: stackFileSection,
			VariableValue:    rawStackVarVal,
			DependencyType:   "import",
		}

		appendSectionValue(result, val)
	}

	return result
}

func appendSectionValue(result *schema.ConfigSourcesStackDependencies, value schema.ConfigSourcesStackDependency) {
	for _, item := range *result {
		if reflect.DeepEqual(item, value) {
			return
		}
	}
	*result = append(*result, value)
}
