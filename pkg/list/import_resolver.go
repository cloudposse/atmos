package list

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/list/tree"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ResolveImportTree resolves the complete import tree for all stacks.
// Returns a map of stack names to their import trees.
func ResolveImportTree(stacksMap map[string]interface{}, atmosConfig *schema.AtmosConfiguration) (map[string][]*tree.ImportNode, error) {
	result := make(map[string][]*tree.ImportNode)

	// Cache to avoid re-reading the same import file multiple times.
	importCache := make(map[string][]string)

	// Process each stack.
	for stackName := range stacksMap {
		// Get the import paths for this stack.
		imports, err := getStackImports(stackName, atmosConfig, importCache)
		if err != nil {
			return nil, fmt.Errorf("failed to get imports for stack %s: %w", stackName, err)
		}

		// Build the import tree recursively.
		var importNodes []*tree.ImportNode
		visited := make(map[string]bool)
		for _, importPath := range imports {
			node := buildImportTree(importPath, atmosConfig, importCache, visited)
			importNodes = append(importNodes, node)
		}

		result[stackName] = importNodes
	}

	return result, nil
}

// getStackImports returns the import paths for a given stack.
func getStackImports(stackName string, atmosConfig *schema.AtmosConfiguration, cache map[string][]string) ([]string, error) {
	// Find the stack file path.
	stackFilePath, err := findStackFilePath(stackName, atmosConfig)
	if err != nil {
		// Stack might not have a direct file (could be generated), return empty imports.
		// Only treat ErrStackManifestFileNotFound as non-error; propagate other errors.
		if errors.Is(err, errUtils.ErrStackManifestFileNotFound) {
			return []string{}, nil
		}
		return nil, err
	}

	// Check cache first.
	if imports, ok := cache[stackFilePath]; ok {
		return imports, nil
	}

	// Read the stack file.
	imports, err := readImportsFromFile(stackFilePath)
	if err != nil {
		return nil, err
	}

	// Cache the result.
	cache[stackFilePath] = imports

	return imports, nil
}

// findStackFilePath attempts to find the file path for a stack.
// Stacks follow the pattern: stacks/orgs/{org}/{tenant}/{environment}/*.yaml.
func findStackFilePath(stackName string, atmosConfig *schema.AtmosConfiguration) (string, error) {
	// Try to construct the file path from the stack name.
	// Stack names follow pattern like "plat-ue2-prod" which maps to files in stacks/orgs/

	// For now, we'll search through all stack files to find matches.
	// This is not perfect but works for the common case.
	stacksBasePath := atmosConfig.StacksBaseAbsolutePath

	// Try common patterns.
	transformed := strings.ReplaceAll(stackName, "-", string(os.PathSeparator))
	possiblePaths := []string{
		filepath.Join(stacksBasePath, "orgs", stackName+".yaml"),
		filepath.Join(stacksBasePath, stackName+".yaml"),
		filepath.Join(stacksBasePath, transformed+".yaml"),
	}

	for _, path := range possiblePaths {
		if u.FileExists(path) {
			return path, nil
		}
	}

	// If not found, return error (stack file might not exist as a standalone file).
	return "", fmt.Errorf("%w: %s", errUtils.ErrStackManifestFileNotFound, stackName)
}

// readImportsFromFile reads the import/imports array from a YAML file.
func readImportsFromFile(filePath string) ([]string, error) {
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
	var imports []string

	// Check for "import" field.
	if importVal, ok := data["import"]; ok {
		imports = append(imports, extractImportStrings(importVal)...)
	}

	// Check for "imports" field.
	if importsVal, ok := data["imports"]; ok {
		imports = append(imports, extractImportStrings(importsVal)...)
	}

	return imports, nil
}

// extractImportStrings extracts import strings from an interface{} (can be string or []interface{}).
func extractImportStrings(val interface{}) []string {
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

// buildImportTree recursively builds the import tree for a given import path.
func buildImportTree(importPath string, atmosConfig *schema.AtmosConfiguration, cache map[string][]string, visited map[string]bool) *tree.ImportNode {
	node := &tree.ImportNode{
		Path:     importPath,
		Circular: false,
	}

	// Check for circular reference.
	if visited[importPath] {
		node.Circular = true
		return node
	}

	// Mark as visited.
	visited[importPath] = true
	defer func() {
		// Unmark when backtracking (allows same import in different branches).
		delete(visited, importPath)
	}()

	// Resolve the file path for this import.
	importFilePath := resolveImportFilePath(importPath, atmosConfig)

	// Read imports from this file.
	childImports, err := readImportsFromFile(importFilePath)
	if err != nil {
		// If we can't read the file, just return the node without children.
		return node
	}

	// Cache the imports.
	cache[importFilePath] = childImports

	// Recursively build children.
	for _, childImportPath := range childImports {
		childNode := buildImportTree(childImportPath, atmosConfig, cache, visited)
		node.Children = append(node.Children, childNode)
	}

	return node
}

// resolveImportFilePath converts an import path to an absolute file path.
func resolveImportFilePath(importPath string, atmosConfig *schema.AtmosConfiguration) string {
	stacksBasePath := atmosConfig.StacksBaseAbsolutePath

	// Import paths are relative to the stacks base path.
	// They may or may not have .yaml extension.
	if !strings.HasSuffix(importPath, ".yaml") && !strings.HasSuffix(importPath, ".yml") {
		importPath += ".yaml"
	}

	return filepath.Join(stacksBasePath, importPath)
}
