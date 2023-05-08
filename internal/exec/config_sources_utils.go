package exec

import (
	"fmt"
	"reflect"

	"github.com/cloudposse/atmos/pkg/schema"
)

// processConfigSources processes the sources (files) for all sections for a component in a stack
func processConfigSources(
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
) (
	map[string]map[string]any,
	error,
) {
	result := map[string]map[string]any{}

	// `vars` section
	vars := map[string]any{}
	result["vars"] = vars

	for varKey, varVal := range configAndStacksInfo.ComponentVarsSection {
		variable := varKey.(string)
		varObj := map[string]any{}
		varObj["name"] = variable
		varObj["final_value"] = varVal
		varObj["stack_dependencies"] = processVariableInStacks(configAndStacksInfo, rawStackConfigs, variable)
		vars[variable] = varObj
	}

	// `env` section
	env := map[string]any{}
	result["env"] = env

	// `settings` section
	settings := map[string]any{}
	result["settings"] = settings

	return result, nil
}

func processVariableInStacks(
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	variable string,
) []map[string]any {

	result := []map[string]any{}

	// Process the variable for the component in the stack
	// Because we want to show the variable dependencies from higher to lower priority,
	// the order of processing is the reverse order from what Atmos follows when calculating the final variables in the `vars` section
	processComponentVariableInStack(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		variable,
	)

	processComponentVariableInStackImports(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		variable,
	)

	// Process the variable for all the base components in the stack from the inheritance chain
	for _, baseComponent := range configAndStacksInfo.ComponentInheritanceChain {
		processComponentVariableInStack(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			variable,
		)

		processComponentVariableInStackImports(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			variable,
		)
	}

	processComponentTypeVariableInStack(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		variable,
	)

	processGlobalVariableInStack(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		rawStackConfigs,
		variable,
	)

	for _, baseComponent := range configAndStacksInfo.ComponentInheritanceChain {
		processComponentTypeVariableInStack(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			variable,
		)

		processGlobalVariableInStack(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			rawStackConfigs,
			variable,
		)
	}

	processComponentTypeVariableInStackImports(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		configAndStacksInfo,
		rawStackConfigs,
		variable,
	)

	processGlobalVariableInStackImports(
		configAndStacksInfo.ComponentFromArg,
		configAndStacksInfo.StackFile,
		&result,
		rawStackConfigs,
		variable,
	)

	for _, baseComponent := range configAndStacksInfo.ComponentInheritanceChain {
		processComponentTypeVariableInStackImports(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			configAndStacksInfo,
			rawStackConfigs,
			variable,
		)

		processGlobalVariableInStackImports(
			baseComponent,
			configAndStacksInfo.StackFile,
			&result,
			rawStackConfigs,
			variable,
		)
	}

	return result
}

// https://medium.com/swlh/golang-tips-why-pointers-to-slices-are-useful-and-how-ignoring-them-can-lead-to-tricky-bugs-cac90f72e77b
func processComponentVariableInStack(
	component string,
	stackFile string,
	result *[]map[string]any,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	variable string,
) *[]map[string]any {

	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStack, ok := rawStackConfig["stack"]
	if !ok {
		return result
	}

	rawStackMap, ok := rawStack.(map[any]any)
	if !ok {
		return result
	}

	rawStackComponentsSection, ok := rawStackMap["components"]
	if !ok {
		return result
	}

	rawStackComponentsSectionMap, ok := rawStackComponentsSection.(map[any]any)
	if !ok {
		return result
	}

	rawStackComponentTypeSection, ok := rawStackComponentsSectionMap[configAndStacksInfo.ComponentType]
	if !ok {
		return result
	}

	rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[any]any)
	if !ok {
		return result
	}

	rawStackComponentSection, ok := rawStackComponentTypeSectionMap[component]
	if !ok {
		return result
	}

	rawStackComponentSectionMap, ok := rawStackComponentSection.(map[any]any)
	if !ok {
		return result
	}

	rawStackVars, ok := rawStackComponentSectionMap["vars"]
	if !ok {
		return result
	}

	rawStackVarsMap, ok := rawStackVars.(map[any]any)
	if !ok {
		return result
	}

	rawStackVarVal, ok := rawStackVarsMap[variable]
	if !ok {
		return result
	}

	val := map[string]any{
		"stack_file":         stackFile,
		"stack_file_section": fmt.Sprintf("components.%s.vars", configAndStacksInfo.ComponentType),
		"variable_value":     rawStackVarVal,
		"dependency_type":    "inline",
	}

	appendVariableDescriptor(result, val)

	return result
}

func processComponentTypeVariableInStack(
	component string,
	stackFile string,
	result *[]map[string]any,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	variable string,
) *[]map[string]any {

	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStack, ok := rawStackConfig["stack"]
	if !ok {
		return result
	}

	rawStackMap, ok := rawStack.(map[any]any)
	if !ok {
		return result
	}

	rawStackComponentTypeSection, ok := rawStackMap[configAndStacksInfo.ComponentType]
	if !ok {
		return result
	}

	rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[any]any)
	if !ok {
		return result
	}

	rawStackVars, ok := rawStackComponentTypeSectionMap["vars"]
	if !ok {
		return result
	}

	rawStackVarsMap, ok := rawStackVars.(map[any]any)
	if !ok {
		return result
	}

	rawStackVarVal, ok := rawStackVarsMap[variable]
	if !ok {
		return result
	}

	val := map[string]any{
		"stack_file":         stackFile,
		"stack_file_section": fmt.Sprintf("%s.vars", configAndStacksInfo.ComponentType),
		"variable_value":     rawStackVarVal,
		"dependency_type":    "inline",
	}

	appendVariableDescriptor(result, val)

	return result
}

func processGlobalVariableInStack(
	component string,
	stackFile string,
	result *[]map[string]any,
	rawStackConfigs map[string]map[string]any,
	variable string,
) *[]map[string]any {

	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStack, ok := rawStackConfig["stack"]
	if !ok {
		return result
	}

	rawStackMap, ok := rawStack.(map[any]any)
	if !ok {
		return result
	}

	rawStackVars, ok := rawStackMap["vars"]
	if !ok {
		return result
	}

	rawStackVarsMap, ok := rawStackVars.(map[any]any)
	if !ok {
		return result
	}

	rawStackVarVal, ok := rawStackVarsMap[variable]
	if !ok {
		return result
	}

	val := map[string]any{
		"stack_file":         stackFile,
		"stack_file_section": "vars",
		"variable_value":     rawStackVarVal,
		"dependency_type":    "inline",
	}

	appendVariableDescriptor(result, val)

	return result
}

func processComponentVariableInStackImports(
	component string,
	stackFile string,
	result *[]map[string]any,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	variable string,
) *[]map[string]any {

	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStackImports, ok := rawStackConfig["imports"]
	if !ok {
		return result
	}

	rawStackImportsMap, ok := rawStackImports.(map[string]map[any]any)
	if !ok {
		return result
	}

	for impKey, impVal := range rawStackImportsMap {
		rawStackComponentsSection, ok := impVal["components"]
		if !ok {
			continue
		}

		rawStackComponentsSectionMap, ok := rawStackComponentsSection.(map[any]any)
		if !ok {
			continue
		}

		rawStackComponentTypeSection, ok := rawStackComponentsSectionMap[configAndStacksInfo.ComponentType]
		if !ok {
			continue
		}

		rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[any]any)
		if !ok {
			continue
		}

		rawStackComponentSection, ok := rawStackComponentTypeSectionMap[component]
		if !ok {
			continue
		}

		rawStackComponentSectionMap, ok := rawStackComponentSection.(map[any]any)
		if !ok {
			continue
		}

		rawStackVars, ok := rawStackComponentSectionMap["vars"]
		if !ok {
			continue
		}

		rawStackVarsMap, ok := rawStackVars.(map[any]any)
		if !ok {
			continue
		}

		rawStackVarVal, ok := rawStackVarsMap[variable]
		if !ok {
			continue
		}

		val := map[string]any{
			"stack_file":         impKey,
			"stack_file_section": fmt.Sprintf("components.%s.vars", configAndStacksInfo.ComponentType),
			"variable_value":     rawStackVarVal,
			"dependency_type":    "import",
		}

		appendVariableDescriptor(result, val)
	}

	return result
}

func processComponentTypeVariableInStackImports(
	component string,
	stackFile string,
	result *[]map[string]any,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	rawStackConfigs map[string]map[string]any,
	variable string,
) *[]map[string]any {

	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStackImports, ok := rawStackConfig["imports"]
	if !ok {
		return result
	}

	rawStackImportsMap, ok := rawStackImports.(map[string]map[any]any)
	if !ok {
		return result
	}

	for impKey, impVal := range rawStackImportsMap {
		rawStackComponentTypeSection, ok := impVal[configAndStacksInfo.ComponentType]
		if !ok {
			continue
		}

		rawStackComponentTypeSectionMap, ok := rawStackComponentTypeSection.(map[any]any)
		if !ok {
			continue
		}

		rawStackVars, ok := rawStackComponentTypeSectionMap["vars"]
		if !ok {
			continue
		}

		rawStackVarsMap, ok := rawStackVars.(map[any]any)
		if !ok {
			continue
		}

		rawStackVarVal, ok := rawStackVarsMap[variable]
		if !ok {
			continue
		}

		val := map[string]any{
			"stack_file":         impKey,
			"stack_file_section": fmt.Sprintf("%s.vars", configAndStacksInfo.ComponentType),
			"variable_value":     rawStackVarVal,
			"dependency_type":    "import",
		}

		appendVariableDescriptor(result, val)
	}

	return result
}

func processGlobalVariableInStackImports(
	component string,
	stackFile string,
	result *[]map[string]any,
	rawStackConfigs map[string]map[string]any,
	variable string,
) *[]map[string]any {

	rawStackConfig, ok := rawStackConfigs[stackFile]
	if !ok {
		return result
	}

	rawStackImports, ok := rawStackConfig["imports"]
	if !ok {
		return result
	}

	rawStackImportsMap, ok := rawStackImports.(map[string]map[any]any)
	if !ok {
		return result
	}

	for impKey, impVal := range rawStackImportsMap {
		rawStackVars, ok := impVal["vars"]
		if !ok {
			continue
		}

		rawStackVarsMap, ok := rawStackVars.(map[any]any)
		if !ok {
			continue
		}

		rawStackVarVal, ok := rawStackVarsMap[variable]
		if !ok {
			continue
		}

		val := map[string]any{
			"stack_file":         impKey,
			"stack_file_section": "vars",
			"variable_value":     rawStackVarVal,
			"dependency_type":    "import",
		}

		appendVariableDescriptor(result, val)
	}

	return result
}

func appendVariableDescriptor(result *[]map[string]any, descriptor map[string]any) {
	for _, item := range *result {
		if reflect.DeepEqual(item, descriptor) {
			return
		}
	}
	*result = append(*result, descriptor)
}
