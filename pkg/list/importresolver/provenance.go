package importresolver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/list/tree"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

const (
	// File extension constants.
	yamlExt = ".yaml"
	ymlExt  = ".yml"

	// Component metadata field names.
	fieldStackFile = "stack_file"
)

// ResolveImportTreeFromProvenance resolves import trees for all stacks using the provenance system.
// Returns: map[stackName]map[componentName][]*tree.ImportNode
//
// Note: This function relies on merge contexts being populated during stack processing.
// Merge contexts are automatically created when ExecuteDescribeStacks processes stack files.
func ResolveImportTreeFromProvenance(
	stacksMap map[string]interface{},
	atmosConfig *schema.AtmosConfiguration,
) (map[string]map[string][]*tree.ImportNode, error) {
	result := make(map[string]map[string][]*tree.ImportNode)

	// Get all merge contexts (keyed by stack file path).
	allMergeContexts := e.GetAllMergeContexts()

	log.Trace("Found merge contexts and stacks", "merge_context_count", len(allMergeContexts), "stack_count", len(stacksMap))

	// Build a map of stack names to their components for quick lookup.
	stackComponents := make(map[string]map[string]bool)
	for stackName, stackData := range stacksMap {
		components := extractComponentsFromStackData(stackData)
		if len(components) > 0 {
			stackComponents[stackName] = components
			log.Trace("Stack has components", "stack", stackName, "component_count", len(components))
		}
	}

	// Iterate over all merge contexts.
	// Each merge context corresponds to a stack file and contains the ImportChain.
	for stackFilePath, ctx := range allMergeContexts {
		if ctx == nil {
			log.Trace("Merge context is nil", fieldStackFile, stackFilePath)
			continue
		}

		if len(ctx.ImportChain) == 0 {
			log.Trace("Merge context has empty import chain", fieldStackFile, stackFilePath)
			continue
		}

		log.Trace("Processing stack file", fieldStackFile, stackFilePath, "import_chain_length", len(ctx.ImportChain), "import_chain", ctx.ImportChain)

		// Find which stack(s) in stacksMap have this file path.
		// We need to search through the stacksMap to find matching atmos_stack_file values.
		matchingStacks := findStacksForFilePath(stackFilePath, stacksMap, atmosConfig)
		if len(matchingStacks) == 0 {
			// Could not determine stack name for this file.
			log.Trace("Could not find stack for file path", fieldStackFile, stackFilePath)
			continue
		}

		// Process each matching stack.
		for stackName, components := range matchingStacks {
			log.Trace("Found stack with components", "stack", stackName, "component_count", len(components), fieldStackFile, stackFilePath)

			// Build import tree from the ImportChain.
			componentImports := make(map[string][]*tree.ImportNode)

			// Get component metadata to extract component folders.
			stackData := stacksMap[stackName]
			componentFolders := extractComponentFolders(stackData)

			for componentName := range components {
				// All components in a stack share the same import chain.
				importNodes := buildImportTreeFromChain(ctx.ImportChain, atmosConfig)

				// Set component folder on the first import node.
				if len(importNodes) > 0 {
					if folder, ok := componentFolders[componentName]; ok {
						// Store component folder in first node for access during rendering.
						importNodes[0].ComponentFolder = folder
					}
				}

				componentImports[componentName] = importNodes
			}

			result[stackName] = componentImports
		}
	}

	return result, nil
}

// findStacksForFilePath finds all stacks that have components from the given file path.
// Returns a map of stackName -> componentNames.
func findStacksForFilePath(
	filePath string,
	stacksMap map[string]interface{},
	atmosConfig *schema.AtmosConfiguration,
) map[string]map[string]bool {
	result := make(map[string]map[string]bool)

	// Normalize the file path for comparison.
	filePath = filepath.Clean(filePath)

	// Convert to relative path for easier comparison.
	basePath := filepath.Clean(atmosConfig.StacksBaseAbsolutePath)
	relFilePath, err := filepath.Rel(basePath, filePath)
	if err != nil {
		relFilePath = filePath
	}

	// Remove .yaml extension from relative path for comparison.
	relFilePathNoExt := strings.TrimSuffix(relFilePath, yamlExt)
	relFilePathNoExt = strings.TrimSuffix(relFilePathNoExt, ymlExt)

	log.Trace("Looking for stacks with file path", "abs", filePath, "rel", relFilePath, "noext", relFilePathNoExt)

	// Iterate through all stacks.
	for stackName, stackData := range stacksMap {
		stackMap, ok := stackData.(map[string]interface{})
		if !ok {
			continue
		}

		// Look for components section.
		componentsSection, ok := stackMap["components"].(map[string]interface{})
		if !ok {
			continue
		}

		components := make(map[string]bool)

		// Iterate through component types (terraform, helmfile, etc).
		for _, typeData := range componentsSection {
			typeMap, ok := typeData.(map[string]interface{})
			if !ok {
				continue
			}

			// Check each component.
			for componentName, componentData := range typeMap {
				componentMap, ok := componentData.(map[string]interface{})
				if !ok {
					continue
				}

				// Check if this component has atmos_stack_file matching our file.
				if stackFile, ok := componentMap["atmos_stack_file"].(string); ok {
					// Normalize stack file path and remove extension.
					stackFileClean := filepath.Clean(stackFile)
					stackFileNoExt := strings.TrimSuffix(stackFileClean, yamlExt)
					stackFileNoExt = strings.TrimSuffix(stackFileNoExt, ymlExt)

					// Try multiple matching strategies:
					// 1. Exact match (with or without extension)
					// 2. Relative path match
					// 3. Match without extensions
					if stackFileClean == filePath || stackFileClean == relFilePath ||
						stackFile == relFilePath || stackFileNoExt == relFilePathNoExt {
						components[componentName] = true
						log.Trace("Component has matching atmos_stack_file", "component", componentName, "stack", stackName, "atmos_stack_file", stackFile, "matched_with", relFilePath)
					}
				}
			}
		}

		if len(components) > 0 {
			result[stackName] = components
		}
	}

	return result
}

// extractComponentFolders extracts component folder paths from stack data.
// Returns a map of componentName -> componentFolder.
func extractComponentFolders(stackData interface{}) map[string]string {
	folders := make(map[string]string)

	stackMap, ok := stackData.(map[string]interface{})
	if !ok {
		return folders
	}

	// Look for components section.
	componentsSection, ok := stackMap["components"].(map[string]interface{})
	if !ok {
		return folders
	}

	// Iterate through component types (terraform, helmfile, etc).
	for componentType, typeData := range componentsSection {
		typeMap, ok := typeData.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract component folder from metadata.
		for componentName, componentData := range typeMap {
			componentMap, ok := componentData.(map[string]interface{})
			if !ok {
				continue
			}

			// Try to get component folder from metadata.component field.
			// This is the "real" component folder that the component uses.
			folder := componentName // Default to component name

			if metadata, ok := componentMap["metadata"].(map[string]interface{}); ok {
				if componentVal, ok := metadata["component"].(string); ok && componentVal != "" {
					folder = componentVal
				}
			}

			// Build full path: components/{type}/{folder}
			fullPath := fmt.Sprintf("components/%s/%s", componentType, folder)
			folders[componentName] = fullPath
		}
	}

	return folders
}

// extractComponentsFromStackData extracts component names from stack data.
func extractComponentsFromStackData(stackData interface{}) map[string]bool {
	components := make(map[string]bool)

	stackMap, ok := stackData.(map[string]interface{})
	if !ok {
		return components
	}

	// Look for components section.
	componentsSection, ok := stackMap["components"].(map[string]interface{})
	if !ok {
		return components
	}

	// Iterate through component types (terraform, helmfile, etc).
	for _, typeData := range componentsSection {
		typeMap, ok := typeData.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract component names.
		for componentName := range typeMap {
			components[componentName] = true
		}
	}

	return components
}

// buildImportTreeFromChain builds an import tree from MergeContext.ImportChain.
// ImportChain[0] = parent stack file
// ImportChain[1..N] = imported files in merge order.
func buildImportTreeFromChain(importChain []string, atmosConfig *schema.AtmosConfiguration) []*tree.ImportNode {
	if len(importChain) <= 1 {
		// No imports (just the stack file itself).
		return nil
	}

	var roots []*tree.ImportNode
	visited := make(map[string]bool)
	importCache := make(map[string][]string)

	// Process each import in the chain (skip index 0 which is the parent stack).
	for i := 1; i < len(importChain); i++ {
		importPath := importChain[i]

		// Convert absolute path to relative path for display.
		relativePath := stripBasePath(importPath, atmosConfig.StacksBaseAbsolutePath)

		// Check for circular reference.
		circular := visited[importPath]

		node := &tree.ImportNode{
			Path:     relativePath,
			Circular: circular,
		}

		if !circular {
			visited[importPath] = true
			// Recursively resolve this import's imports.
			node.Children = resolveImportFileImports(importPath, atmosConfig, visited, importCache)
			// Backtrack to allow same import in different branches.
			delete(visited, importPath)
		}

		roots = append(roots, node)
	}

	return roots
}

// stripBasePath converts an absolute path to a relative path by removing the base path.
func stripBasePath(absolutePath, basePath string) string {
	// Ensure both paths end with separator for consistent comparison.
	if !strings.HasSuffix(basePath, string(filepath.Separator)) {
		basePath += string(filepath.Separator)
	}

	relativePath := strings.TrimPrefix(absolutePath, basePath)

	// Remove .yaml extension for cleaner display.
	relativePath = strings.TrimSuffix(relativePath, yamlExt)
	relativePath = strings.TrimSuffix(relativePath, ymlExt)

	return relativePath
}

// resolveImportFileImports recursively resolves imports from a file.
func resolveImportFileImports(
	importFilePath string,
	atmosConfig *schema.AtmosConfiguration,
	visited map[string]bool,
	cache map[string][]string,
) []*tree.ImportNode {
	// Check cache first.
	if imports, ok := cache[importFilePath]; ok {
		return buildNodesFromImportPaths(imports, importFilePath, atmosConfig, visited, cache)
	}

	// Read imports from the file.
	imports, err := readImportsFromYAMLFile(importFilePath)
	if err != nil {
		// File can't be read or has no imports.
		return nil
	}

	// Cache the imports.
	cache[importFilePath] = imports

	return buildNodesFromImportPaths(imports, importFilePath, atmosConfig, visited, cache)
}

// buildNodesFromImportPaths builds import nodes from a list of import paths.
func buildNodesFromImportPaths(
	imports []string,
	parentFilePath string,
	atmosConfig *schema.AtmosConfiguration,
	visited map[string]bool,
	cache map[string][]string,
) []*tree.ImportNode {
	var children []*tree.ImportNode

	for _, importPath := range imports {
		// Resolve import path to absolute path.
		absolutePath := resolveImportPath(importPath, parentFilePath, atmosConfig)

		// Check for circular reference.
		circular := visited[absolutePath]

		node := &tree.ImportNode{
			Path:     importPath, // Use original relative path for display
			Circular: circular,
		}

		if !circular {
			visited[absolutePath] = true
			// Recursively resolve children.
			node.Children = resolveImportFileImports(absolutePath, atmosConfig, visited, cache)
			// Backtrack for other branches.
			delete(visited, absolutePath)
		}

		children = append(children, node)
	}

	return children
}

// resolveImportPath converts a relative import path to an absolute file path.
func resolveImportPath(importPath, _ string, atmosConfig *schema.AtmosConfiguration) string {
	// Import paths are relative to the stacks base path.
	basePath := atmosConfig.StacksBaseAbsolutePath

	// Add .yaml extension if not present.
	if !strings.HasSuffix(importPath, yamlExt) && !strings.HasSuffix(importPath, ymlExt) {
		importPath += yamlExt
	}

	return filepath.Join(basePath, importPath)
}

// readImportsFromYAMLFile reads the import/imports array from a YAML file.
func readImportsFromYAMLFile(filePath string) ([]string, error) {
	// Read the file.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Parse as YAML.
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", filePath, err)
	}

	// Extract imports (can be "import" or "imports" array).
	// Initialize as empty slice to ensure we return []string{} instead of nil.
	imports := []string{}

	// Check for "import" field.
	if importVal, ok := data["import"]; ok {
		imports = append(imports, extractImportStringsHelper(importVal)...)
	}

	// Check for "imports" field.
	if importsVal, ok := data["imports"]; ok {
		imports = append(imports, extractImportStringsHelper(importsVal)...)
	}

	return imports, nil
}

// extractImportStringsHelper extracts import strings from an interface{} (can be string or []interface{}).
func extractImportStringsHelper(val interface{}) []string {
	var results []string

	switch v := val.(type) {
	case string:
		results = append(results, v)
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok {
				results = append(results, str)
			}
		}
	}

	return results
}
