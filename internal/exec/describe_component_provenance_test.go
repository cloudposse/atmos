package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeComponent_NestedImportProvenance tests that provenance is tracked correctly
// for imports that come from nested import chains (imports that themselves have imports).
func TestDescribeComponent_NestedImportProvenance(t *testing.T) {
	// Clear cache and merge context to ensure fresh processing.
	ClearBaseComponentConfigCache()
	ClearMergeContexts()
	ClearLastMergeContext()
	ClearFileContentCache()

	// Skip if not in repo root or examples directory doesn't exist.
	examplesPath := "../../examples/quick-start-advanced"
	if _, err := os.Stat(examplesPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: examples/quick-start-advanced directory not found")
	}

	// Change to the quick-start-advanced directory.
	t.Chdir(examplesPath)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize config. `kms-key` is used because it resolves fully offline (no
	// `!terraform.output`/`!store`/`!secret` that need a running emulator), while
	// still inheriting the deep nested import chain via `catalog/backend`.
	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = "kms-key"
	configAndStacksInfo.Stack = "plat-ue2-dev"
	configAndStacksInfo.ComponentType = cfg.TerraformComponentType

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Enable provenance tracking
	atmosConfig.TrackProvenance = true

	// Execute describe component with context to get provenance
	result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
		AtmosConfig:          &atmosConfig,
		Component:            "kms-key",
		Stack:                "plat-ue2-dev",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.MergeContext)

	// Get the imports from the component section
	importsRaw, exists := result.ComponentSection["imports"]
	require.True(t, exists, "imports key should exist in component section")

	// Convert []any to []string
	importsAny, ok := importsRaw.([]any)
	require.True(t, ok, "imports should be a []any, got %T", importsRaw)

	imports := make([]string, len(importsAny))
	for i, imp := range importsAny {
		imports[i] = imp.(string)
	}
	require.NotEmpty(t, imports, "imports should not be empty")

	// Expected imports in the flattened list (order may vary). Most of these are
	// nested: `orgs/acme/plat/_defaults` imports `catalog/backend`, which in turn
	// imports every catalog component (kms-key, s3-bucket, …).
	expectedImports := map[string]bool{
		"catalog/backend":                 true,
		"catalog/emulator/aws":            true, // Nested: from catalog/backend
		"catalog/kms-key/defaults":        true, // Nested: from catalog/backend
		"catalog/s3-bucket/defaults":      true, // Nested: from catalog/backend
		"catalog/sns-topic/defaults":      true, // Nested: from catalog/backend
		"catalog/sqs-queue/defaults":      true, // Nested: from catalog/backend
		"catalog/dynamodb-table/defaults": true, // Nested: from catalog/backend
		"catalog/app-config/defaults":     true, // Nested: from catalog/backend
		"mixins/region/us-east-2":         true,
		"orgs/acme/_defaults":             true,
		"orgs/acme/plat/_defaults":        true,
		"orgs/acme/plat/dev/_defaults":    true,
	}

	// Verify we have all expected imports
	for _, imp := range imports {
		if expectedImports[imp] {
			delete(expectedImports, imp)
		}
	}
	assert.Empty(t, expectedImports, "All expected imports should be present in the flattened list")

	// Check that provenance was recorded for each import
	ctx := result.MergeContext
	require.True(t, ctx.IsProvenanceEnabled(), "Provenance should be enabled")

	// Build a map of which imports have provenance
	importsWithProvenance := make(map[string]bool)
	importsMissingProvenance := []string{}

	for _, importPath := range imports {
		// Check if ANY provenance exists for this import
		hasProvenance := false

		// Check __import_meta__ key
		metaKey := "__import_meta__:" + importPath
		if ctx.HasProvenance(metaKey) {
			hasProvenance = true
		}

		// Check __import__ key
		yamlKey := "__import__:" + importPath
		if ctx.HasProvenance(yamlKey) {
			hasProvenance = true
		}

		if hasProvenance {
			importsWithProvenance[importPath] = true
		} else {
			importsMissingProvenance = append(importsMissingProvenance, importPath)
		}
	}

	// CRITICAL ASSERTION: All imports should have provenance metadata
	// This is the test that will fail with the bug and pass with the fix
	if len(importsMissingProvenance) > 0 {
		t.Errorf("The following imports are missing provenance metadata:\n")
		for _, imp := range importsMissingProvenance {
			t.Errorf("  - %s\n", imp)
		}
		t.Errorf("\nThis indicates that nested imports (imports that come from imported files)\n")
		t.Errorf("are not having their provenance tracked correctly.\n")
		t.Errorf("\nImports WITH provenance: %v\n", len(importsWithProvenance))
		t.Errorf("Imports WITHOUT provenance: %v\n", len(importsMissingProvenance))
		t.Fail()
	}

	// Additional assertion: a nested import has correct provenance.
	// catalog/kms-key/defaults is imported by catalog/backend.yaml (which is itself
	// imported by orgs/acme/plat/_defaults), so it is a nested import (depth > 0)
	// whose importing file is catalog/backend.yaml.
	metaKey := "__import_meta__:catalog/kms-key/defaults"
	if ctx.HasProvenance(metaKey) {
		entries := ctx.GetProvenance(metaKey)
		require.NotEmpty(t, entries, "catalog/kms-key/defaults should have provenance entry")

		entry := entries[0]
		assert.Contains(t, entry.File, "backend",
			"catalog/kms-key/defaults should be imported from catalog/backend.yaml")

		// Depth should be > 0 since it's a nested import.
		assert.Greater(t, entry.Depth, 0,
			"catalog/kms-key/defaults is a nested import and should have depth > 0")
	}

	// Clean up after test to avoid polluting subsequent tests
	ClearBaseComponentConfigCache()
	ClearMergeContexts()
	ClearLastMergeContext()
	ClearFileContentCache()
}

// TestDescribeComponent_DirectImportProvenance tests that provenance is tracked correctly
// for imports that appear directly in the stack file being described (not nested).
func TestDescribeComponent_DirectImportProvenance(t *testing.T) {
	// Clear cache and merge context to ensure fresh processing.
	ClearBaseComponentConfigCache()
	ClearMergeContexts()
	ClearLastMergeContext()
	ClearFileContentCache()

	// Skip if not in repo root or examples directory doesn't exist.
	examplesPath := "../../examples/quick-start-advanced"
	if _, err := os.Stat(examplesPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: examples/quick-start-advanced directory not found")
	}

	// Change to the quick-start-advanced directory.
	t.Chdir(examplesPath)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize config. `kms-key` resolves fully offline (see the nested-import test).
	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = "kms-key"
	configAndStacksInfo.Stack = "plat-ue2-dev"
	configAndStacksInfo.ComponentType = cfg.TerraformComponentType

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Enable provenance tracking
	atmosConfig.TrackProvenance = true

	// Execute describe component with context to get provenance
	result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
		AtmosConfig:          &atmosConfig,
		Component:            "kms-key",
		Stack:                "plat-ue2-dev",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.MergeContext)

	ctx := result.MergeContext

	// The leaf stack file orgs/acme/plat/dev/us-east-2.yaml has these direct imports:
	// - orgs/acme/plat/dev/_defaults
	// - mixins/region/us-east-2

	// Check that these direct imports have BOTH __import_meta__ AND __import__ entries
	directImports := []string{
		"orgs/acme/plat/dev/_defaults",
		"mixins/region/us-east-2",
	}

	for _, importPath := range directImports {
		// Should have __import__ entry with accurate line numbers from YAML parsing
		yamlKey := "__import__:" + importPath
		assert.True(t, ctx.HasProvenance(yamlKey),
			"Direct import %s should have __import__ entry from YAML parsing", importPath)

		if ctx.HasProvenance(yamlKey) {
			entries := ctx.GetProvenance(yamlKey)
			require.NotEmpty(t, entries)
			entry := entries[0]

			// Line number should be > 1 (not placeholder)
			assert.Greater(t, entry.Line, 1,
				"Direct import %s should have real line number from YAML, not placeholder", importPath)

			// Should point to the stack file being described
			assert.Contains(t, entry.File, "us-east-2",
				"Direct import %s should be from the us-east-2 stack file", importPath)
		}

		// Should also have __import_meta__ entry
		metaKey := "__import_meta__:" + importPath
		assert.True(t, ctx.HasProvenance(metaKey),
			"Direct import %s should have __import_meta__ entry", importPath)
	}

	// Clean up after test to avoid polluting subsequent tests
	ClearBaseComponentConfigCache()
	ClearMergeContexts()
	ClearLastMergeContext()
	ClearFileContentCache()
}
