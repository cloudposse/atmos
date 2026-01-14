package exec

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSetAndGetMergeContextForStack tests storing and retrieving merge contexts.
func TestSetAndGetMergeContextForStack(t *testing.T) {
	// Clear to start fresh.
	ClearMergeContexts()

	// Create a merge context.
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()
	ctx.ImportChain = []string{"file1.yaml", "file2.yaml"}

	// Store it.
	SetMergeContextForStack("my-stack", ctx)

	// Retrieve it.
	retrieved := GetMergeContextForStack("my-stack")
	require.NotNil(t, retrieved)
	assert.Equal(t, []string{"file1.yaml", "file2.yaml"}, retrieved.ImportChain)
	assert.True(t, retrieved.IsProvenanceEnabled())

	// Clean up.
	ClearMergeContexts()
}

// TestGetMergeContextForStackNotFound tests retrieving a non-existent context.
func TestGetMergeContextForStackNotFound(t *testing.T) {
	ClearMergeContexts()

	retrieved := GetMergeContextForStack("nonexistent-stack")
	assert.Nil(t, retrieved)
}

// TestClearMergeContexts tests clearing all stored merge contexts.
func TestClearMergeContexts(t *testing.T) {
	// Store multiple contexts.
	ctx1 := m.NewMergeContext()
	ctx2 := m.NewMergeContext()

	SetMergeContextForStack("stack1", ctx1)
	SetMergeContextForStack("stack2", ctx2)

	// Verify they exist.
	assert.NotNil(t, GetMergeContextForStack("stack1"))
	assert.NotNil(t, GetMergeContextForStack("stack2"))

	// Clear all.
	ClearMergeContexts()

	// Verify they're gone.
	assert.Nil(t, GetMergeContextForStack("stack1"))
	assert.Nil(t, GetMergeContextForStack("stack2"))
}

// TestGetAllMergeContexts tests retrieving all stored merge contexts.
func TestGetAllMergeContexts(t *testing.T) {
	ClearMergeContexts()

	// Store multiple contexts.
	ctx1 := m.NewMergeContext()
	ctx1.ImportChain = []string{"chain1"}
	ctx2 := m.NewMergeContext()
	ctx2.ImportChain = []string{"chain2"}

	SetMergeContextForStack("stack1", ctx1)
	SetMergeContextForStack("stack2", ctx2)

	// Get all contexts.
	allContexts := GetAllMergeContexts()
	require.Len(t, allContexts, 2)

	assert.Equal(t, []string{"chain1"}, allContexts["stack1"].ImportChain)
	assert.Equal(t, []string{"chain2"}, allContexts["stack2"].ImportChain)

	// Verify it returns a copy (modifications don't affect original).
	allContexts["stack1"] = m.NewMergeContext()
	allContexts["stack1"].ImportChain = []string{"modified"}

	// Original should be unchanged.
	original := GetMergeContextForStack("stack1")
	assert.Equal(t, []string{"chain1"}, original.ImportChain)

	// Clean up.
	ClearMergeContexts()
}

// TestGetAllMergeContextsEmpty tests retrieving when no contexts exist.
func TestGetAllMergeContextsEmpty(t *testing.T) {
	ClearMergeContexts()

	allContexts := GetAllMergeContexts()
	assert.NotNil(t, allContexts, "should return empty map, not nil")
	assert.Len(t, allContexts, 0)
}

// TestSetAndGetLastMergeContext tests the deprecated last merge context functions.
func TestSetAndGetLastMergeContext(t *testing.T) {
	ClearLastMergeContext()

	// Initially should be nil.
	assert.Nil(t, GetLastMergeContext())

	// Set a context.
	ctx := m.NewMergeContext()
	ctx.ImportChain = []string{"last-file.yaml"}

	SetLastMergeContext(ctx)

	// Retrieve it.
	retrieved := GetLastMergeContext()
	require.NotNil(t, retrieved)
	assert.Equal(t, []string{"last-file.yaml"}, retrieved.ImportChain)

	// Clear it.
	ClearLastMergeContext()
	assert.Nil(t, GetLastMergeContext())
}

// TestConcurrentMergeContextAccess tests thread-safety of merge context operations.
func TestConcurrentMergeContextAccess(t *testing.T) {
	ClearMergeContexts()
	ClearLastMergeContext()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Test concurrent SetMergeContextForStack.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := m.NewMergeContext()
			ctx.ImportChain = []string{"file.yaml"}
			SetMergeContextForStack("concurrent-stack", ctx)
		}()
	}

	// Test concurrent GetMergeContextForStack.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetMergeContextForStack("concurrent-stack")
		}()
	}

	// Test concurrent GetAllMergeContexts.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetAllMergeContexts()
		}()
	}

	// Test concurrent last merge context operations.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := m.NewMergeContext()
			SetLastMergeContext(ctx)
			GetLastMergeContext()
		}()
	}

	wg.Wait()

	// Clean up.
	ClearMergeContexts()
	ClearLastMergeContext()
}

// TestProcessImportProvenanceTracking tests the provenance tracking helper function.
func TestProcessImportProvenanceTracking(t *testing.T) {
	ClearMergeContexts()

	t.Run("nil atmosConfig returns early", func(t *testing.T) {
		result := &importFileResult{
			importRelativePathWithoutExt: "test-import",
		}
		// Should not panic with nil atmosConfig.
		processImportProvenanceTracking(nil, result, nil)
	})

	t.Run("provenance disabled returns early", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			TrackProvenance: false,
		}
		result := &importFileResult{
			importRelativePathWithoutExt: "test-import",
		}
		// Should return early without storing anything.
		processImportProvenanceTracking(atmosConfig, result, nil)
		assert.Nil(t, GetMergeContextForStack("test-import"))
	})

	t.Run("nil result merge context returns early", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			TrackProvenance: true,
		}
		result := &importFileResult{
			importRelativePathWithoutExt: "test-import",
			mergeContext:                 nil,
		}
		processImportProvenanceTracking(atmosConfig, result, nil)
		assert.Nil(t, GetMergeContextForStack("test-import"))
	})

	t.Run("provenance not enabled on context returns early", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			TrackProvenance: true,
		}
		ctx := m.NewMergeContext()
		// Don't enable provenance on context.
		result := &importFileResult{
			importRelativePathWithoutExt: "test-import",
			mergeContext:                 ctx,
		}
		processImportProvenanceTracking(atmosConfig, result, nil)
		assert.Nil(t, GetMergeContextForStack("test-import"))
	})

	t.Run("stores merge context when provenance enabled", func(t *testing.T) {
		ClearMergeContexts()

		atmosConfig := &schema.AtmosConfiguration{
			TrackProvenance: true,
		}
		ctx := m.NewMergeContext()
		ctx.EnableProvenance()
		ctx.ImportChain = []string{"nested-file.yaml"}

		result := &importFileResult{
			importRelativePathWithoutExt: "stored-import",
			mergeContext:                 ctx,
		}

		processImportProvenanceTracking(atmosConfig, result, nil)

		// Should be stored.
		stored := GetMergeContextForStack("stored-import")
		require.NotNil(t, stored)
		assert.Equal(t, []string{"nested-file.yaml"}, stored.ImportChain)

		ClearMergeContexts()
	})

	t.Run("updates parent import chain", func(t *testing.T) {
		ClearMergeContexts()

		atmosConfig := &schema.AtmosConfiguration{
			TrackProvenance: true,
		}

		childCtx := m.NewMergeContext()
		childCtx.EnableProvenance()
		childCtx.ImportChain = []string{"child-file.yaml", "nested-child.yaml"}

		parentCtx := m.NewMergeContext()
		parentCtx.EnableProvenance()
		parentCtx.ImportChain = []string{"parent-file.yaml"}

		result := &importFileResult{
			importRelativePathWithoutExt: "child-import",
			mergeContext:                 childCtx,
		}

		processImportProvenanceTracking(atmosConfig, result, parentCtx)

		// Parent should now have child's imports added.
		assert.Contains(t, parentCtx.ImportChain, "parent-file.yaml")
		assert.Contains(t, parentCtx.ImportChain, "child-file.yaml")
		assert.Contains(t, parentCtx.ImportChain, "nested-child.yaml")

		ClearMergeContexts()
	})
}

// TestUpdateParentImportChain tests the import chain update helper function.
func TestUpdateParentImportChain(t *testing.T) {
	t.Run("nil parent context does nothing", func(t *testing.T) {
		childCtx := m.NewMergeContext()
		childCtx.ImportChain = []string{"file1.yaml"}

		// Should not panic with nil parent.
		updateParentImportChain(childCtx, nil)
	})

	t.Run("adds child imports to parent", func(t *testing.T) {
		childCtx := m.NewMergeContext()
		childCtx.ImportChain = []string{"child1.yaml", "child2.yaml"}

		parentCtx := m.NewMergeContext()
		parentCtx.ImportChain = []string{"parent.yaml"}

		updateParentImportChain(childCtx, parentCtx)

		assert.Equal(t, []string{"parent.yaml", "child1.yaml", "child2.yaml"}, parentCtx.ImportChain)
	})

	t.Run("avoids duplicates", func(t *testing.T) {
		childCtx := m.NewMergeContext()
		childCtx.ImportChain = []string{"shared.yaml", "child.yaml"}

		parentCtx := m.NewMergeContext()
		parentCtx.ImportChain = []string{"parent.yaml", "shared.yaml"}

		updateParentImportChain(childCtx, parentCtx)

		// "shared.yaml" should appear only once.
		count := 0
		for _, f := range parentCtx.ImportChain {
			if f == "shared.yaml" {
				count++
			}
		}
		assert.Equal(t, 1, count, "shared.yaml should appear only once")
		assert.Contains(t, parentCtx.ImportChain, "child.yaml")
	})

	t.Run("empty child chain does nothing", func(t *testing.T) {
		childCtx := m.NewMergeContext()
		childCtx.ImportChain = []string{}

		parentCtx := m.NewMergeContext()
		parentCtx.ImportChain = []string{"parent.yaml"}

		updateParentImportChain(childCtx, parentCtx)

		assert.Equal(t, []string{"parent.yaml"}, parentCtx.ImportChain)
	})
}

// TestMergeContextOverwrite tests that setting a context overwrites the previous one.
func TestMergeContextOverwrite(t *testing.T) {
	ClearMergeContexts()

	// Set first context.
	ctx1 := m.NewMergeContext()
	ctx1.ImportChain = []string{"first.yaml"}
	SetMergeContextForStack("my-stack", ctx1)

	// Set second context with same key.
	ctx2 := m.NewMergeContext()
	ctx2.ImportChain = []string{"second.yaml"}
	SetMergeContextForStack("my-stack", ctx2)

	// Should have second context.
	retrieved := GetMergeContextForStack("my-stack")
	require.NotNil(t, retrieved)
	assert.Equal(t, []string{"second.yaml"}, retrieved.ImportChain)

	ClearMergeContexts()
}
