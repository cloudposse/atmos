package merge

import (
	"fmt"
	"strings"
)

// MergeContext tracks file paths and import chains during merge operations
// to provide better error messages when merge conflicts occur.
type MergeContext struct {
	// CurrentFile is the file currently being processed.
	CurrentFile string

	// ImportChain tracks the chain of imports leading to the current file.
	// The first element is the root file, the last is the current file.
	ImportChain []string

	// ParentContext is the parent merge context for nested operations.
	ParentContext *MergeContext
}

// NewMergeContext creates a new merge context.
func NewMergeContext() *MergeContext {
	return &MergeContext{
		ImportChain: []string{},
	}
}

// WithFile creates a new context for processing a specific file.
func (mc *MergeContext) WithFile(filePath string) *MergeContext {
	if mc == nil {
		mc = NewMergeContext()
	}

	newContext := &MergeContext{
		CurrentFile:   filePath,
		ImportChain:   append(mc.ImportChain, filePath),
		ParentContext: mc,
	}

	return newContext
}

// Clone creates a copy of the merge context.
func (mc *MergeContext) Clone() *MergeContext {
	if mc == nil {
		return NewMergeContext()
	}

	return &MergeContext{
		CurrentFile:   mc.CurrentFile,
		ImportChain:   append([]string{}, mc.ImportChain...),
		ParentContext: mc.ParentContext,
	}
}

// FormatError formats an error with merge context information.
func (mc *MergeContext) FormatError(err error, additionalInfo ...string) error {
	if err == nil {
		return nil
	}

	if mc == nil || (mc.CurrentFile == "" && len(mc.ImportChain) == 0) {
		// No context available, return original error unchanged
		return err
	}

	var sb strings.Builder

	// Build context message (without the original error text)
	// Add current file being processed
	if mc.CurrentFile != "" {
		sb.WriteString(fmt.Sprintf("File being processed: %s", mc.CurrentFile))
	}

	// Add import chain if available
	if len(mc.ImportChain) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("Import chain:")
		for i, file := range mc.ImportChain {
			prefix := "  "
			if i == 0 {
				prefix = "\n  → "
			} else {
				prefix = "\n    → "
			}
			sb.WriteString(fmt.Sprintf("%s%s", prefix, file))
		}
	}

	// Add any additional information
	if len(additionalInfo) > 0 {
		for _, info := range additionalInfo {
			if info != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(info)
			}
		}
	}

	// Add helpful hints for common merge errors
	errStr := err.Error()
	if strings.Contains(errStr, "cannot override two slices with different type") {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("Likely cause: A key is defined as an array in one file and as a string in another.")
		sb.WriteString("\nDebug hint: Check the files above for keys that have different types.")
		sb.WriteString("\nCommon issues:")
		sb.WriteString("\n  - vars defined as both array and string")
		sb.WriteString("\n  - settings with inconsistent types across imports")
		sb.WriteString("\n  - overrides attempting to change field types")
	} else if strings.Contains(errStr, "cannot override") {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("Likely cause: Type mismatch when merging configurations.")
		sb.WriteString("\nDebug hint: Ensure consistent types for the same keys across all files.")
	}

	// If we have context to add, wrap the error with the context message
	if sb.Len() > 0 {
		return fmt.Errorf("%s: %w", sb.String(), err)
	}
	
	// No context was added, return original error
	return err
}

// GetImportChainString returns a formatted string of the import chain.
func (mc *MergeContext) GetImportChainString() string {
	if mc == nil || len(mc.ImportChain) == 0 {
		return ""
	}

	return strings.Join(mc.ImportChain, " → ")
}

// GetDepth returns the depth of the import chain.
func (mc *MergeContext) GetDepth() int {
	if mc == nil {
		return 0
	}
	return len(mc.ImportChain)
}

// HasFile checks if a file is already in the import chain (to detect circular imports).
func (mc *MergeContext) HasFile(filePath string) bool {
	if mc == nil {
		return false
	}

	for _, file := range mc.ImportChain {
		if file == filePath {
			return true
		}
	}

	return false
}
