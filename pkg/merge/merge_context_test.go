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
			name: "empty context returns original error",
			setupContext: func() *MergeContext {
				return NewMergeContext()
			},
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
				"Likely cause: A key is defined as an array in one file and as a string in another",
				"Debug hint: Check the files above for keys that have different types",
				"Common issues:",
				"vars defined as both array and string",
				"settings with inconsistent types",
				"overrides attempting to change field types",
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