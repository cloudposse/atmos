package exec

import (
	"path/filepath"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/generate"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Section key constants for component configuration.
const (
	sectionKeyComponent  = "component"
	sectionKeyMetadata   = "metadata"
	contextKeyComponent  = "component"
	contextKeyAtmosComp  = "atmos_component"
	contextKeyAtmosStack = "atmos_stack"
)

// ExecuteTerraformGenerateFiles generates files for a single terraform component.
func ExecuteTerraformGenerateFiles(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	dryRun bool,
	clean bool,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateFiles")()

	log.Debug("ExecuteTerraformGenerateFiles called",
		logKeyComponent, component,
		logKeyStack, stack,
		"dryRun", dryRun,
		"clean", clean,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
		StackFromArg:     stack,
		ComponentType:    "terraform",
		CliArgs:          []string{"terraform", "generate", "files"},
	}

	// Process stacks to get component configuration.
	info, err := ProcessStacks(atmosConfig, info, true, true, true, nil, nil)
	if err != nil {
		return err
	}

	// Get generate section from component.
	generateSection := getGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		log.Info("No generate section found for component", logKeyComponent, component, logKeyStack, stack)
		return nil
	}

	// Build template context.
	templateContext := buildTemplateContext(&info)

	// Get component directory.
	componentDir := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)

	// Generate files.
	config := generate.GenerateConfig{
		DryRun: dryRun,
		Clean:  clean,
	}

	_, err = generate.GenerateFiles(generateSection, componentDir, templateContext, config)
	return err
}

// ExecuteTerraformGenerateFilesAll generates files for all terraform components.
func ExecuteTerraformGenerateFilesAll(
	atmosConfig *schema.AtmosConfiguration,
	stacks []string,
	components []string,
	dryRun bool,
	clean bool,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateFilesAll")()

	log.Debug("ExecuteTerraformGenerateFilesAll called",
		"stacks", stacks,
		"components", components,
		"dryRun", dryRun,
		"clean", clean,
	)

	stacksMap, _, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return err
	}

	config := generate.GenerateConfig{
		DryRun: dryRun,
		Clean:  clean,
	}

	for stackFileName, stackSection := range stacksMap {
		if len(stacks) > 0 && !matchesStackFilter(stackFileName, stacks) {
			continue
		}

		processStackForGenerate(atmosConfig, stackFileName, stackSection, components, config)
	}

	return nil
}

// processStackForGenerate processes a single stack for file generation.
func processStackForGenerate(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	stackSection any,
	components []string,
	config generate.GenerateConfig,
) {
	terraformSection := extractTerraformSection(stackSection)
	if terraformSection == nil {
		return
	}

	for componentName, compSection := range terraformSection {
		if len(components) > 0 && !u.SliceContainsString(components, componentName) {
			continue
		}

		processComponentForGenerate(atmosConfig, componentName, stackFileName, compSection, config)
	}
}

// extractTerraformSection extracts the terraform section from a stack.
func extractTerraformSection(stackSection any) map[string]any {
	stackMap, ok := stackSection.(map[string]any)
	if !ok {
		return nil
	}

	componentsSection, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any)
	if !ok {
		return nil
	}

	return terraformSection
}

// processComponentForGenerate processes a single component for file generation.
func processComponentForGenerate(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackFileName string,
	compSection any,
	config generate.GenerateConfig,
) {
	componentSection, ok := compSection.(map[string]any)
	if !ok {
		return
	}

	if isAbstractComponent(componentSection) {
		return
	}

	generateSection := getGenerateSectionFromComponent(componentSection)
	if generateSection == nil {
		return
	}

	templateContext := buildTemplateContextFromSection(componentSection, componentName, stackFileName)
	componentPath := getComponentPath(componentSection, componentName)
	componentDir := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		componentPath,
	)

	log.Info("Processing component",
		logKeyComponent, componentName,
		logKeyStack, stackFileName,
	)

	_, genErr := generate.GenerateFiles(generateSection, componentDir, templateContext, config)
	if genErr != nil {
		log.Error("Error generating files", logKeyComponent, componentName, logKeyStack, stackFileName, "error", genErr)
	}
}

// generateFilesForComponent generates files from the generate section during terraform execution.
// This is called automatically when auto_generate_files is enabled.
func generateFilesForComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "exec.generateFilesForComponent")()

	if !atmosConfig.Components.Terraform.AutoGenerateFiles {
		return nil
	}

	generateSection := getGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		return nil
	}

	log.Debug("Auto-generating files for component",
		logKeyComponent, info.ComponentFromArg,
		logKeyStack, info.Stack,
	)

	templateContext := buildTemplateContext(info)
	config := generate.GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	_, err := generate.GenerateFiles(generateSection, workingDir, templateContext, config)
	return err
}

// getGenerateSectionFromComponent extracts the generate section from a component.
func getGenerateSectionFromComponent(componentSection map[string]any) map[string]any {
	if componentSection == nil {
		return nil
	}

	generateSection, ok := componentSection["generate"].(map[string]any)
	if !ok {
		return nil
	}

	return generateSection
}

// buildTemplateContext builds the template context from ConfigAndStacksInfo.
func buildTemplateContext(info *schema.ConfigAndStacksInfo) map[string]any {
	context := make(map[string]any)

	// Add component info.
	context[contextKeyAtmosComp] = info.ComponentFromArg
	context[contextKeyAtmosStack] = info.Stack
	context["atmos_stack_file"] = info.StackFile
	context[contextKeyComponent] = info.FinalComponent
	context["base_component"] = info.BaseComponent

	// Add context variables.
	context["namespace"] = info.Context.Namespace
	context["tenant"] = info.Context.Tenant
	context["environment"] = info.Context.Environment
	context["stage"] = info.Context.Stage
	context["region"] = info.Context.Region

	// Add workspace.
	context["workspace"] = info.TerraformWorkspace

	// Add sections.
	context["vars"] = info.ComponentVarsSection
	context["settings"] = info.ComponentSettingsSection
	context["env"] = info.ComponentEnvSection
	context["backend"] = info.ComponentBackendSection
	context["backend_type"] = info.ComponentBackendType
	context["providers"] = info.ComponentProvidersSection
	context["metadata"] = info.ComponentMetadataSection

	return context
}

// buildTemplateContextFromSection builds template context from a component section map.
// Used when processing all components without full ProcessStacks.
func buildTemplateContextFromSection(componentSection map[string]any, componentName, stackName string) map[string]any {
	context := make(map[string]any)

	context[contextKeyAtmosComp] = componentName
	context[contextKeyAtmosStack] = stackName
	context[contextKeyComponent] = componentName

	// Extract vars and context variables.
	extractVarsContext(componentSection, context)

	// Extract other sections.
	extractSectionAsMap(componentSection, context, "settings")
	extractSectionAsMap(componentSection, context, "env")
	extractSectionAsMap(componentSection, context, "backend")
	extractSectionAsMap(componentSection, context, "providers")
	extractSectionAsMap(componentSection, context, sectionKeyMetadata)
	extractSectionAsString(componentSection, context, "backend_type")
	extractSectionAsString(componentSection, context, "workspace")

	return context
}

// extractVarsContext extracts the vars section and context variables from it.
func extractVarsContext(componentSection, context map[string]any) {
	vars, ok := componentSection["vars"].(map[string]any)
	if !ok {
		return
	}
	context["vars"] = vars

	// Extract context variables from vars.
	contextVars := []string{"namespace", "tenant", "environment", "stage", "region"}
	for _, key := range contextVars {
		if val, ok := vars[key].(string); ok {
			context[key] = val
		}
	}
}

// extractSectionAsMap extracts a map section from componentSection to context.
func extractSectionAsMap(componentSection, context map[string]any, key string) {
	if val, ok := componentSection[key].(map[string]any); ok {
		context[key] = val
	}
}

// extractSectionAsString extracts a string section from componentSection to context.
func extractSectionAsString(componentSection, context map[string]any, key string) {
	if val, ok := componentSection[key].(string); ok {
		context[key] = val
	}
}

// isAbstractComponent checks if a component is abstract.
func isAbstractComponent(componentSection map[string]any) bool {
	metadata, ok := componentSection[sectionKeyMetadata].(map[string]any)
	if !ok {
		return false
	}

	componentType, ok := metadata["type"].(string)
	return ok && componentType == "abstract"
}

// getComponentPath gets the component path from component section.
func getComponentPath(componentSection map[string]any, componentName string) string {
	metadata, ok := componentSection[sectionKeyMetadata].(map[string]any)
	if ok {
		if component, ok := metadata[sectionKeyComponent].(string); ok {
			return component
		}
	}
	return componentName
}

// matchesStackFilter checks if a stack matches the filter patterns.
func matchesStackFilter(stackName string, filters []string) bool {
	for _, filter := range filters {
		matched, err := filepath.Match(filter, stackName)
		if err == nil && matched {
			return true
		}
		// Also check if the filter matches as a prefix.
		if len(filter) > 0 && filter[len(filter)-1] == '*' {
			prefix := filter[:len(filter)-1]
			if len(stackName) >= len(prefix) && stackName[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}

// GetGenerateFilenamesForComponent returns the list of filenames from the generate section.
// This is used by terraform clean to know which files to delete.
func GetGenerateFilenamesForComponent(componentSection map[string]any) []string {
	defer perf.Track(nil, "exec.GetGenerateFilenamesForComponent")()

	generateSection := getGenerateSectionFromComponent(componentSection)
	if generateSection == nil {
		return nil
	}
	return generate.GetGenerateFilenames(generateSection)
}
