package merge

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvenanceStorage(t *testing.T) {
	storage := NewProvenanceStorage()

	require.NotNil(t, storage)
	assert.NotNil(t, storage.entries)
	assert.Equal(t, 0, storage.Size())
}

func TestProvenanceStorage_Record(t *testing.T) {
	tests := []struct {
		name    string
		storage *ProvenanceStorage
		path    string
		entry   ProvenanceEntry
		wantNil bool
	}{
		{
			name:    "record to valid storage",
			storage: NewProvenanceStorage(),
			path:    "vars.name",
			entry: ProvenanceEntry{
				File: "config.yaml",
				Line: 10,
				Type: ProvenanceTypeInline,
			},
			wantNil: false,
		},
		{
			name:    "record to nil storage (should not panic)",
			storage: nil,
			path:    "vars.name",
			entry: ProvenanceEntry{
				File: "config.yaml",
				Line: 10,
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic.
			assert.NotPanics(t, func() {
				tt.storage.Record(tt.path, tt.entry)
			})

			if !tt.wantNil {
				entries := tt.storage.Get(tt.path)
				require.Len(t, entries, 1)
				assert.Equal(t, tt.entry.File, entries[0].File)
				assert.Equal(t, tt.entry.Line, entries[0].Line)
			}
		})
	}
}

func TestProvenanceStorage_Record_MultipleEntries(t *testing.T) {
	storage := NewProvenanceStorage()

	// Record multiple entries for the same path (inheritance chain).
	entry1 := ProvenanceEntry{
		File: "base.yaml",
		Line: 5,
		Type: ProvenanceTypeImport,
	}
	entry2 := ProvenanceEntry{
		File: "override.yaml",
		Line: 10,
		Type: ProvenanceTypeOverride,
	}

	storage.Record("vars.name", entry1)
	storage.Record("vars.name", entry2)

	entries := storage.Get("vars.name")
	require.Len(t, entries, 2)

	// Verify order (base → override).
	assert.Equal(t, entry1.File, entries[0].File)
	assert.Equal(t, entry2.File, entries[1].File)
}

func TestProvenanceStorage_Get(t *testing.T) {
	tests := []struct {
		name     string
		storage  *ProvenanceStorage
		path     string
		expected []ProvenanceEntry
	}{
		{
			name: "get existing entry",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "config.yaml", Line: 10})
				return s
			}(),
			path: "vars.name",
			expected: []ProvenanceEntry{
				{File: "config.yaml", Line: 10},
			},
		},
		{
			name:     "get non-existing entry",
			storage:  NewProvenanceStorage(),
			path:     "nonexistent",
			expected: nil,
		},
		{
			name:     "get from nil storage",
			storage:  nil,
			path:     "vars.name",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.storage.Get(tt.path)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.Equal(t, len(tt.expected), len(result))
				for i, expected := range tt.expected {
					assert.Equal(t, expected.File, result[i].File)
					assert.Equal(t, expected.Line, result[i].Line)
				}
			}
		})
	}
}

func TestProvenanceStorage_Get_ReturnsCopy(t *testing.T) {
	storage := NewProvenanceStorage()
	storage.Record("vars.name", ProvenanceEntry{File: "config.yaml", Line: 10})

	// Get the entries.
	entries1 := storage.Get("vars.name")

	// Modify the returned slice.
	entries1[0].File = "modified.yaml"

	// Get again and verify original is unchanged.
	entries2 := storage.Get("vars.name")
	assert.Equal(t, "config.yaml", entries2[0].File)
}

func TestProvenanceStorage_Has(t *testing.T) {
	tests := []struct {
		name     string
		storage  *ProvenanceStorage
		path     string
		expected bool
	}{
		{
			name: "has existing entry",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "config.yaml", Line: 10})
				return s
			}(),
			path:     "vars.name",
			expected: true,
		},
		{
			name:     "has non-existing entry",
			storage:  NewProvenanceStorage(),
			path:     "nonexistent",
			expected: false,
		},
		{
			name:     "has on nil storage",
			storage:  nil,
			path:     "vars.name",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.storage.Has(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProvenanceStorage_GetPaths(t *testing.T) {
	tests := []struct {
		name     string
		storage  *ProvenanceStorage
		expected []string
	}{
		{
			name: "get paths from storage with entries",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				s.Record("vars.tags", ProvenanceEntry{File: "b.yaml", Line: 2})
				s.Record("settings.foo", ProvenanceEntry{File: "c.yaml", Line: 3})
				return s
			}(),
			expected: []string{"settings.foo", "vars.name", "vars.tags"}, // Sorted.
		},
		{
			name:     "get paths from empty storage",
			storage:  NewProvenanceStorage(),
			expected: []string{},
		},
		{
			name:     "get paths from nil storage",
			storage:  nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.storage.GetPaths()

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestProvenanceStorage_Clear(t *testing.T) {
	tests := []struct {
		name    string
		storage *ProvenanceStorage
	}{
		{
			name: "clear storage with entries",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				s.Record("vars.tags", ProvenanceEntry{File: "b.yaml", Line: 2})
				return s
			}(),
		},
		{
			name:    "clear empty storage",
			storage: NewProvenanceStorage(),
		},
		{
			name:    "clear nil storage (should not panic)",
			storage: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				tt.storage.Clear()
			})

			if tt.storage != nil {
				assert.Equal(t, 0, tt.storage.Size())
				assert.Empty(t, tt.storage.GetPaths())
			}
		})
	}
}

func TestProvenanceStorage_Clone(t *testing.T) {
	tests := []struct {
		name    string
		storage *ProvenanceStorage
	}{
		{
			name: "clone storage with entries",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				s.Record("vars.name", ProvenanceEntry{File: "b.yaml", Line: 2})
				s.Record("vars.tags", ProvenanceEntry{File: "c.yaml", Line: 3})
				return s
			}(),
		},
		{
			name:    "clone empty storage",
			storage: NewProvenanceStorage(),
		},
		{
			name:    "clone nil storage",
			storage: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := tt.storage.Clone()

			if tt.storage == nil {
				assert.Nil(t, cloned)
				return
			}

			require.NotNil(t, cloned)

			// Verify it's a different instance.
			assert.NotSame(t, tt.storage, cloned)

			// Verify same size.
			assert.Equal(t, tt.storage.Size(), cloned.Size())

			// Verify same paths.
			assert.Equal(t, tt.storage.GetPaths(), cloned.GetPaths())

			// Verify same entries for each path.
			for _, path := range tt.storage.GetPaths() {
				originalEntries := tt.storage.Get(path)
				clonedEntries := cloned.Get(path)

				require.Equal(t, len(originalEntries), len(clonedEntries))
				for i := range originalEntries {
					assert.True(t, originalEntries[i].Equals(&clonedEntries[i]))
				}
			}

			// Modify original and verify clone is unaffected.
			if tt.storage.Size() > 0 {
				tt.storage.Clear()
				assert.Equal(t, 0, tt.storage.Size())
				assert.NotEqual(t, 0, cloned.Size())
			}
		})
	}
}

func TestProvenanceStorage_Size(t *testing.T) {
	tests := []struct {
		name     string
		storage  *ProvenanceStorage
		expected int
	}{
		{
			name: "size with entries",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				s.Record("vars.tags", ProvenanceEntry{File: "b.yaml", Line: 2})
				return s
			}(),
			expected: 2,
		},
		{
			name: "size with multiple entries for same path",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				s.Record("vars.name", ProvenanceEntry{File: "b.yaml", Line: 2})
				return s
			}(),
			expected: 1, // Same path, so size is 1.
		},
		{
			name:     "size of empty storage",
			storage:  NewProvenanceStorage(),
			expected: 0,
		},
		{
			name:     "size of nil storage",
			storage:  nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.storage.Size()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProvenanceStorage_Remove(t *testing.T) {
	tests := []struct {
		name    string
		storage *ProvenanceStorage
		path    string
	}{
		{
			name: "remove existing path",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				s.Record("vars.tags", ProvenanceEntry{File: "b.yaml", Line: 2})
				return s
			}(),
			path: "vars.name",
		},
		{
			name: "remove non-existing path",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				return s
			}(),
			path: "nonexistent",
		},
		{
			name:    "remove from nil storage (should not panic)",
			storage: nil,
			path:    "vars.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalSize := 0
			if tt.storage != nil {
				originalSize = tt.storage.Size()
			}

			assert.NotPanics(t, func() {
				tt.storage.Remove(tt.path)
			})

			if tt.storage != nil {
				assert.False(t, tt.storage.Has(tt.path))

				// Size should decrease if path existed.
				if originalSize > 0 && tt.name == "remove existing path" {
					assert.Equal(t, originalSize-1, tt.storage.Size())
				}
			}
		})
	}
}

func TestProvenanceStorage_GetLatest(t *testing.T) {
	tests := []struct {
		name     string
		storage  *ProvenanceStorage
		path     string
		expected *ProvenanceEntry
	}{
		{
			name: "get latest from single entry",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				return s
			}(),
			path: "vars.name",
			expected: &ProvenanceEntry{
				File: "a.yaml",
				Line: 1,
			},
		},
		{
			name: "get latest from multiple entries",
			storage: func() *ProvenanceStorage {
				s := NewProvenanceStorage()
				s.Record("vars.name", ProvenanceEntry{File: "a.yaml", Line: 1})
				s.Record("vars.name", ProvenanceEntry{File: "b.yaml", Line: 2})
				s.Record("vars.name", ProvenanceEntry{File: "c.yaml", Line: 3})
				return s
			}(),
			path: "vars.name",
			expected: &ProvenanceEntry{
				File: "c.yaml",
				Line: 3,
			},
		},
		{
			name:     "get latest for non-existing path",
			storage:  NewProvenanceStorage(),
			path:     "nonexistent",
			expected: nil,
		},
		{
			name:     "get latest from nil storage",
			storage:  nil,
			path:     "vars.name",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.storage.GetLatest(tt.path)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.File, result.File)
				assert.Equal(t, tt.expected.Line, result.Line)
			}
		})
	}
}

func TestProvenanceStorage_GetLatest_ReturnsCopy(t *testing.T) {
	storage := NewProvenanceStorage()
	storage.Record("vars.name", ProvenanceEntry{File: "config.yaml", Line: 10})

	// Get the latest entry.
	latest := storage.GetLatest("vars.name")
	require.NotNil(t, latest)

	// Modify the returned entry.
	latest.File = "modified.yaml"

	// Get again and verify original is unchanged.
	latest2 := storage.GetLatest("vars.name")
	assert.Equal(t, "config.yaml", latest2.File)
}

func TestProvenanceStorage_ConcurrentAccess(t *testing.T) {
	storage := NewProvenanceStorage()

	// Use WaitGroup to coordinate goroutines.
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperationsPerGoroutine := 100

	// Concurrent writes.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				storage.Record("concurrent.path", ProvenanceEntry{
					File: "file.yaml",
					Line: id*1000 + j,
				})
			}
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				_ = storage.Get("concurrent.path")
				_ = storage.Has("concurrent.path")
				_ = storage.GetPaths()
			}
		}()
	}

	// Wait for all goroutines to complete.
	wg.Wait()

	// Verify storage is still valid.
	assert.True(t, storage.Has("concurrent.path"))
	entries := storage.Get("concurrent.path")
	assert.Equal(t, numGoroutines*numOperationsPerGoroutine, len(entries))
}
