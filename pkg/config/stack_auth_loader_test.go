package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadStackAuthDefaults_EmptyIncludePaths(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	assert.Empty(t, defaults)
}

func TestLoadStackAuthDefaults_WithStackFiles(t *testing.T) {
	// Create a temporary directory with stack files.
	tmpDir := t.TempDir()

	// Create a stack file with a default identity.
	stackContent := `
auth:
  identities:
    test-identity:
      default: true
    other-identity:
      default: false
`
	stackFile := filepath.Join(tmpDir, "test-stack.yaml")
	err := os.WriteFile(stackFile, []byte(stackContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	assert.Len(t, defaults, 1)
	assert.True(t, defaults["test-identity"])
	assert.False(t, defaults["other-identity"]) // Not present or false.
}

func TestLoadStackAuthDefaults_MultipleFilesConflictingDefaults(t *testing.T) {
	// Create a temporary directory with multiple stack files with DIFFERENT defaults.
	tmpDir := t.TempDir()

	// Create first stack file with one default identity.
	stack1Content := `
auth:
  identities:
    identity-a:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "stack1.yaml"), []byte(stack1Content), 0o644)
	require.NoError(t, err)

	// Create second stack file with a DIFFERENT default identity.
	stack2Content := `
auth:
  identities:
    identity-b:
      default: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "stack2.yaml"), []byte(stack2Content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	// Conflicting defaults from different files are discarded to avoid
	// false "multiple default identities" errors. See issue #2072.
	assert.Empty(t, defaults, "conflicting defaults from different files should be discarded")
}

func TestLoadStackAuthDefaults_MultipleFilesAgreeingDefaults(t *testing.T) {
	// Create a temporary directory with multiple stack files with the SAME default.
	tmpDir := t.TempDir()

	stack1Content := `
auth:
  identities:
    identity-a:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "stack1.yaml"), []byte(stack1Content), 0o644)
	require.NoError(t, err)

	stack2Content := `
auth:
  identities:
    identity-a:
      default: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "stack2.yaml"), []byte(stack2Content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	// When all files agree on the same default, it is preserved.
	assert.Len(t, defaults, 1)
	assert.True(t, defaults["identity-a"])
}

func TestLoadStackAuthDefaults_ExcludePaths(t *testing.T) {
	// Create a temporary directory with stack files.
	tmpDir := t.TempDir()

	// Create a stack file that should be included.
	includeContent := `
auth:
  identities:
    included-identity:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "include.yaml"), []byte(includeContent), 0o644)
	require.NoError(t, err)

	// Create a stack file that should be excluded.
	excludeContent := `
auth:
  identities:
    excluded-identity:
      default: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "exclude.yaml"), []byte(excludeContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "exclude.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	assert.Len(t, defaults, 1)
	assert.True(t, defaults["included-identity"])
	assert.False(t, defaults["excluded-identity"]) // Not present.
}

func TestLoadStackAuthDefaults_NoAuthSection(t *testing.T) {
	// Create a temporary directory with a stack file without auth section.
	tmpDir := t.TempDir()

	stackContent := `
vars:
  stage: dev
  environment: ue2
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.yaml"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	assert.Empty(t, defaults)
}

func TestLoadStackAuthDefaults_InvalidYAML(t *testing.T) {
	// Create a temporary directory with an invalid YAML file.
	tmpDir := t.TempDir()

	// Invalid YAML (unclosed bracket).
	invalidContent := `
auth:
  identities:
    test: [
`
	err := os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte(invalidContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	// Should not error, just skip the invalid file.
	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	assert.Empty(t, defaults)
}

func TestLoadStackAuthDefaults_YAMLWithTemplates(t *testing.T) {
	// Create a temporary directory with a stack file containing Go templates.
	// The loader should handle files that can't be parsed due to templates.
	tmpDir := t.TempDir()

	// YAML with Go template syntax that prevents parsing.
	templateContent := `
auth:
  identities:
    {{ .Identity }}:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "template.yaml"), []byte(templateContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	// Should not error, just skip files with templates.
	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	assert.Empty(t, defaults)
}

func TestMergeStackAuthDefaults_NilAuthConfig(t *testing.T) {
	stackDefaults := map[string]bool{"test-identity": true}

	// Should not panic.
	MergeStackAuthDefaults(nil, stackDefaults)
}

func TestMergeStackAuthDefaults_EmptyDefaults(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"test-identity": {Kind: "aws/assume-role"},
		},
	}

	MergeStackAuthDefaults(authConfig, map[string]bool{})

	// Identity should not have default set.
	assert.False(t, authConfig.Identities["test-identity"].Default)
}

func TestMergeStackAuthDefaults_SetDefault(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"test-identity": {Kind: "aws/assume-role"},
		},
	}

	stackDefaults := map[string]bool{"test-identity": true}

	MergeStackAuthDefaults(authConfig, stackDefaults)

	// Identity should now have default set.
	assert.True(t, authConfig.Identities["test-identity"].Default)
}

func TestMergeStackAuthDefaults_OverridesAtmosYamlDefault(t *testing.T) {
	// Identity already has default: false in atmos.yaml.
	// Stack config sets it to true - stack should take precedence.
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"test-identity": {Kind: "aws/assume-role", Default: false},
		},
	}

	stackDefaults := map[string]bool{"test-identity": true}

	MergeStackAuthDefaults(authConfig, stackDefaults)

	// Stack takes precedence - should be true now.
	assert.True(t, authConfig.Identities["test-identity"].Default)
}

func TestMergeStackAuthDefaults_ClearsAtmosYamlDefault(t *testing.T) {
	// atmos.yaml has identity-a as default.
	// Stack config sets identity-b as default.
	// Stack should take precedence - identity-a should lose default, identity-b should gain it.
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity-a": {Kind: "aws/assume-role", Default: true},
			"identity-b": {Kind: "aws/assume-role", Default: false},
		},
	}

	stackDefaults := map[string]bool{"identity-b": true}

	MergeStackAuthDefaults(authConfig, stackDefaults)

	// Stack takes precedence - identity-a should no longer be default.
	assert.False(t, authConfig.Identities["identity-a"].Default)
	// identity-b should now be default.
	assert.True(t, authConfig.Identities["identity-b"].Default)
}

func TestMergeStackAuthDefaults_NoStackDefault_PreservesAtmosYaml(t *testing.T) {
	// atmos.yaml has a default set.
	// Stack config has no defaults.
	// atmos.yaml default should be preserved.
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"test-identity": {Kind: "aws/assume-role", Default: true},
		},
	}

	// Empty stack defaults - no changes should be made.
	stackDefaults := map[string]bool{}

	MergeStackAuthDefaults(authConfig, stackDefaults)

	// atmos.yaml default should be preserved when stack has no defaults.
	assert.True(t, authConfig.Identities["test-identity"].Default)
}

func TestMergeStackAuthDefaults_IdentityNotInConfig(t *testing.T) {
	// Auth config doesn't have the identity from stack defaults.
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"other-identity": {Kind: "aws/assume-role"},
		},
	}

	stackDefaults := map[string]bool{"missing-identity": true}

	// Should not panic and should not add the missing identity.
	MergeStackAuthDefaults(authConfig, stackDefaults)

	// Missing identity should not be added.
	_, exists := authConfig.Identities["missing-identity"]
	assert.False(t, exists)
	// Other identity should be unchanged.
	assert.False(t, authConfig.Identities["other-identity"].Default)
}

func TestLoadFileForAuthDefaults_NonYAMLFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a non-YAML file.
	err := os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("# README"), 0o644)
	require.NoError(t, err)

	defaults, err := loadFileForAuthDefaults(filepath.Join(tmpDir, "readme.md"))

	require.NoError(t, err)
	assert.Empty(t, defaults)
}

func TestLoadFileForAuthDefaults_YMLExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .yml file with default identity.
	content := `
auth:
  identities:
    yml-identity:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.yml"), []byte(content), 0o644)
	require.NoError(t, err)

	defaults, err := loadFileForAuthDefaults(filepath.Join(tmpDir, "test.yml"))

	require.NoError(t, err)
	assert.Len(t, defaults, 1)
	assert.True(t, defaults["yml-identity"])
}

func TestGetAllStackFiles_EmptyPatterns(t *testing.T) {
	files := getAllStackFiles([]string{}, []string{})

	assert.Empty(t, files)
}

func TestGetAllStackFiles_InvalidPattern(t *testing.T) {
	// An invalid glob pattern should be skipped without error.
	files := getAllStackFiles([]string{"/nonexistent/path/*.yaml"}, []string{})

	assert.Empty(t, files)
}

func TestGetAllStackFiles_InvalidExcludePattern(t *testing.T) {
	// Create a temporary directory with a stack file.
	tmpDir := t.TempDir()

	content := `
auth:
  identities:
    test-identity:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.yaml"), []byte(content), 0o644)
	require.NoError(t, err)

	// Invalid exclude pattern should be skipped without error.
	// The file should still be included.
	files := getAllStackFiles(
		[]string{filepath.Join(tmpDir, "*.yaml")},
		[]string{"/nonexistent/path/[invalid/glob"},
	)

	assert.Len(t, files, 1)
}

func TestLoadFileForAuthDefaults_ReadError(t *testing.T) {
	// Try to read a non-existent file.
	defaults, err := loadFileForAuthDefaults("/nonexistent/path/test.yaml")

	require.Error(t, err)
	assert.Nil(t, defaults)
}

func TestHasAnyDefault_AllFalse(t *testing.T) {
	// Test when all defaults are false.
	defaults := map[string]bool{
		"identity-a": false,
		"identity-b": false,
	}

	result := hasAnyDefault(defaults)
	assert.False(t, result)
}

func TestHasAnyDefault_Empty(t *testing.T) {
	// Test with empty map.
	defaults := map[string]bool{}

	result := hasAnyDefault(defaults)
	assert.False(t, result)
}

func TestHasAnyDefault_OneTrue(t *testing.T) {
	// Test when one default is true.
	defaults := map[string]bool{
		"identity-a": false,
		"identity-b": true,
	}

	result := hasAnyDefault(defaults)
	assert.True(t, result)
}

func TestApplyStackDefaults_FalseDefault(t *testing.T) {
	// Test when stack defaults has an identity set to false.
	// This should not change the identity's default status.
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"test-identity": {Kind: "aws/assume-role", Default: true},
		},
	}

	// Stack defaults has identity set to false - should be skipped.
	stackDefaults := map[string]bool{"test-identity": false}

	// Call applyStackDefaults directly.
	applyStackDefaults(authConfig, stackDefaults)

	// Identity should keep its original default status.
	assert.True(t, authConfig.Identities["test-identity"].Default)
}

func TestLoadStackAuthDefaults_FileReadError(t *testing.T) {
	// Create a temporary directory.
	tmpDir := t.TempDir()

	// Create a valid stack file.
	validContent := `
auth:
  identities:
    valid-identity:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte(validContent), 0o644)
	require.NoError(t, err)

	// Create a directory named with .yaml extension (will fail to read as file).
	err = os.Mkdir(filepath.Join(tmpDir, "invalid.yaml"), 0o755)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	// Should successfully scan valid file, skipping the directory.
	defaults, err := LoadStackAuthDefaults(atmosConfig)

	require.NoError(t, err)
	// Should have found the default from the valid file.
	assert.True(t, defaults["valid-identity"])
}

// ─── Import chain tests ───────────────────────────────────────────────────────

// TestLoadStackAuthDefaults_DefaultInImportedFile verifies that auth defaults
// defined in an imported file (e.g. _defaults.yaml excluded from IncludeStackAbsolutePaths)
// are discovered via the import-chain traversal.
func TestLoadStackAuthDefaults_DefaultInImportedFile(t *testing.T) {
tmpDir := t.TempDir()

// Top-level stack file — no auth section, just imports _defaults.yaml.
topLevel := `
import:
  - _defaults
vars:
  region: us-east-1
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foundation.yaml"), []byte(topLevel), 0o644))

// Imported _defaults file — contains the auth default.
defaults := `
vars:
  stage: dev
auth:
  identities:
    acme-dev:
      default: true
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "_defaults.yaml"), []byte(defaults), 0o644))

atmosConfig := &schema.AtmosConfiguration{
// Only include foundation.yaml (simulate excluded_paths: ["**/_defaults.yaml"]).
IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "foundation.yaml")},
ExcludeStackAbsolutePaths: []string{},
StacksBaseAbsolutePath:    tmpDir,
}

result, err := LoadStackAuthDefaults(atmosConfig)

require.NoError(t, err)
assert.True(t, result["acme-dev"], "auth default from imported file should be discovered")
}

// TestLoadStackAuthDefaults_DefaultInTransitiveImport verifies that auth defaults
// are found even when they are in a file imported by an imported file (two hops).
func TestLoadStackAuthDefaults_DefaultInTransitiveImport(t *testing.T) {
tmpDir := t.TempDir()

// foundation.yaml → dev/_defaults.yaml → acme/_defaults.yaml (has auth)
require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "dev"), 0o755))
require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "acme"), 0o755))

topLevel := `
import:
  - dev/_defaults
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foundation.yaml"), []byte(topLevel), 0o644))

devDefaults := `
import:
  - acme/_defaults
vars:
  stage: dev
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "dev", "_defaults.yaml"), []byte(devDefaults), 0o644))

acmeDefaults := `
auth:
  identities:
    acme-root:
      default: true
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "acme", "_defaults.yaml"), []byte(acmeDefaults), 0o644))

atmosConfig := &schema.AtmosConfiguration{
IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "foundation.yaml")},
ExcludeStackAbsolutePaths: []string{},
StacksBaseAbsolutePath:    tmpDir,
}

result, err := LoadStackAuthDefaults(atmosConfig)

require.NoError(t, err)
assert.True(t, result["acme-root"], "auth default from transitive import should be discovered")
}

// TestLoadStackAuthDefaults_RelativeImportPath verifies that relative import
// paths starting with ".." are correctly resolved from the importing file's directory.
func TestLoadStackAuthDefaults_RelativeImportPath(t *testing.T) {
tmpDir := t.TempDir()

// stacks/orgs/dev/us-east-1/foundation.yaml → imports ../../_defaults (relative)
regionDir := filepath.Join(tmpDir, "orgs", "dev", "us-east-1")
require.NoError(t, os.MkdirAll(regionDir, 0o755))
require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "orgs"), 0o755))

topLevel := `
import:
  - ../../_defaults
`
require.NoError(t, os.WriteFile(filepath.Join(regionDir, "foundation.yaml"), []byte(topLevel), 0o644))

orgsDefaults := `
auth:
  identities:
    org-default:
      default: true
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "orgs", "_defaults.yaml"), []byte(orgsDefaults), 0o644))

atmosConfig := &schema.AtmosConfiguration{
IncludeStackAbsolutePaths: []string{filepath.Join(regionDir, "foundation.yaml")},
ExcludeStackAbsolutePaths: []string{},
StacksBaseAbsolutePath:    tmpDir,
}

result, err := LoadStackAuthDefaults(atmosConfig)

require.NoError(t, err)
assert.True(t, result["org-default"], "auth default from relative-path import should be discovered")
}

// TestLoadStackAuthDefaults_ImportObjectForm verifies that imports specified as
// {path: "..."} objects (not plain strings) are also followed.
func TestLoadStackAuthDefaults_ImportObjectForm(t *testing.T) {
tmpDir := t.TempDir()

topLevel := `
import:
  - path: _defaults
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foundation.yaml"), []byte(topLevel), 0o644))

defaults := `
auth:
  identities:
    obj-identity:
      default: true
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "_defaults.yaml"), []byte(defaults), 0o644))

atmosConfig := &schema.AtmosConfiguration{
IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "foundation.yaml")},
ExcludeStackAbsolutePaths: []string{},
StacksBaseAbsolutePath:    tmpDir,
}

result, err := LoadStackAuthDefaults(atmosConfig)

require.NoError(t, err)
assert.True(t, result["obj-identity"], "auth default from {path:} import object should be discovered")
}

// TestLoadStackAuthDefaults_CircularImports verifies that circular imports do not
// cause an infinite loop.
func TestLoadStackAuthDefaults_CircularImports(t *testing.T) {
tmpDir := t.TempDir()

// a.yaml imports b.yaml; b.yaml imports a.yaml (cycle).
aContent := `
import:
  - b
auth:
  identities:
    identity-a:
      default: true
`
bContent := `
import:
  - a
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(aContent), 0o644))
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.yaml"), []byte(bContent), 0o644))

atmosConfig := &schema.AtmosConfiguration{
IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "a.yaml")},
ExcludeStackAbsolutePaths: []string{},
StacksBaseAbsolutePath:    tmpDir,
}

// Should not hang; circular imports are silently skipped.
result, err := LoadStackAuthDefaults(atmosConfig)

require.NoError(t, err)
assert.True(t, result["identity-a"])
}

// TestLoadStackAuthDefaults_TemplateImportPathSkipped verifies that import paths
// containing Go template syntax are skipped (they cannot be resolved statically).
func TestLoadStackAuthDefaults_TemplateImportPathSkipped(t *testing.T) {
tmpDir := t.TempDir()

// The import path contains a template expression — cannot be resolved.
topLevel := `
import:
  - "{{ .DynamicPath }}"
auth:
  identities:
    direct-identity:
      default: true
`
require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foundation.yaml"), []byte(topLevel), 0o644))

atmosConfig := &schema.AtmosConfiguration{
IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "foundation.yaml")},
ExcludeStackAbsolutePaths: []string{},
StacksBaseAbsolutePath:    tmpDir,
}

result, err := LoadStackAuthDefaults(atmosConfig)

require.NoError(t, err)
// The direct auth default should still be found (the template import path is just skipped).
assert.True(t, result["direct-identity"])
}

// TestExtractImportPathStrings tests the helper that extracts path strings from
// various import section shapes.
func TestExtractImportPathStrings_Nil(t *testing.T) {
assert.Empty(t, extractImportPathStrings(nil))
}

func TestExtractImportPathStrings_PlainString(t *testing.T) {
result := extractImportPathStrings("_defaults")
assert.Equal(t, []string{"_defaults"}, result)
}

func TestExtractImportPathStrings_SliceOfStrings(t *testing.T) {
result := extractImportPathStrings([]interface{}{"a", "b", "c"})
assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestExtractImportPathStrings_SliceOfObjects(t *testing.T) {
result := extractImportPathStrings([]interface{}{
map[string]interface{}{"path": "foo"},
map[string]interface{}{"path": "bar", "context": map[string]interface{}{}},
})
assert.Equal(t, []string{"foo", "bar"}, result)
}

func TestExtractImportPathStrings_SkipsTemplatePaths(t *testing.T) {
result := extractImportPathStrings([]interface{}{"good-path", "{{ .Bad }}"})
assert.Equal(t, []string{"good-path"}, result)
}

func TestContainsTemplateSyntax(t *testing.T) {
assert.True(t, containsTemplateSyntax("{{ .Foo }}"))
assert.True(t, containsTemplateSyntax("prefix-{{ .Foo }}-suffix"))
assert.False(t, containsTemplateSyntax("plain/path"))
assert.False(t, containsTemplateSyntax(""))
}

func TestResolveImportToAbsPath_RelativeDotDot(t *testing.T) {
parentFile := filepath.Join("/", "stacks", "orgs", "dev", "us-east-1", "foundation.yaml")
stacksBase := filepath.Join("/", "stacks")

resolved := resolveImportToAbsPath("../_defaults", parentFile, stacksBase)

expected := filepath.Join("/", "stacks", "orgs", "dev", "_defaults.yaml")
assert.Equal(t, expected, resolved)
}

func TestResolveImportToAbsPath_BasePathRelative(t *testing.T) {
parentFile := filepath.Join("/", "stacks", "orgs", "dev", "foundation.yaml")
stacksBase := filepath.Join("/", "stacks")

resolved := resolveImportToAbsPath("mixins/region/us-east-1", parentFile, stacksBase)

expected := filepath.Join("/", "stacks", "mixins", "region", "us-east-1.yaml")
assert.Equal(t, expected, resolved)
}

func TestResolveImportToAbsPath_EmptyPath(t *testing.T) {
assert.Empty(t, resolveImportToAbsPath("", "/any/parent.yaml", "/stacks"))
}
