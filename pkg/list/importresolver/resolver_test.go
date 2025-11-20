package importresolver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveImportTree_EmptyStacks tests behavior with empty stacks map.
func TestResolveImportTree_EmptyStacks(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: t.TempDir(),
	}

	result, err := ResolveImportTree(map[string]interface{}{}, atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestResolveImportTree_StackWithNoFile tests behavior when stack file doesn't exist.
func TestResolveImportTree_StackWithNoFile(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: t.TempDir(),
	}

	stacksMap := map[string]interface{}{
		"nonexistent-stack": map[string]interface{}{
			"components": map[string]interface{}{},
		},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	// Stack should be in result but with empty imports.
	assert.Contains(t, result, "nonexistent-stack")
	assert.Empty(t, result["nonexistent-stack"])
}

// TestResolveImportTree_StackWithSingleImport tests single import resolution.
func TestResolveImportTree_StackWithSingleImport(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack file with single import.
	stackContent := `
imports:
  - catalog/base
vars:
  environment: prod
`
	stackPath := filepath.Join(tmpDir, "prod.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Create catalog/base file (no imports).
	catalogDir := filepath.Join(tmpDir, "catalog")
	err = os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	baseContent := `
vars:
  common: true
`
	basePath := filepath.Join(catalogDir, "base.yaml")
	err = os.WriteFile(basePath, []byte(baseContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"prod": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	assert.Contains(t, result, "prod")
	assert.Len(t, result["prod"], 1)
	assert.Equal(t, "catalog/base", result["prod"][0].Path)
	assert.False(t, result["prod"][0].Circular)
	assert.Empty(t, result["prod"][0].Children)
}

// TestResolveImportTree_StackWithNestedImports tests nested import chains.
func TestResolveImportTree_StackWithNestedImports(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack file.
	stackContent := `
imports:
  - catalog/base
`
	stackPath := filepath.Join(tmpDir, "prod.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Create catalog directory.
	catalogDir := filepath.Join(tmpDir, "catalog")
	err = os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	// catalog/base imports common/variables.
	baseContent := `
imports:
  - common/variables
vars:
  common: true
`
	basePath := filepath.Join(catalogDir, "base.yaml")
	err = os.WriteFile(basePath, []byte(baseContent), 0o644)
	require.NoError(t, err)

	// Create common directory.
	commonDir := filepath.Join(tmpDir, "common")
	err = os.MkdirAll(commonDir, 0o755)
	require.NoError(t, err)

	// common/variables has no imports.
	variablesContent := `
vars:
  region: us-east-1
`
	variablesPath := filepath.Join(commonDir, "variables.yaml")
	err = os.WriteFile(variablesPath, []byte(variablesContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"prod": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	assert.Contains(t, result, "prod")
	assert.Len(t, result["prod"], 1)
	assert.Equal(t, "catalog/base", result["prod"][0].Path)
	assert.False(t, result["prod"][0].Circular)

	// Check nested import.
	assert.Len(t, result["prod"][0].Children, 1)
	assert.Equal(t, "common/variables", result["prod"][0].Children[0].Path)
	assert.False(t, result["prod"][0].Children[0].Circular)
	assert.Empty(t, result["prod"][0].Children[0].Children)
}

// TestResolveImportTree_MultipleImports tests stack with multiple imports.
func TestResolveImportTree_MultipleImports(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack file with multiple imports.
	stackContent := `
imports:
  - catalog/base
  - catalog/network
  - catalog/security
`
	stackPath := filepath.Join(tmpDir, "prod.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Create catalog files (no imports).
	catalogDir := filepath.Join(tmpDir, "catalog")
	err = os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	for _, name := range []string{"base", "network", "security"} {
		content := `vars: {}`
		path := filepath.Join(catalogDir, name+".yaml")
		err = os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"prod": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	assert.Contains(t, result, "prod")
	assert.Len(t, result["prod"], 3)

	// Verify all imports are present.
	paths := make([]string, 3)
	for i, node := range result["prod"] {
		paths[i] = node.Path
	}
	assert.Contains(t, paths, "catalog/base")
	assert.Contains(t, paths, "catalog/network")
	assert.Contains(t, paths, "catalog/security")
}

// TestResolveImportTree_CircularReference tests circular import detection.
func TestResolveImportTree_CircularReference(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack file importing circular/a.
	stackContent := `
imports:
  - circular/a
`
	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Create circular directory.
	circularDir := filepath.Join(tmpDir, "circular")
	err = os.MkdirAll(circularDir, 0o755)
	require.NoError(t, err)

	// circular/a imports circular/b.
	aContent := `
imports:
  - circular/b
vars:
  name: a
`
	aPath := filepath.Join(circularDir, "a.yaml")
	err = os.WriteFile(aPath, []byte(aContent), 0o644)
	require.NoError(t, err)

	// circular/b imports circular/a (creates circle).
	bContent := `
imports:
  - circular/a
vars:
  name: b
`
	bPath := filepath.Join(circularDir, "b.yaml")
	err = os.WriteFile(bPath, []byte(bContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"stack": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	assert.Contains(t, result, "stack")
	assert.Len(t, result["stack"], 1)
	assert.Equal(t, "circular/a", result["stack"][0].Path)
	assert.False(t, result["stack"][0].Circular)

	// circular/a has child circular/b.
	assert.Len(t, result["stack"][0].Children, 1)
	assert.Equal(t, "circular/b", result["stack"][0].Children[0].Path)
	assert.False(t, result["stack"][0].Children[0].Circular)

	// circular/b tries to import circular/a again - should be marked circular.
	assert.Len(t, result["stack"][0].Children[0].Children, 1)
	assert.Equal(t, "circular/a", result["stack"][0].Children[0].Children[0].Path)
	assert.True(t, result["stack"][0].Children[0].Children[0].Circular, "Expected circular reference to be detected")
}

// TestResolveImportTree_DeepNesting tests deep import chains.
func TestResolveImportTree_DeepNesting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack importing deep/level1.
	stackContent := `
imports:
  - deep/level1
`
	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Create deep directory with nested imports.
	deepDir := filepath.Join(tmpDir, "deep")
	err = os.MkdirAll(deepDir, 0o755)
	require.NoError(t, err)

	// Create 5 levels of nested imports.
	for i := 1; i <= 5; i++ {
		var content string
		if i < 5 {
			content = `
imports:
  - deep/level` + string(rune('0'+i+1)) + `
vars:
  level: ` + string(rune('0'+i))
		} else {
			content = `
vars:
  level: 5
`
		}
		path := filepath.Join(deepDir, "level"+string(rune('0'+i))+".yaml")
		err = os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"stack": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	// Verify deep nesting is resolved.
	assert.Contains(t, result, "stack")
	node := result["stack"][0]
	assert.Equal(t, "deep/level1", node.Path)

	// Navigate through levels 2-4.
	for i := 2; i <= 4; i++ {
		assert.Len(t, node.Children, 1)
		node = node.Children[0]
		assert.Equal(t, "deep/level"+string(rune('0'+i)), node.Path)
		assert.False(t, node.Circular)
	}

	// Level 4's child is level 5, which has no children.
	assert.Len(t, node.Children, 1)
	assert.Equal(t, "deep/level5", node.Children[0].Path)
	assert.Empty(t, node.Children[0].Children)
}

// TestResolveImportTree_MultipleStacks tests multiple stacks independently.
func TestResolveImportTree_MultipleStacks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create prod.yaml.
	prodContent := `
imports:
  - catalog/base
`
	prodPath := filepath.Join(tmpDir, "prod.yaml")
	err := os.WriteFile(prodPath, []byte(prodContent), 0o644)
	require.NoError(t, err)

	// Create staging.yaml.
	stagingContent := `
imports:
  - catalog/network
`
	stagingPath := filepath.Join(tmpDir, "staging.yaml")
	err = os.WriteFile(stagingPath, []byte(stagingContent), 0o644)
	require.NoError(t, err)

	// Create catalog files.
	catalogDir := filepath.Join(tmpDir, "catalog")
	err = os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	for _, name := range []string{"base", "network"} {
		content := `vars: {}`
		path := filepath.Join(catalogDir, name+".yaml")
		err = os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"prod":    map[string]interface{}{},
		"staging": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	// Verify both stacks have their respective imports.
	assert.Contains(t, result, "prod")
	assert.Contains(t, result, "staging")
	assert.Len(t, result["prod"], 1)
	assert.Len(t, result["staging"], 1)
	assert.Equal(t, "catalog/base", result["prod"][0].Path)
	assert.Equal(t, "catalog/network", result["staging"][0].Path)
}

// TestResolveImportTree_CacheBehavior tests that caching works correctly.
func TestResolveImportTree_CacheBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack files that import the same base.
	for _, name := range []string{"prod", "staging"} {
		content := `
imports:
  - catalog/base
`
		path := filepath.Join(tmpDir, name+".yaml")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	// Create catalog/base.
	catalogDir := filepath.Join(tmpDir, "catalog")
	err := os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	baseContent := `
imports:
  - common/variables
vars:
  common: true
`
	basePath := filepath.Join(catalogDir, "base.yaml")
	err = os.WriteFile(basePath, []byte(baseContent), 0o644)
	require.NoError(t, err)

	// Create common/variables.
	commonDir := filepath.Join(tmpDir, "common")
	err = os.MkdirAll(commonDir, 0o755)
	require.NoError(t, err)

	variablesContent := `vars: {}`
	variablesPath := filepath.Join(commonDir, "variables.yaml")
	err = os.WriteFile(variablesPath, []byte(variablesContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"prod":    map[string]interface{}{},
		"staging": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	// Both stacks should have the same import tree structure.
	// This verifies caching doesn't cause issues.
	assert.Contains(t, result, "prod")
	assert.Contains(t, result, "staging")

	for _, stack := range []string{"prod", "staging"} {
		assert.Len(t, result[stack], 1)
		assert.Equal(t, "catalog/base", result[stack][0].Path)
		assert.Len(t, result[stack][0].Children, 1)
		assert.Equal(t, "common/variables", result[stack][0].Children[0].Path)
	}
}

// TestResolveImportTree_BothImportFields tests stack with both import and imports fields.
func TestResolveImportTree_BothImportFields(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack file with both import and imports.
	stackContent := `
import: catalog/base
imports:
  - catalog/network
  - catalog/security
`
	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Create catalog files.
	catalogDir := filepath.Join(tmpDir, "catalog")
	err = os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	for _, name := range []string{"base", "network", "security"} {
		content := `vars: {}`
		path := filepath.Join(catalogDir, name+".yaml")
		err = os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	stacksMap := map[string]interface{}{
		"stack": map[string]interface{}{},
	}

	result, err := ResolveImportTree(stacksMap, atmosConfig)
	require.NoError(t, err)

	assert.Contains(t, result, "stack")
	// Should have all 3 imports (both fields combined).
	assert.Len(t, result["stack"], 3)

	// Verify all expected imports are present.
	paths := []string{}
	for _, node := range result["stack"] {
		paths = append(paths, node.Path)
	}
	assert.ElementsMatch(t, []string{"catalog/base", "catalog/network", "catalog/security"}, paths)
}

// TestFindStackFilePath_PatternOrgsWithStackName tests orgs/<stackName>.yaml pattern.
func TestFindStackFilePath_PatternOrgsWithStackName(t *testing.T) {
	tmpDir := t.TempDir()

	orgsDir := filepath.Join(tmpDir, "orgs")
	err := os.MkdirAll(orgsDir, 0o755)
	require.NoError(t, err)

	stackPath := filepath.Join(orgsDir, "test-stack.yaml")
	err = os.WriteFile(stackPath, []byte("vars: {}"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	result, err := findStackFilePath("test-stack", atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, stackPath, result)
}

// TestFindStackFilePath_PatternRootStackName tests <stackName>.yaml pattern in root.
func TestFindStackFilePath_PatternRootStackName(t *testing.T) {
	tmpDir := t.TempDir()

	stackPath := filepath.Join(tmpDir, "test-stack.yaml")
	err := os.WriteFile(stackPath, []byte("vars: {}"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	result, err := findStackFilePath("test-stack", atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, stackPath, result)
}

// TestFindStackFilePath_PatternTransformed tests hyphen-to-path transformation.
func TestFindStackFilePath_PatternTransformed(t *testing.T) {
	tmpDir := t.TempDir()

	// Create path: platform/region/environment.yaml.
	platformDir := filepath.Join(tmpDir, "platform", "region")
	err := os.MkdirAll(platformDir, 0o755)
	require.NoError(t, err)

	stackPath := filepath.Join(platformDir, "environment.yaml")
	err = os.WriteFile(stackPath, []byte("vars: {}"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	result, err := findStackFilePath("platform-region-environment", atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, stackPath, result)
}

// TestFindStackFilePath_NotFound tests error when stack file not found.
func TestFindStackFilePath_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	_, err := findStackFilePath("nonexistent-stack", atmosConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrStackManifestFileNotFound))
}

// TestFindStackFilePath_EmptyStackName tests behavior with empty stack name.
func TestFindStackFilePath_EmptyStackName(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	_, err := findStackFilePath("", atmosConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrStackManifestFileNotFound))
}

// TestFindStackFilePath_SpecialCharacters tests stack names with special characters.
func TestFindStackFilePath_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stack with underscores.
	stackPath := filepath.Join(tmpDir, "test_stack_name.yaml")
	err := os.WriteFile(stackPath, []byte("vars: {}"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	result, err := findStackFilePath("test_stack_name", atmosConfig)
	require.NoError(t, err)
	assert.Equal(t, stackPath, result)
}

// TestGetStackImports_StackNotFound tests behavior when stack file not found.
func TestGetStackImports_StackNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)

	result, err := getStackImports("nonexistent-stack", atmosConfig, cache)
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Empty(t, cache)
}

// TestGetStackImports_NoImports tests stack file with no imports.
func TestGetStackImports_NoImports(t *testing.T) {
	tmpDir := t.TempDir()

	stackContent := `
vars:
  environment: prod
`
	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)

	result, err := getStackImports("stack", atmosConfig, cache)
	require.NoError(t, err)
	assert.Empty(t, result)

	// Cache should be populated even for empty imports.
	assert.Contains(t, cache, stackPath)
	assert.Empty(t, cache[stackPath])
}

// TestGetStackImports_CacheHit tests that cache is used on second call.
func TestGetStackImports_CacheHit(t *testing.T) {
	tmpDir := t.TempDir()

	stackContent := `
imports:
  - catalog/base
`
	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)

	// First call - populates cache.
	result1, err := getStackImports("stack", atmosConfig, cache)
	require.NoError(t, err)
	assert.Equal(t, []string{"catalog/base"}, result1)
	assert.Contains(t, cache, stackPath)

	// Modify file on disk (shouldn't affect cached result).
	err = os.WriteFile(stackPath, []byte("imports:\n  - different/import"), 0o644)
	require.NoError(t, err)

	// Second call - should use cache.
	result2, err := getStackImports("stack", atmosConfig, cache)
	require.NoError(t, err)
	assert.Equal(t, []string{"catalog/base"}, result2, "Expected cached result, not re-read file")
}

// TestGetStackImports_MultipleImports tests stack with multiple imports.
func TestGetStackImports_MultipleImports(t *testing.T) {
	tmpDir := t.TempDir()

	stackContent := `
imports:
  - catalog/base
  - catalog/network
  - catalog/security
`
	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err := os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)

	result, err := getStackImports("stack", atmosConfig, cache)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "catalog/base")
	assert.Contains(t, result, "catalog/network")
	assert.Contains(t, result, "catalog/security")
}

// TestGetStackImports_InvalidYAML tests error handling for invalid YAML.
func TestGetStackImports_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	invalidContent := `
imports: [unclosed
vars: {broken
`
	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err := os.WriteFile(stackPath, []byte(invalidContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)

	_, err = getStackImports("stack", atmosConfig, cache)
	assert.Error(t, err)
}

// TestReadImportsFromFile_SingleImportField tests reading single import field.
func TestReadImportsFromFile_SingleImportField(t *testing.T) {
	tmpDir := t.TempDir()

	content := `
import: catalog/base
vars:
  environment: prod
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readImportsFromFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"catalog/base"}, result)
}

// TestReadImportsFromFile_MultipleImportsField tests reading imports array.
func TestReadImportsFromFile_MultipleImportsField(t *testing.T) {
	tmpDir := t.TempDir()

	content := `
imports:
  - catalog/base
  - catalog/network
  - catalog/security
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readImportsFromFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"catalog/base", "catalog/network", "catalog/security"}, result)
}

// TestReadImportsFromFile_BothFields tests reading both import and imports fields.
func TestReadImportsFromFile_BothFields(t *testing.T) {
	tmpDir := t.TempDir()

	content := `
import: catalog/base
imports:
  - catalog/network
  - catalog/security
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readImportsFromFile(filePath)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "catalog/base")
	assert.Contains(t, result, "catalog/network")
	assert.Contains(t, result, "catalog/security")
}

// TestReadImportsFromFile_NoImports tests file with no import fields.
func TestReadImportsFromFile_NoImports(t *testing.T) {
	tmpDir := t.TempDir()

	content := `
vars:
  environment: prod
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := readImportsFromFile(filePath)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestReadImportsFromFile_FileNotFound tests error when file doesn't exist.
func TestReadImportsFromFile_FileNotFound(t *testing.T) {
	_, err := readImportsFromFile("/nonexistent/file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

// TestReadImportsFromFile_InvalidYAML tests error handling for invalid YAML.
func TestReadImportsFromFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	invalidContent := `
imports: [unclosed
`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(invalidContent), 0o644)
	require.NoError(t, err)

	_, err = readImportsFromFile(filePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

// TestExtractImportStrings_StringValue tests extracting string value.
func TestExtractImportStrings_StringValue(t *testing.T) {
	result := extractImportStrings("catalog/base")
	assert.Equal(t, []string{"catalog/base"}, result)
}

// TestExtractImportStrings_ArrayOfStrings tests extracting array of strings.
func TestExtractImportStrings_ArrayOfStrings(t *testing.T) {
	result := extractImportStrings([]interface{}{"catalog/base", "catalog/network"})
	assert.Equal(t, []string{"catalog/base", "catalog/network"}, result)
}

// TestExtractImportStrings_EmptyArray tests empty array.
func TestExtractImportStrings_EmptyArray(t *testing.T) {
	result := extractImportStrings([]interface{}{})
	assert.Nil(t, result)
}

// TestExtractImportStrings_MixedTypes tests array with mixed types.
func TestExtractImportStrings_MixedTypes(t *testing.T) {
	result := extractImportStrings([]interface{}{"catalog/base", 123, "catalog/network", true})
	assert.Equal(t, []string{"catalog/base", "catalog/network"}, result)
}

// TestExtractImportStrings_NilValue tests nil value.
func TestExtractImportStrings_NilValue(t *testing.T) {
	result := extractImportStrings(nil)
	assert.Nil(t, result)
}

// TestExtractImportStrings_NonStringNonArray tests non-string, non-array value.
func TestExtractImportStrings_NonStringNonArray(t *testing.T) {
	result := extractImportStrings(123)
	assert.Nil(t, result)
}

// TestResolveImportFilePath_NoExtension tests adding .yaml extension.
func TestResolveImportFilePath_NoExtension(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: "/tmp/stacks",
	}

	result := resolveImportFilePath("catalog/base", atmosConfig)
	assert.Equal(t, "/tmp/stacks/catalog/base.yaml", result)
}

// TestResolveImportFilePath_WithYamlExtension tests existing .yaml extension.
func TestResolveImportFilePath_WithYamlExtension(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: "/tmp/stacks",
	}

	result := resolveImportFilePath("catalog/base.yaml", atmosConfig)
	assert.Equal(t, "/tmp/stacks/catalog/base.yaml", result)
}

// TestResolveImportFilePath_WithYmlExtension tests existing .yml extension.
func TestResolveImportFilePath_WithYmlExtension(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: "/tmp/stacks",
	}

	result := resolveImportFilePath("catalog/base.yml", atmosConfig)
	assert.Equal(t, "/tmp/stacks/catalog/base.yml", result)
}

// TestBuildImportTree_FileNotFound tests graceful handling of missing file.
func TestBuildImportTree_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)
	visited := make(map[string]bool)

	node := buildImportTree("nonexistent/import", atmosConfig, cache, visited)

	assert.Equal(t, "nonexistent/import", node.Path)
	assert.False(t, node.Circular)
	assert.Empty(t, node.Children)
}

// TestBuildImportTree_CircularDetection tests circular reference detection.
func TestBuildImportTree_CircularDetection(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)
	visited := make(map[string]bool)

	// Mark a path as visited to simulate circular reference.
	visited["catalog/base"] = true

	node := buildImportTree("catalog/base", atmosConfig, cache, visited)

	assert.Equal(t, "catalog/base", node.Path)
	assert.True(t, node.Circular, "Expected circular reference to be detected")
	assert.Empty(t, node.Children)
}

// TestBuildImportTree_DeferCleanup tests that visited map is cleaned up.
func TestBuildImportTree_DeferCleanup(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file with no imports.
	content := `vars: {}`
	filePath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	cache := make(map[string][]string)
	visited := make(map[string]bool)

	_ = buildImportTree("test", atmosConfig, cache, visited)

	// After function returns, visited should be cleaned up (defer delete).
	assert.False(t, visited[filepath.Join(tmpDir, "test.yaml")], "Expected visited map to be cleaned up after defer")
}
