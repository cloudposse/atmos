// Package generate provides functionality to generate files for Terraform components
// from the generate section in Atmos stack configuration.
package generate

//go:generate go run go.uber.org/mock/mockgen@latest -typed -destination=mock_stack_processor_test.go -package=generate github.com/cloudposse/atmos/pkg/terraform/generate StackProcessor

import (
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
	logKeyComponent      = "component"
	logKeyStack          = "stack"
)

// StackProcessor defines the interface for processing stacks.
// This allows for dependency injection and easier testing.
type StackProcessor interface {
	// ProcessStacks processes stacks and returns component configuration.
	ProcessStacks(
		atmosConfig *schema.AtmosConfiguration,
		info schema.ConfigAndStacksInfo,
		checkStack bool,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		authManager auth.AuthManager,
	) (schema.ConfigAndStacksInfo, error)

	// FindStacksMap discovers all stacks in the configuration.
	FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error)
}

// Service provides file generation operations for Terraform components.
type Service struct {
	stackProcessor StackProcessor
}

// NewService creates a new generate service with the given stack processor.
func NewService(processor StackProcessor) *Service {
	defer perf.Track(nil, "generate.NewService")()

	return &Service{
		stackProcessor: processor,
	}
}

// ExecuteForComponent generates files for a single terraform component.
func (s *Service) ExecuteForComponent(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	dryRun bool,
	clean bool,
) error {
	defer perf.Track(atmosConfig, "terraform.generate.ExecuteForComponent")()

	log.Debug("ExecuteForComponent called",
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
	info, err := s.stackProcessor.ProcessStacks(atmosConfig, info, true, true, true, nil, nil)
	if err != nil {
		return err
	}

	// Get generate section from component.
	generateSection := GetGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		log.Info("No generate section found for component", logKeyComponent, component, logKeyStack, stack)
		return nil
	}

	// Build template context.
	templateContext := BuildTemplateContext(&info)

	// Get component directory.
	componentDir := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)

	// Generate files.
	config := GenerateConfig{
		DryRun: dryRun,
		Clean:  clean,
	}

	_, err = GenerateFiles(generateSection, componentDir, templateContext, config)
	return err
}

// ExecuteForAll generates files for all terraform components.
func (s *Service) ExecuteForAll(
	atmosConfig *schema.AtmosConfiguration,
	stacks []string,
	components []string,
	dryRun bool,
	clean bool,
) error {
	defer perf.Track(atmosConfig, "terraform.generate.ExecuteForAll")()

	log.Debug("ExecuteForAll called",
		"stacks", stacks,
		"components", components,
		"dryRun", dryRun,
		"clean", clean,
	)

	stacksMap, _, err := s.stackProcessor.FindStacksMap(atmosConfig, false)
	if err != nil {
		return err
	}

	config := GenerateConfig{
		DryRun: dryRun,
		Clean:  clean,
	}

	for stackFileName, stackSection := range stacksMap {
		if len(stacks) > 0 && !MatchesStackFilter(stackFileName, stacks) {
			continue
		}

		processStackForGenerate(atmosConfig, stackFileName, stackSection, components, config)
	}

	return nil
}

// GenerateFilesForComponent generates files from the generate section during terraform execution.
// This is called automatically when auto_generate_files is enabled.
func (s *Service) GenerateFilesForComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "terraform.generate.GenerateFilesForComponent")()

	if !atmosConfig.Components.Terraform.AutoGenerateFiles {
		return nil
	}

	generateSection := GetGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		return nil
	}

	log.Debug("Auto-generating files for component",
		logKeyComponent, info.ComponentFromArg,
		logKeyStack, info.Stack,
	)

	templateContext := BuildTemplateContext(info)
	config := GenerateConfig{
		DryRun: false,
		Clean:  false,
	}

	_, err := GenerateFiles(generateSection, workingDir, templateContext, config)
	return err
}

// processStackForGenerate processes a single stack for file generation.
func processStackForGenerate(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	stackSection any,
	components []string,
	config GenerateConfig,
) {
	terraformSection := ExtractTerraformSection(stackSection)
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

// processComponentForGenerate processes a single component for file generation.
func processComponentForGenerate(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackFileName string,
	compSection any,
	config GenerateConfig,
) {
	componentSection, ok := compSection.(map[string]any)
	if !ok {
		return
	}

	if IsAbstractComponent(componentSection) {
		return
	}

	generateSection := GetGenerateSectionFromComponent(componentSection)
	if generateSection == nil {
		return
	}

	templateContext := BuildTemplateContextFromSection(componentSection, componentName, stackFileName)
	componentPath := GetComponentPath(componentSection, componentName)
	componentDir := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		componentPath,
	)

	log.Info("Processing component",
		logKeyComponent, componentName,
		logKeyStack, stackFileName,
	)

	_, genErr := GenerateFiles(generateSection, componentDir, templateContext, config)
	if genErr != nil {
		// Log error but continue processing other components to maximize successful generations.
		log.Error("Error generating files", logKeyComponent, componentName, logKeyStack, stackFileName, "error", genErr)
	}
}

// GetGenerateSectionFromComponent extracts the generate section from a component.
func GetGenerateSectionFromComponent(componentSection map[string]any) map[string]any {
	defer perf.Track(nil, "generate.GetGenerateSectionFromComponent")()

	if componentSection == nil {
		return nil
	}

	generateSection, ok := componentSection["generate"].(map[string]any)
	if !ok {
		return nil
	}

	return generateSection
}

// GetFilenamesForComponent returns the list of filenames from the generate section.
// This is used by terraform clean to know which files to delete.
func GetFilenamesForComponent(componentSection map[string]any) []string {
	defer perf.Track(nil, "terraform.generate.GetFilenamesForComponent")()

	generateSection := GetGenerateSectionFromComponent(componentSection)
	if generateSection == nil {
		return nil
	}
	return GetGenerateFilenames(generateSection)
}

// BuildTemplateContext builds the template context from ConfigAndStacksInfo.
func BuildTemplateContext(info *schema.ConfigAndStacksInfo) map[string]any {
	defer perf.Track(nil, "generate.BuildTemplateContext")()

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

// BuildTemplateContextFromSection builds template context from a component section map.
// Used when processing all components without full ProcessStacks.
func BuildTemplateContextFromSection(componentSection map[string]any, componentName, stackName string) map[string]any {
	defer perf.Track(nil, "generate.BuildTemplateContextFromSection")()

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

// IsAbstractComponent checks if a component is abstract.
func IsAbstractComponent(componentSection map[string]any) bool {
	defer perf.Track(nil, "generate.IsAbstractComponent")()

	metadata, ok := componentSection[sectionKeyMetadata].(map[string]any)
	if !ok {
		return false
	}

	componentType, ok := metadata["type"].(string)
	return ok && componentType == "abstract"
}

// GetComponentPath gets the component path from component section.
func GetComponentPath(componentSection map[string]any, componentName string) string {
	defer perf.Track(nil, "generate.GetComponentPath")()

	metadata, ok := componentSection[sectionKeyMetadata].(map[string]any)
	if ok {
		if component, ok := metadata[sectionKeyComponent].(string); ok {
			return component
		}
	}
	return componentName
}

// MatchesStackFilter checks if a stack matches the filter patterns.
// It checks both the full stack file path and the basename, allowing users to filter by
// either "deploy/dev" (full path) or "dev" (basename).
// Uses filepath.Match which supports glob patterns.
func MatchesStackFilter(stackName string, filters []string) bool {
	defer perf.Track(nil, "generate.MatchesStackFilter")()

	// Get the basename for user-friendly matching (e.g., "deploy/dev" -> "dev").
	baseName := filepath.Base(stackName)

	for _, filter := range filters {
		// Check against the full stack file path.
		if matched, err := filepath.Match(filter, stackName); err == nil && matched {
			return true
		}
		// Check against the basename for user-friendly matching.
		if baseName != stackName {
			if matched, err := filepath.Match(filter, baseName); err == nil && matched {
				return true
			}
		}
	}
	return false
}

// ExtractTerraformSection extracts the terraform section from a stack.
func ExtractTerraformSection(stackSection any) map[string]any {
	defer perf.Track(nil, "generate.ExtractTerraformSection")()

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
