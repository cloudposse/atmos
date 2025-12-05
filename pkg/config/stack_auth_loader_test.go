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

func TestLoadStackAuthDefaults_MultipleFiles(t *testing.T) {
	// Create a temporary directory with multiple stack files.
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

	// Create second stack file with another default identity.
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
	assert.Len(t, defaults, 2)
	assert.True(t, defaults["identity-a"])
	assert.True(t, defaults["identity-b"])
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
