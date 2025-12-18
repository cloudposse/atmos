package vendor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// metadataComponentKey is the key for the component in metadata.
const metadataComponentKey = "component"

// executeStackVendorInternal executes the command to vendor all components in an Atmos stack.
func executeStackVendorInternal(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	dryRun bool,
) error {
	defer perf.Track(atmosConfig, "vendor.executeStackVendorInternal")()

	log.Info("Vendoring components for stack", "stack", stack)

	// 1. Load the stack using ExecuteDescribeStacks
	stacksMap, err := exec.ExecuteDescribeStacks(
		atmosConfig,
		stack,      // filterByStack
		nil,        // components
		[]string{}, // componentTypes - empty to get all
		nil,        // sections
		false,      // ignoreMissingFiles
		false,      // processTemplates - don't process templates for vendoring
		false,      // processYamlFunctions
		false,      // includeEmptyStacks
		nil,        // skip
		nil,        // authManager
	)
	if err != nil {
		return fmt.Errorf("failed to describe stack '%s': %w", stack, err)
	}

	if len(stacksMap) == 0 {
		return fmt.Errorf("%w: stack '%s' not found or has no components", errors.ErrStackNotFound, stack)
	}

	// 2. Extract components with vendor configs
	packages, skipped, err := extractVendorableComponents(atmosConfig, stacksMap)
	if err != nil {
		return err
	}

	if len(packages) == 0 {
		log.Info("No vendorable components found in stack", "stack", stack, "skipped", skipped)
		return nil
	}

	log.Info("Found vendorable components", "count", len(packages), "skipped", skipped)

	// 3. Run TUI vendor model
	return executeVendorModel(packages, dryRun, atmosConfig)
}

// extractVendorableComponents extracts components with component.yaml from the stack map.
func extractVendorableComponents(
	atmosConfig *schema.AtmosConfiguration,
	stacksMap map[string]any,
) ([]pkgComponentVendor, int, error) {
	var packages []pkgComponentVendor
	skipped := 0
	processedComponents := make(map[string]bool) // Track processed components to avoid duplicates

	for stackName, stackData := range stacksMap {
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}

		componentsData, ok := stackMap["components"].(map[string]any)
		if !ok {
			continue
		}

		// Process terraform components
		if terraformComponents, ok := componentsData["terraform"].(map[string]any); ok {
			pkgs, skip, err := processStackComponents(
				atmosConfig,
				stackName,
				terraformComponents,
				cfg.TerraformComponentType,
				processedComponents,
			)
			if err != nil {
				return nil, 0, err
			}
			packages = append(packages, pkgs...)
			skipped += skip
		}

		// Process helmfile components
		if helmfileComponents, ok := componentsData["helmfile"].(map[string]any); ok {
			pkgs, skip, err := processStackComponents(
				atmosConfig,
				stackName,
				helmfileComponents,
				cfg.HelmfileComponentType,
				processedComponents,
			)
			if err != nil {
				return nil, 0, err
			}
			packages = append(packages, pkgs...)
			skipped += skip
		}
	}

	return packages, skipped, nil
}

// getComponentBasePath returns the base path for a component type.
func getComponentBasePath(atmosConfig *schema.AtmosConfiguration, componentType string) string {
	switch componentType {
	case cfg.TerraformComponentType:
		return atmosConfig.Components.Terraform.BasePath
	case cfg.HelmfileComponentType:
		return atmosConfig.Components.Helmfile.BasePath
	case cfg.PackerComponentType:
		return atmosConfig.Components.Packer.BasePath
	default:
		return atmosConfig.Components.Terraform.BasePath
	}
}

// processStackComponents processes components from a stack and returns vendorable packages.
func processStackComponents(
	atmosConfig *schema.AtmosConfiguration,
	stackName string,
	components map[string]any,
	componentType string,
	processedComponents map[string]bool,
) ([]pkgComponentVendor, int, error) {
	var packages []pkgComponentVendor
	skipped := 0
	componentBasePath := getComponentBasePath(atmosConfig, componentType)

	for componentName, componentData := range components {
		actualComponentPath := resolveComponentPath(componentName, componentData)
		componentKey := fmt.Sprintf("%s/%s", componentType, actualComponentPath)
		if processedComponents[componentKey] {
			continue
		}

		componentPath := filepath.Join(atmosConfig.BasePath, componentBasePath, actualComponentPath)
		componentConfigFile, err := findComponentConfigFile(componentPath, strings.TrimSuffix(cfg.ComponentVendorConfigFileName, ".yaml"))
		if err != nil {
			log.Debug("Skipping component (no vendor config)", metadataComponentKey, componentName, "path", componentPath)
			skipped++
			continue
		}

		componentConfigFileContent, err := os.ReadFile(componentConfigFile)
		if err != nil {
			return nil, skipped, err
		}
		componentConfig, err := u.UnmarshalYAML[schema.VendorComponentConfig](string(componentConfigFileContent))
		if err != nil {
			return nil, skipped, err
		}
		if componentConfig.Kind != "ComponentVendorConfig" {
			log.Debug("Skipping component (invalid kind)", metadataComponentKey, componentName, "kind", componentConfig.Kind)
			skipped++
			continue
		}

		pkgs, err := createComponentPackages(atmosConfig, actualComponentPath, componentPath, &componentConfig.Spec, componentType)
		if err != nil {
			return nil, skipped, err
		}
		packages = append(packages, pkgs...)
		processedComponents[componentKey] = true
		log.Debug("Found vendorable component", metadataComponentKey, componentName, "stack", stackName, "path", actualComponentPath)
	}

	return packages, skipped, nil
}

// resolveComponentPath resolves the actual component path from component data.
// If metadata.component is set, use that; otherwise use the component name.
func resolveComponentPath(componentName string, componentData any) string {
	compMap, ok := componentData.(map[string]any)
	if !ok {
		return componentName
	}

	// Check if metadata.component is set
	if metadataRaw, ok := compMap["metadata"]; ok {
		if metadata, ok := metadataRaw.(map[string]any); ok {
			if component, ok := metadata[metadataComponentKey].(string); ok && component != "" {
				return component
			}
		}
	}

	return componentName
}

// createComponentPackages creates vendor packages for a component.
func createComponentPackages(
	_ *schema.AtmosConfiguration,
	componentName string,
	componentPath string,
	vendorComponentSpec *schema.VendorComponentSpec,
	_ string,
) ([]pkgComponentVendor, error) {
	var packages []pkgComponentVendor

	if vendorComponentSpec.Source.Uri == "" {
		return nil, nil // No URI, nothing to vendor
	}

	uri := vendorComponentSpec.Source.Uri

	// Determine package type
	useOciScheme := strings.HasPrefix(uri, ociScheme)
	if useOciScheme {
		uri = strings.TrimPrefix(uri, ociScheme)
	}

	var useLocalFileSystem, sourceIsLocalFile bool
	if !useOciScheme {
		uri, useLocalFileSystem, sourceIsLocalFile = handleLocalFileScheme(componentPath, uri)
	}

	pType := determinePackageType(useOciScheme, useLocalFileSystem)

	// Create the main component package
	componentPkg := pkgComponentVendor{
		uri:                 uri,
		name:                componentName,
		componentPath:       componentPath,
		sourceIsLocalFile:   sourceIsLocalFile,
		pkgType:             pType,
		version:             vendorComponentSpec.Source.Version,
		vendorComponentSpec: vendorComponentSpec,
		IsComponent:         true,
	}
	packages = append(packages, componentPkg)

	// Process mixins
	if len(vendorComponentSpec.Mixins) > 0 {
		mixinPkgs, err := processComponentMixins(vendorComponentSpec, componentPath)
		if err != nil {
			return nil, err
		}
		packages = append(packages, mixinPkgs...)
	}

	return packages, nil
}
