package merge

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMergeContext(t *testing.T) {
	ctx := NewMergeContext()
	assert.NotNil(t, ctx)
	assert.Empty(t, ctx.CurrentFile)
	assert.Empty(t, ctx.ImportChain)
	assert.Nil(t, ctx.ParentContext)
}

func TestMergeContext_WithFile(t *testing.T) {
	ctx := NewMergeContext()

	// Test adding first file
	ctx1 := ctx.WithFile("stacks/base.yaml")
	assert.Equal(t, "stacks/base.yaml", ctx1.CurrentFile)
	assert.Equal(t, []string{"stacks/base.yaml"}, ctx1.ImportChain)
	assert.Equal(t, ctx, ctx1.ParentContext)

	// Test adding second file (import chain)
	ctx2 := ctx1.WithFile("stacks/import.yaml")
	assert.Equal(t, "stacks/import.yaml", ctx2.CurrentFile)
	assert.Equal(t, []string{"stacks/base.yaml", "stacks/import.yaml"}, ctx2.ImportChain)
	assert.Equal(t, ctx1, ctx2.ParentContext)

	// Test nil context
	var nilCtx *MergeContext
	ctx3 := nilCtx.WithFile("stacks/file.yaml")
	assert.NotNil(t, ctx3)
	assert.Equal(t, "stacks/file.yaml", ctx3.CurrentFile)
	assert.Equal(t, []string{"stacks/file.yaml"}, ctx3.ImportChain)
}

func TestMergeContext_Clone(t *testing.T) {
	// Test cloning populated context
	ctx := NewMergeContext()
	ctx = ctx.WithFile("file1.yaml")
	ctx = ctx.WithFile("file2.yaml")

	clone := ctx.Clone()
	assert.Equal(t, ctx.CurrentFile, clone.CurrentFile)
	assert.Equal(t, ctx.ImportChain, clone.ImportChain)
	assert.Equal(t, ctx.ParentContext, clone.ParentContext)

	// Ensure it's a deep copy - modifying clone shouldn't affect original
	clone.ImportChain = append(clone.ImportChain, "file3.yaml")
	assert.NotEqual(t, ctx.ImportChain, clone.ImportChain)

	// Test cloning nil context
	var nilCtx *MergeContext
	clone2 := nilCtx.Clone()
	assert.NotNil(t, clone2)
	assert.Empty(t, clone2.CurrentFile)
	assert.Empty(t, clone2.ImportChain)
}

func TestMergeContext_FormatError(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *MergeContext
		inputError     error
		additionalInfo []string
		expectedParts  []string
	}{
		{
			name: "nil error returns nil",
			setupContext: func() *MergeContext {
				return NewMergeContext().WithFile("test.yaml")
			},
			inputError:    nil,
			expectedParts: nil,
		},
		{
			name: "nil context returns original error",
			setupContext: func() *MergeContext {
				return nil
			},
			inputError:    errors.New("original error"),
			expectedParts: []string{"original error"},
		},
		{
			name:          "empty context returns original error",
			setupContext:  NewMergeContext,
			inputError:    errors.New("original error"),
			expectedParts: []string{"original error"},
		},
		{
			name: "context with single file",
			setupContext: func() *MergeContext {
				return NewMergeContext().WithFile("stacks/base.yaml")
			},
			inputError: errors.New("test error"),
			expectedParts: []string{
				"test error",
				"File being processed: stacks/base.yaml",
				"Import chain:",
				"→ stacks/base.yaml",
			},
		},
		{
			name: "context with import chain",
			setupContext: func() *MergeContext {
				ctx := NewMergeContext()
				ctx = ctx.WithFile("stacks/base.yaml")
				ctx = ctx.WithFile("stacks/import1.yaml")
				ctx = ctx.WithFile("stacks/import2.yaml")
				return ctx
			},
			inputError: errors.New("merge failed"),
			expectedParts: []string{
				"merge failed",
				"File being processed: stacks/import2.yaml",
				"Import chain:",
				"→ stacks/base.yaml",
				"→ stacks/import1.yaml",
				"→ stacks/import2.yaml",
			},
		},
		{
			name: "slice type mismatch error",
			setupContext: func() *MergeContext {
				return NewMergeContext().WithFile("stacks/conflict.yaml")
			},
			inputError: errors.New("cannot override two slices with different type ([]interface {}, string)"),
			expectedParts: []string{
				"cannot override two slices with different type",
				"File being processed: stacks/conflict.yaml",
				"**Likely cause:** A key is defined as an array in one file and as a string in another",
				"**Debug hint:** Check the files above for keys that have different types",
				"**Common issues:**",
				"`vars` defined as both array and string",
				"`settings` with inconsistent types",
				"`overrides` attempting to change field types",
			},
		},
		{
			name: "error with additional info",
			setupContext: func() *MergeContext {
				return NewMergeContext().WithFile("test.yaml")
			},
			inputError:     errors.New("validation error"),
			additionalInfo: []string{"Check line 42", "Expected: array, Got: string"},
			expectedParts: []string{
				"validation error",
				"File being processed: test.yaml",
				"Check line 42",
				"Expected: array, Got: string",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupContext()
			formattedErr := ctx.FormatError(tt.inputError, tt.additionalInfo...)

			if tt.inputError == nil {
				assert.Nil(t, formattedErr)
			} else if tt.expectedParts != nil {
				assert.NotNil(t, formattedErr)
				errStr := formattedErr.Error()
				for _, part := range tt.expectedParts {
					assert.Contains(t, errStr, part, "Error message should contain: %s", part)
				}
			}
		})
	}
}

func TestMergeContext_GetImportChainString(t *testing.T) {
	// Test nil context
	var nilCtx *MergeContext
	assert.Empty(t, nilCtx.GetImportChainString())

	// Test empty context
	ctx := NewMergeContext()
	assert.Empty(t, ctx.GetImportChainString())

	// Test single file
	ctx = ctx.WithFile("file1.yaml")
	assert.Equal(t, "file1.yaml", ctx.GetImportChainString())

	// Test multiple files
	ctx = ctx.WithFile("file2.yaml")
	ctx = ctx.WithFile("file3.yaml")
	assert.Equal(t, "file1.yaml → file2.yaml → file3.yaml", ctx.GetImportChainString())
}

func TestMergeContext_GetDepth(t *testing.T) {
	// Test nil context
	var nilCtx *MergeContext
	assert.Equal(t, 0, nilCtx.GetDepth())

	// Test empty context
	ctx := NewMergeContext()
	assert.Equal(t, 0, ctx.GetDepth())

	// Test with files
	ctx = ctx.WithFile("file1.yaml")
	assert.Equal(t, 1, ctx.GetDepth())

	ctx = ctx.WithFile("file2.yaml")
	assert.Equal(t, 2, ctx.GetDepth())

	ctx = ctx.WithFile("file3.yaml")
	assert.Equal(t, 3, ctx.GetDepth())
}

func TestMergeContext_HasFile(t *testing.T) {
	// Test nil context
	var nilCtx *MergeContext
	assert.False(t, nilCtx.HasFile("any.yaml"))

	// Test empty context
	ctx := NewMergeContext()
	assert.False(t, ctx.HasFile("test.yaml"))

	// Add files and test
	ctx = ctx.WithFile("file1.yaml")
	ctx = ctx.WithFile("file2.yaml")
	ctx = ctx.WithFile("file3.yaml")

	assert.True(t, ctx.HasFile("file1.yaml"))
	assert.True(t, ctx.HasFile("file2.yaml"))
	assert.True(t, ctx.HasFile("file3.yaml"))
	assert.False(t, ctx.HasFile("file4.yaml"))
	assert.False(t, ctx.HasFile(""))
}

func TestMergeContext_CircularImportDetection(t *testing.T) {
	ctx := NewMergeContext()

	// Build an import chain
	ctx = ctx.WithFile("stacks/base.yaml")
	assert.False(t, ctx.HasFile("stacks/import.yaml"))

	ctx = ctx.WithFile("stacks/import.yaml")
	assert.True(t, ctx.HasFile("stacks/import.yaml"))

	// This would indicate a circular import
	assert.True(t, ctx.HasFile("stacks/base.yaml"))
}

func TestMergeContext_RealWorldErrorScenario(t *testing.T) {
	// Simulate a real-world merge error scenario
	ctx := NewMergeContext()
	ctx = ctx.WithFile("stacks/catalog/base.yaml")
	ctx = ctx.WithFile("stacks/mixins/region/us-east-1.yaml")
	ctx = ctx.WithFile("stacks/dev/environment.yaml")

	// Simulate the actual mergo error
	mergoError := errors.New("cannot override two slices with different type ([]interface {}, string)")

	formattedErr := ctx.FormatError(mergoError)
	assert.NotNil(t, formattedErr)

	errStr := formattedErr.Error()

	// Verify the error message contains all the helpful information
	assert.Contains(t, errStr, "cannot override two slices with different type")
	assert.Contains(t, errStr, "File being processed: stacks/dev/environment.yaml")
	assert.Contains(t, errStr, "Import chain:")
	assert.Contains(t, errStr, "stacks/catalog/base.yaml")
	assert.Contains(t, errStr, "stacks/mixins/region/us-east-1.yaml")
	assert.Contains(t, errStr, "stacks/dev/environment.yaml")
	assert.Contains(t, errStr, "Likely cause:")
	assert.Contains(t, errStr, "Debug hint:")

	// The error should be actionable
	assert.True(t, strings.Contains(errStr, "array") && strings.Contains(errStr, "string"))
}

// Provenance-related tests.

func TestMergeContext_EnableProvenance(t *testing.T) {
	tests := []struct {
		name    string
		context *MergeContext
	}{
		{
			name:    "enable on new context",
			context: NewMergeContext(),
		},
		{
			name:    "enable on nil context (should not panic)",
			context: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				tt.context.EnableProvenance()
			})

			if tt.context != nil {
				assert.NotNil(t, tt.context.Provenance)
				assert.True(t, tt.context.IsProvenanceEnabled())
			}
		})
	}
}

func TestMergeContext_EnableProvenance_Idempotent(t *testing.T) {
	ctx := NewMergeContext()

	// Enable twice - should not create a new storage.
	ctx.EnableProvenance()
	firstStorage := ctx.Provenance

	ctx.EnableProvenance()
	secondStorage := ctx.Provenance

	assert.Same(t, firstStorage, secondStorage)
}

func TestMergeContext_RecordProvenance(t *testing.T) {
	tests := []struct {
		name    string
		context *MergeContext
		enabled bool
	}{
		{
			name:    "record with provenance enabled",
			context: NewMergeContext(),
			enabled: true,
		},
		{
			name:    "record with provenance disabled (should be no-op)",
			context: NewMergeContext(),
			enabled: false,
		},
		{
			name:    "record on nil context (should not panic)",
			context: nil,
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.enabled && tt.context != nil {
				tt.context.EnableProvenance()
			}

			entry := ProvenanceEntry{
				File: "config.yaml",
				Line: 10,
				Type: ProvenanceTypeInline,
			}

			assert.NotPanics(t, func() {
				tt.context.RecordProvenance("vars.name", entry)
			})

			if tt.enabled && tt.context != nil {
				assert.True(t, tt.context.HasProvenance("vars.name"))
			} else if tt.context != nil {
				assert.False(t, tt.context.HasProvenance("vars.name"))
			}
		})
	}
}

func TestMergeContext_GetProvenance(t *testing.T) {
	ctx := NewMergeContext()
	ctx.EnableProvenance()

	// Record multiple entries for inheritance chain.
	entry1 := ProvenanceEntry{File: "base.yaml", Line: 5, Type: ProvenanceTypeImport}
	entry2 := ProvenanceEntry{File: "override.yaml", Line: 10, Type: ProvenanceTypeOverride}

	ctx.RecordProvenance("vars.name", entry1)
	ctx.RecordProvenance("vars.name", entry2)

	// Get provenance chain.
	chain := ctx.GetProvenance("vars.name")
	assert.Len(t, chain, 2)
	assert.Equal(t, entry1.File, chain[0].File)
	assert.Equal(t, entry2.File, chain[1].File)
}

func TestMergeContext_GetProvenance_Disabled(t *testing.T) {
	ctx := NewMergeContext()

	// Get provenance without enabling (should return nil).
	chain := ctx.GetProvenance("vars.name")
	assert.Nil(t, chain)
}

func TestMergeContext_GetProvenance_NilContext(t *testing.T) {
	var ctx *MergeContext

	// Get provenance on nil context (should return nil, not panic).
	chain := ctx.GetProvenance("vars.name")
	assert.Nil(t, chain)
}

func TestMergeContext_HasProvenance(t *testing.T) {
	ctx := NewMergeContext()
	ctx.EnableProvenance()

	// Initially no provenance.
	assert.False(t, ctx.HasProvenance("vars.name"))

	// Record provenance.
	ctx.RecordProvenance("vars.name", ProvenanceEntry{File: "config.yaml", Line: 10})

	// Now has provenance.
	assert.True(t, ctx.HasProvenance("vars.name"))
}

func TestMergeContext_GetProvenancePaths(t *testing.T) {
	ctx := NewMergeContext()
	ctx.EnableProvenance()

	// Initially empty.
	assert.Empty(t, ctx.GetProvenancePaths())

	// Record provenance for multiple paths.
	ctx.RecordProvenance("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
	ctx.RecordProvenance("vars.tags", ProvenanceEntry{File: "b.yaml", Line: 2})

	// Get paths (should be sorted).
	paths := ctx.GetProvenancePaths()
	assert.Equal(t, []string{"vars.name", "vars.tags"}, paths)
}

func TestMergeContext_IsProvenanceEnabled(t *testing.T) {
	tests := []struct {
		name     string
		context  *MergeContext
		enabled  bool
		expected bool
	}{
		{
			name:     "enabled",
			context:  NewMergeContext(),
			enabled:  true,
			expected: true,
		},
		{
			name:     "disabled",
			context:  NewMergeContext(),
			enabled:  false,
			expected: false,
		},
		{
			name:     "nil context",
			context:  nil,
			enabled:  false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.enabled && tt.context != nil {
				tt.context.EnableProvenance()
			}

			result := tt.context.IsProvenanceEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeContext_Clone_WithProvenance(t *testing.T) {
	ctx := NewMergeContext()
	ctx.EnableProvenance()

	// Record some provenance.
	ctx.RecordProvenance("vars.name", ProvenanceEntry{File: "config.yaml", Line: 10})

	// Clone.
	cloned := ctx.Clone()

	// Verify provenance is cloned.
	assert.NotNil(t, cloned.Provenance)
	assert.NotSame(t, ctx.Provenance, cloned.Provenance)
	assert.True(t, cloned.HasProvenance("vars.name"))

	// Verify modification to original doesn't affect clone.
	ctx.RecordProvenance("vars.tags", ProvenanceEntry{File: "config.yaml", Line: 20})
	assert.True(t, ctx.HasProvenance("vars.tags"))
	assert.False(t, cloned.HasProvenance("vars.tags"))
}

func TestMergeContext_WithFile_ProvenanceShared(t *testing.T) {
	ctx := NewMergeContext()
	ctx.EnableProvenance()

	// Record provenance in parent.
	ctx.RecordProvenance("vars.name", ProvenanceEntry{File: "base.yaml", Line: 5})

	// Create child context with WithFile.
	childCtx := ctx.WithFile("child.yaml")

	// Provenance should be shared (not cloned).
	assert.Same(t, ctx.Provenance, childCtx.Provenance)
	assert.True(t, childCtx.HasProvenance("vars.name"))

	// Recording in child should be visible in parent.
	childCtx.RecordProvenance("vars.tags", ProvenanceEntry{File: "child.yaml", Line: 10})
	assert.True(t, ctx.HasProvenance("vars.tags"))
}

func TestMergeContext_GetProvenanceType(t *testing.T) {
	tests := []struct {
		name         string
		setupContext func() *MergeContext
		expected     ProvenanceType
	}{
		{
			name: "nil context returns inline",
			setupContext: func() *MergeContext {
				return nil
			},
			expected: ProvenanceTypeInline,
		},
		{
			name:         "new context (no imports) returns inline",
			setupContext: NewMergeContext,
			expected:     ProvenanceTypeInline,
		},
		{
			name: "root file (first in chain) returns inline",
			setupContext: func() *MergeContext {
				ctx := NewMergeContext()
				return ctx.WithFile("stacks/dev.yaml")
			},
			expected: ProvenanceTypeInline,
		},
		{
			name: "first level import returns import",
			setupContext: func() *MergeContext {
				ctx := NewMergeContext()
				ctx = ctx.WithFile("stacks/dev.yaml")
				return ctx.WithFile("mixins/region/us-east-2.yaml")
			},
			expected: ProvenanceTypeImport,
		},
		{
			name: "second level import returns import",
			setupContext: func() *MergeContext {
				ctx := NewMergeContext()
				ctx = ctx.WithFile("stacks/dev.yaml")
				ctx = ctx.WithFile("mixins/region/us-east-2.yaml")
				return ctx.WithFile("mixins/stage/prod.yaml")
			},
			expected: ProvenanceTypeImport,
		},
		{
			name: "deep nested import returns import",
			setupContext: func() *MergeContext {
				ctx := NewMergeContext()
				ctx = ctx.WithFile("stacks/dev.yaml")
				ctx = ctx.WithFile("catalog/base.yaml")
				ctx = ctx.WithFile("mixins/region.yaml")
				ctx = ctx.WithFile("mixins/stage.yaml")
				return ctx.WithFile("mixins/tenant.yaml")
			},
			expected: ProvenanceTypeImport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupContext()
			result := ctx.GetProvenanceType()
			assert.Equal(t, tt.expected, result, "Expected %s but got %s for context with import chain: %v",
				tt.expected, result, ctx.GetImportChainString())
		})
	}
}
