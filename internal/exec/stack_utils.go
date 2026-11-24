package exec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// BuildTerraformWorkspace builds Terraform workspace.
func BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
	return NamingResolver().BuildTerraformWorkspace(atmosConfig, &configAndStacksInfo)
}

// ProcessComponentMetadata processes component metadata and returns a base component (if any) and whether
// the component is real or abstract and whether the component is disabled or not and whether the component is locked.
func ProcessComponentMetadata(
	componentName string,
	componentSection map[string]any,
) (map[string]any, string, bool, bool, bool) {
	result := component.ProcessMetadata(componentName, componentSection)
	return result.Fields, result.BaseComponent, result.Abstract, result.Enabled, result.Locked
}

// BuildDependentStackNameFromDependsOnLegacy builds the dependent stack name from "settings.spacelift.depends_on" config.
func BuildDependentStackNameFromDependsOnLegacy(
	dependsOn string,
	allStackNames []string,
	currentStackName string,
	componentNamesInCurrentStack []string,
	currentComponentName string,
) (string, error) {
	defer perf.Track(nil, "exec.BuildDependentStackNameFromDependsOnLegacy")()

	dep := strings.Replace(dependsOn, "/", "-", -1)

	if slices.Contains(allStackNames, dep) {
		return dep, nil
	}

	if slices.Contains(componentNamesInCurrentStack, dep) {
		return fmt.Sprintf("%s-%s", currentStackName, dep), nil
	}

	return "", fmt.Errorf(
		"%w: the component '%[2]s' in the stack '%[3]s' specifies 'depends_on' dependency '%[4]s', "+
			"but '%[4]s' is not a stack and not a component in the '%[3]s' stack",
		errUtils.ErrInvalidDependsOn,
		currentComponentName,
		currentStackName,
		dependsOn,
	)
}

// BuildDependentStackNameFromDependsOn builds the dependent stack name from "settings.depends_on" config.
func BuildDependentStackNameFromDependsOn(
	currentComponentName string,
	currentStackName string,
	dependsOnComponentName string,
	dependsOnStackName string,
	allStackNames []string,
) (string, error) {
	defer perf.Track(nil, "exec.BuildDependentStackNameFromDependsOn")()

	dep := strings.Replace(fmt.Sprintf("%s-%s", dependsOnStackName, dependsOnComponentName), "/", "-", -1)

	if slices.Contains(allStackNames, dep) {
		return dep, nil
	}

	return "", fmt.Errorf(
		"%w: the component '%s' in the stack '%s' specifies 'settings.depends_on' dependency "+
			"on the component '%s' in the stack '%s', but '%s' is not defined in the '%s' stack, or the component and stack names are not correct",
		errUtils.ErrInvalidSettingsDependsOn,
		currentComponentName,
		currentStackName,
		dependsOnComponentName,
		dependsOnStackName,
		dependsOnComponentName,
		dependsOnStackName,
	)
}

// BuildDependentStackNamesFromDependenciesComponents builds the Spacelift stack names that a component
// depends on from its `dependencies.components` config, which supersedes the deprecated `settings.depends_on`.
//
// The returned bool reports whether `dependencies.components` declares at least one Terraform component
// dependency. Callers use it to give `dependencies.components` precedence over `settings.depends_on`, the same
// rule `atmos describe dependents` applies, so both views of the dependency graph agree.
//
// Each entry's `stack` is a logical stack name (the same value Spacelift naming derives as the context prefix),
// and may be a Go template, which is rendered with the component's own sections (`.vars`, `.settings`, `.env`,
// `.metadata`) when templates are enabled. An omitted `stack` means the declaring component's own stack.
// Entries that are not Terraform dependencies (other kinds, files, folders) have no Spacelift stack and are skipped.
// A target must be a component that generates a Spacelift stack (see BuildSpaceliftGeneratedStackIndex), and its
// dependency name is that stack's actual name. A target that does not resolve is an error unless the entry sets
// `required: false`, in which case it is skipped.
func BuildDependentStackNamesFromDependenciesComponents(
	atmosConfig *schema.AtmosConfiguration,
	componentMap map[string]any,
	currentComponentName string,
	currentStackName string,
	generatedStacks map[SpaceliftStackKey]string,
) ([]string, bool, error) {
	defer perf.Track(atmosConfig, "exec.BuildDependentStackNamesFromDependenciesComponents")()

	deps, err := parseTerraformDependenciesComponents(atmosConfig, componentMap)
	if err != nil || len(deps) == 0 {
		return nil, false, err
	}

	templateData := map[string]any{
		cfg.VarsSectionName:     componentMap[cfg.VarsSectionName],
		cfg.SettingsSectionName: componentMap[cfg.SettingsSectionName],
		cfg.EnvSectionName:      componentMap[cfg.EnvSectionName],
		cfg.MetadataSectionName: componentMap[cfg.MetadataSectionName],
		"atmos_component":       currentComponentName,
		"atmos_stack":           currentStackName,
	}

	names := make([]string, 0, len(deps))
	for i := range deps {
		dep := &deps[i]

		dependsOnStackName, err := resolveDependenciesComponentsStack(atmosConfig, dep, currentComponentName, currentStackName, templateData)
		if err != nil {
			return nil, false, err
		}

		// Use the name of the Spacelift stack the target is generated with, which honors `stack_name` and
		// `stack_name_pattern` overrides. Targets that generate no Spacelift stack (abstract, disabled, or without
		// `workspace_enabled`) are not in the index.
		name, found := generatedStacks[NewSpaceliftStackKey(dependsOnStackName, dep.Component)]
		switch {
		case found:
			names = append(names, name)
		case dep.IsRequired():
			return nil, false, fmt.Errorf(
				"%w: the component '%s' in the stack '%s' specifies a 'dependencies.components' dependency "+
					"on the component '%s' in the stack '%s', but '%s' is not a Spacelift-enabled component in the '%s' stack, or the component and stack names are not correct",
				errUtils.ErrInvalidDependenciesComponents,
				currentComponentName,
				currentStackName,
				dep.Component,
				dependsOnStackName,
				dep.Component,
				dependsOnStackName,
			)
		}
	}

	return names, true, nil
}

// parseTerraformDependenciesComponents returns the Terraform component entries of a component's
// `dependencies.components`. Other kinds, files and folders are not Spacelift stacks, so they are dropped.
func parseTerraformDependenciesComponents(atmosConfig *schema.AtmosConfiguration, componentMap map[string]any) ([]schema.ComponentDependency, error) {
	dependenciesValue, exists := componentMap[cfg.DependenciesSectionName]
	if !exists {
		return nil, nil
	}
	depsSection, ok := dependenciesValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be a map", errUtils.ErrInvalidDependenciesSection, cfg.DependenciesSectionName)
	}
	if _, hasComponents := depsSection["components"]; !hasComponents {
		return nil, nil
	}

	leftDelim := ""
	if delimiters := atmosConfig.Templates.Settings.Delimiters; len(delimiters) == 2 {
		leftDelim = delimiters[0]
	}

	// An unrendered `required` template can't be evaluated here, so it keeps the conservative default (required).
	deps, err := schema.ParseComponentDependencies(schema.DeferUnresolvedRequired(depsSection, leftDelim), cfg.TerraformComponentType, "")
	if err != nil {
		return nil, err
	}

	// ParseComponentDependencies only applies the default kind to its dedup key, so an omitted kind stays empty here.
	var terraformDeps []schema.ComponentDependency
	for i := range deps {
		if deps[i].Kind == "" || deps[i].Kind == cfg.TerraformComponentType {
			terraformDeps = append(terraformDeps, deps[i])
		}
	}
	return terraformDeps, nil
}

// resolveDependenciesComponentsStack returns the logical stack a `dependencies.components` entry points at: the
// declaring component's own stack when `stack` is omitted, otherwise `stack` with its Go template rendered.
func resolveDependenciesComponentsStack(
	atmosConfig *schema.AtmosConfiguration,
	dep *schema.ComponentDependency,
	currentComponentName string,
	currentStackName string,
	templateData map[string]any,
) (string, error) {
	if dep.Stack == "" {
		return currentStackName, nil
	}
	if !atmosConfig.Templates.Settings.Enabled {
		return dep.Stack, nil
	}

	return ProcessTmpl(
		atmosConfig,
		fmt.Sprintf("%s-dependencies-components-stack", currentComponentName),
		dep.Stack,
		templateData,
		atmosConfig.Templates.Settings.IgnoreMissingTemplateValues,
	)
}

// BuildComponentPath builds component path (path to the component's physical location on disk).
// The optional componentNameFallback parameter is used when the "component" field is not set
// in the componentSectionMap (e.g., for JIT vendored components with "source" attribute).
func BuildComponentPath(
	atmosConfig *schema.AtmosConfiguration,
	componentSectionMap *map[string]any,
	componentType string,
	componentNameFallback ...string,
) string {
	defer perf.Track(atmosConfig, "exec.BuildComponentPath")()

	// Determine the component folder name.
	componentFolder := GetComponentFolder(componentSectionMap, componentNameFallback...)
	if componentFolder == "" {
		return ""
	}

	// Build the full path based on component type.
	switch componentType {
	case cfg.TerraformComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, componentFolder)
	case cfg.HelmfileComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath, componentFolder)
	case cfg.PackerComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath, componentFolder)
	case cfg.AnsibleComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Ansible.BasePath, componentFolder)
	case cfg.ContainerComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Container.BasePath, componentFolder)
	default:
		// Emulator components (and any type with no filesystem source tree) have no component
		// path. See getComponentBasePath in describe_stacks.go.
		return ""
	}
}

// GetComponentFolder extracts the component folder name from a component section.
// It checks for an explicit "component" field, falling back to componentNameFallback if provided.
// This ensures components with "source" (JIT vendored) or without explicit "component" field
// still have their folder resolved correctly.
func GetComponentFolder(componentSectionMap *map[string]any, componentNameFallback ...string) string {
	defer perf.Track(nil, "exec.GetComponentFolder")()

	// Check for explicit "component" field.
	if stackComponentSection, ok := (*componentSectionMap)[cfg.ComponentSectionName].(string); ok && stackComponentSection != "" {
		return stackComponentSection
	}

	// Use fallback if provided.
	if len(componentNameFallback) > 0 && componentNameFallback[0] != "" {
		return componentNameFallback[0]
	}

	return ""
}

// GetStackNamePattern returns the stack name pattern.
func GetStackNamePattern(atmosConfig *schema.AtmosConfiguration) string {
	return atmosConfig.Stacks.NamePattern
}

// GetStackNameTemplate returns the stack name template.
func GetStackNameTemplate(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "exec.GetStackNameTemplate")()

	return atmosConfig.Stacks.NameTemplate
}

// IsComponentAbstract returns 'true' if the component is abstract.
func IsComponentAbstract(metadataSection map[string]any) bool {
	defer perf.Track(nil, "exec.IsComponentAbstract")()

	if metadataType, ok := metadataSection["type"].(string); ok {
		if metadataType == "abstract" {
			return true
		}
	}
	return false
}
