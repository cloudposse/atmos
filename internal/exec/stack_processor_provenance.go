package exec

import (
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// The mergeContexts stores MergeContexts keyed by stack file path when provenance tracking is enabled.
	// This is used to capture provenance data for the describe component command.
	mergeContexts   = make(map[string]*m.MergeContext)
	mergeContextsMu sync.RWMutex

	// Deprecated: Use SetMergeContextForStack/GetMergeContextForStack instead.
	lastMergeContext   *m.MergeContext
	lastMergeContextMu sync.RWMutex
)

// SetMergeContextForStack stores the merge context for a specific stack file.
func SetMergeContextForStack(stackFile string, ctx *m.MergeContext) {
	defer perf.Track(nil, "exec.SetMergeContextForStack")()

	mergeContextsMu.Lock()
	defer mergeContextsMu.Unlock()
	mergeContexts[stackFile] = ctx
}

// GetMergeContextForStack retrieves the merge context for a specific stack file.
func GetMergeContextForStack(stackFile string) *m.MergeContext {
	defer perf.Track(nil, "exec.GetMergeContextForStack")()

	mergeContextsMu.RLock()
	defer mergeContextsMu.RUnlock()
	return mergeContexts[stackFile]
}

// ClearMergeContexts clears all stored merge contexts.
func ClearMergeContexts() {
	defer perf.Track(nil, "exec.ClearMergeContexts")()

	mergeContextsMu.Lock()
	defer mergeContextsMu.Unlock()
	mergeContexts = make(map[string]*m.MergeContext)
}

// GetAllMergeContexts returns all stored merge contexts.
// Returns a map of stack file paths to their merge contexts.
func GetAllMergeContexts() map[string]*m.MergeContext {
	defer perf.Track(nil, "exec.GetAllMergeContexts")()

	mergeContextsMu.RLock()
	defer mergeContextsMu.RUnlock()

	// Return a copy to prevent external modifications.
	result := make(map[string]*m.MergeContext, len(mergeContexts))
	for k, v := range mergeContexts {
		result[k] = v
	}
	return result
}

// SetLastMergeContext stores the merge context for later retrieval.
// Deprecated: Use SetMergeContextForStack instead.
func SetLastMergeContext(ctx *m.MergeContext) {
	defer perf.Track(nil, "exec.SetLastMergeContext")()

	lastMergeContextMu.Lock()
	defer lastMergeContextMu.Unlock()
	lastMergeContext = ctx
}

// GetLastMergeContext retrieves the last stored merge context.
// Deprecated: Use GetMergeContextForStack instead.
func GetLastMergeContext() *m.MergeContext {
	defer perf.Track(nil, "exec.GetLastMergeContext")()

	lastMergeContextMu.RLock()
	defer lastMergeContextMu.RUnlock()
	return lastMergeContext
}

// ClearLastMergeContext clears the stored merge context.
// Deprecated: Use ClearMergeContexts instead.
func ClearLastMergeContext() {
	defer perf.Track(nil, "exec.ClearLastMergeContext")()

	lastMergeContextMu.Lock()
	defer lastMergeContextMu.Unlock()
	lastMergeContext = nil
}

// processImportProvenanceTracking handles storing merge context and updating import chains.
// It stores the merge context for imported files and adds imported files to the parent's import chain.
func processImportProvenanceTracking(
	atmosConfig *schema.AtmosConfiguration,
	result *importFileResult,
	mergeContext *m.MergeContext,
) {
	defer perf.Track(atmosConfig, "exec.processImportProvenanceTracking")()

	if atmosConfig == nil || !atmosConfig.TrackProvenance {
		return
	}

	if result.mergeContext == nil {
		log.Trace("Import has nil merge context", "import", result.importRelativePathWithoutExt)
		return
	}

	if !result.mergeContext.IsProvenanceEnabled() {
		log.Trace("Import has merge context but provenance not enabled", "import", result.importRelativePathWithoutExt)
		return
	}

	log.Trace("Storing merge context for import", "import", result.importRelativePathWithoutExt, "chain_length", len(result.mergeContext.ImportChain))
	SetMergeContextForStack(result.importRelativePathWithoutExt, result.mergeContext)

	// Add imported files to parent merge context's import chain.
	updateParentImportChain(result.mergeContext, mergeContext)
}

// updateParentImportChain adds imported files from the child's import chain to the parent's chain.
func updateParentImportChain(childContext, parentContext *m.MergeContext) {
	defer perf.Track(nil, "exec.updateParentImportChain")()

	if parentContext == nil {
		return
	}

	for i, importedFile := range childContext.ImportChain {
		if u.SliceContainsString(parentContext.ImportChain, importedFile) {
			continue
		}
		parentContext.ImportChain = append(parentContext.ImportChain, importedFile)
		if i == 0 {
			log.Trace("Added import to parent import chain", "file", importedFile)
		} else {
			log.Trace("Added nested import to parent import chain", "file", importedFile)
		}
	}
}
