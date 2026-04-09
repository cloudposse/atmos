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

// ============================================================================
// Import-following scanner tests — Issue #2293 for Category B commands.
//
// These tests cover the new recursive loadAuthWithImports helper that makes
// LoadStackAuthDefaults follow `import:` chains when scanning for
// `auth.identities.*.default: true`. Before this fix, the scanner only looked
// at each top-level stack file's own auth section and missed defaults declared
// in imported `_defaults.yaml` files — including files that were explicitly
// excluded from standalone processing via `excluded_paths`.
//
// See docs/fixes/2026-04-08-atmos-auth-identity-resolution-fixes.md for the
// full design rationale (option d+).
// ============================================================================

func TestLoadStackAuthDefaults_FollowsImports(t *testing.T) {
	// Top-level stack manifest imports a _defaults.yaml that declares the default.
	// The scanner must follow the import and surface the default to the loader.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	defaultsContent := `
auth:
  identities:
    imported-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "_defaults.yaml"), []byte(defaultsContent), 0o644))

	manifestContent := `
import:
  - _defaults
vars:
  stage: dev
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "manifest.yaml"), []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "manifest.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["imported-identity"], "scanner should follow the import: chain and surface the imported default")
}

func TestLoadStackAuthDefaults_FollowsImportsFromExcludedPath(t *testing.T) {
	// The real-world Issue #2293 layout: _defaults.yaml is in `excluded_paths`
	// so `getAllStackFiles` filters it out, but it is still referenced via
	// `import:` from a top-level manifest. The scanner must resolve that import
	// through the excluded file's path despite the exclusion.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	defaultsContent := `
auth:
  identities:
    excluded-imported-identity:
      default: true
`
	defaultsPath := filepath.Join(stacksDir, "_defaults.yaml")
	require.NoError(t, os.WriteFile(defaultsPath, []byte(defaultsContent), 0o644))

	manifestContent := `
import:
  - _defaults
`
	manifestPath := filepath.Join(stacksDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "*.yaml")},
		// _defaults.yaml is explicitly excluded from standalone processing.
		ExcludeStackAbsolutePaths: []string{defaultsPath},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["excluded-imported-identity"],
		"scanner must follow imports into excluded_paths files — excluded_paths filters standalone processing, not import resolution")
}

func TestLoadStackAuthDefaults_ImportCycleProtection(t *testing.T) {
	// Two files that import each other. The recursive scanner must terminate
	// and return a sensible result without infinite recursion.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	aContent := `
import:
  - b
auth:
  identities:
    a-identity:
      default: true
`
	bContent := `
import:
  - a
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "a.yaml"), []byte(aContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "b.yaml"), []byte(bContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "a.yaml")},
	}

	// Must terminate (if cycle protection fails this test hangs / stack-overflows).
	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["a-identity"], "default from the top-level file must still be returned despite the cycle")
}

func TestLoadStackAuthDefaults_GlobImports(t *testing.T) {
	// Glob import should expand and be followed.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	mixinsDir := filepath.Join(stacksDir, "mixins")
	require.NoError(t, os.MkdirAll(mixinsDir, 0o755))

	mixinContent := `
auth:
  identities:
    mixin-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(mixinsDir, "region.yaml"), []byte(mixinContent), 0o644))

	manifestContent := `
import:
  - mixins/*
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "manifest.yaml"), []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "manifest.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["mixin-identity"], "scanner should glob-expand import paths and follow the matches")
}

func TestLoadStackAuthDefaults_TemplatedImportSkipped(t *testing.T) {
	// Go-template imports cannot be resolved without template context — the
	// scanner must skip them gracefully rather than erroring.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	// The manifest imports a templated path AND declares its own default, so
	// we can verify: (1) scanner does not crash, (2) scanner still picks up
	// the static default from the manifest itself.
	manifestContent := `
import:
  - '{{ .stage }}/_defaults'
auth:
  identities:
    manifest-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "manifest.yaml"), []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "manifest.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["manifest-identity"], "scanner must skip templated imports gracefully and still surface static defaults from the same file")
}

func TestLoadStackAuthDefaults_ConflictingDefaultsAcrossImportAndFileDiscarded(t *testing.T) {
	// When the importing file and its imported file declare defaults for
	// DIFFERENT identities, the merged view of that file contains two
	// competing defaults. The top-level allAgree check detects the conflict
	// and discards both — matching Issue #2072 behavior.
	//
	// This also serves as a proof that the import WAS followed: if the
	// scanner had not seen the imported file, only `manifest-identity`
	// would appear and allAgree would pass (returning a single default).
	// The empty result proves both were seen and conflicted.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	defaultsContent := `
auth:
  identities:
    imported-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "_defaults.yaml"), []byte(defaultsContent), 0o644))

	manifestContent := `
import:
  - _defaults
auth:
  identities:
    manifest-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "manifest.yaml"), []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "manifest.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, defaults, "two competing defaults from import + file must be discarded by allAgree (Issue #2072)")
}

func TestLoadStackAuthDefaults_ExplicitFalseRevokesImportedDefault(t *testing.T) {
	// An imported _defaults.yaml sets `foo.default: true`. The importing file
	// overrides it with `foo.default: false`. The scanner must honor the
	// explicit `false` and NOT report `foo` as a default.
	//
	// Before the *bool fix, `default: false` was indistinguishable from "not
	// mentioned" (both decoded as Go's zero value `false`), so the imported
	// `true` leaked through — the wrong identity was selected for the stack.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	defaultsContent := `
auth:
  identities:
    imported-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "_defaults.yaml"), []byte(defaultsContent), 0o644))

	// The importing file explicitly revokes the imported default.
	manifestContent := `
import:
  - _defaults
auth:
  identities:
    imported-identity:
      default: false
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "manifest.yaml"), []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "manifest.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, defaults, "explicit default: false in the importing file must revoke the imported default: true")
}

func TestLoadStackAuthDefaults_IdentityWithoutDefaultFieldLeavesImportedDefault(t *testing.T) {
	// An imported _defaults.yaml sets `foo.default: true`. The importing file
	// mentions `foo` but without a `default` field at all. The scanner must
	// treat the nil `default` as "not mentioned" and preserve the imported
	// default. This is the complementary test to
	// ExplicitFalseRevokesImportedDefault — it verifies the three-state
	// distinction between nil (preserve), false (revoke), and true (set).
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	defaultsContent := `
auth:
  identities:
    imported-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "_defaults.yaml"), []byte(defaultsContent), 0o644))

	// The importing file mentions the identity but does NOT set `default` at all.
	manifestContent := `
import:
  - _defaults
auth:
  identities:
    imported-identity:
      kind: aws/assume-role
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "manifest.yaml"), []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "manifest.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["imported-identity"], "identity mentioned without default field must preserve the imported default: true")
}

func TestLoadStackAuthDefaults_ImportedDefaultAgreesAcrossStacks(t *testing.T) {
	// Two top-level stacks import the SAME _defaults.yaml that declares a
	// default. Both should report the same identity, allAgree passes, and
	// the default is returned. This is the positive happy-path companion to
	// TestLoadStackAuthDefaults_FollowsImports.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	defaultsContent := `
auth:
  identities:
    shared-identity:
      default: true
`
	defaultsPath := filepath.Join(stacksDir, "_defaults.yaml")
	require.NoError(t, os.WriteFile(defaultsPath, []byte(defaultsContent), 0o644))

	for _, name := range []string{"stack-a.yaml", "stack-b.yaml"} {
		content := `
import:
  - _defaults
`
		require.NoError(t, os.WriteFile(filepath.Join(stacksDir, name), []byte(content), 0o644))
	}

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "stack-*.yaml")},
		ExcludeStackAbsolutePaths: []string{defaultsPath},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["shared-identity"], "both stacks agree on the imported default, so allAgree passes and the default is returned")
}

func TestLoadStackAuthDefaults_RelativeImports(t *testing.T) {
	// `./` and `../` imports must resolve against the importing file's dir,
	// not the stacks base path.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	subDir := filepath.Join(stacksDir, "orgs", "acme", "dev")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	defaultsContent := `
auth:
  identities:
    relative-identity:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "_defaults.yaml"), []byte(defaultsContent), 0o644))

	manifestContent := `
import:
  - ./_defaults
`
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "manifest.yaml"), []byte(manifestContent), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:    stacksDir,
		IncludeStackAbsolutePaths: []string{filepath.Join(subDir, "manifest.yaml")},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.True(t, defaults["relative-identity"], "./ imports must resolve against the importing file's directory")
}

// Helper edge-case tests for resolveAuthImportPaths, extractImportPathString,
// and loadAuthWithImports are in stack_auth_helpers_test.go (table-driven).
