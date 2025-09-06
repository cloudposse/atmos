package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNewManager(t *testing.T) {
	// Create temp dir for testing
	tempDir := t.TempDir()
	logger := log.New(os.Stderr)

	tests := []struct {
		name        string
		setup       func()
		expectError bool
	}{
		{
			name: "cache enabled",
			setup: func() {
				viper.Set("cache.enabled", true)
				viper.Set("cache.dir", tempDir)
			},
			expectError: false,
		},
		{
			name: "cache disabled",
			setup: func() {
				viper.Set("cache.enabled", false)
			},
			expectError: true,
		},
		{
			name: "with existing cache file",
			setup: func() {
				viper.Set("cache.enabled", true)
				viper.Set("cache.dir", tempDir)
				// Create a cache file
				cache := &CacheFile{
					Version: CurrentCacheVersion,
					Metadata: CacheMetadata{
						SchemaVersion: CurrentSchemaVersion,
					},
					Discovery: DiscoveryCache{
						TestCounts: map[string]TestCountEntry{
							"./...": {
								Count:     100,
								Timestamp: time.Now(),
							},
						},
					},
				}
				data, _ := yaml.Marshal(cache)
				os.WriteFile(filepath.Join(tempDir, CacheFileName), data, 0644)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test
			viper.Reset()
			tt.setup()

			manager, err := NewManager(logger)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
			}
		})
	}
}

func TestGetTestCount(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.New(os.Stderr)
	
	viper.Reset()
	viper.Set("cache.enabled", true)
	viper.Set("cache.dir", tempDir)
	viper.Set("cache.max_age", 1*time.Hour)

	manager, err := NewManager(logger)
	require.NoError(t, err)

	// Test with no cache
	count, ok := manager.GetTestCount("./...")
	assert.False(t, ok)
	assert.Equal(t, 0, count)

	// Add a cache entry
	err = manager.UpdateTestCount("./...", 150, 10)
	require.NoError(t, err)

	// Test with valid cache
	count, ok = manager.GetTestCount("./...")
	assert.True(t, ok)
	assert.Equal(t, 150, count)

	// Test with different pattern
	count, ok = manager.GetTestCount("./pkg/...")
	assert.False(t, ok)
	assert.Equal(t, 0, count)

	// Test with expired cache
	manager.file.Discovery.TestCounts["./..."] = TestCountEntry{
		Count:     150,
		Timestamp: time.Now().Add(-2 * time.Hour), // Expired
	}
	count, ok = manager.GetTestCount("./...")
	assert.False(t, ok)
	assert.Equal(t, 0, count)
}

func TestUpdateTestCount(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.New(os.Stderr)
	
	viper.Reset()
	viper.Set("cache.enabled", true)
	viper.Set("cache.dir", tempDir)

	manager, err := NewManager(logger)
	require.NoError(t, err)

	// Update test count
	err = manager.UpdateTestCount("./...", 200, 15)
	require.NoError(t, err)

	// Verify it was saved
	count, ok := manager.GetTestCount("./...")
	assert.True(t, ok)
	assert.Equal(t, 200, count)

	// Verify file was written
	cacheFile := filepath.Join(tempDir, CacheFileName)
	assert.FileExists(t, cacheFile)

	// Load the file and verify contents
	data, err := os.ReadFile(cacheFile)
	require.NoError(t, err)

	var cache CacheFile
	err = yaml.Unmarshal(data, &cache)
	require.NoError(t, err)

	assert.Equal(t, CurrentCacheVersion, cache.Version)
	assert.Equal(t, CurrentSchemaVersion, cache.Metadata.SchemaVersion)
	assert.Equal(t, 200, cache.Discovery.TestCounts["./..."].Count)
	assert.Equal(t, 15, cache.Discovery.TestCounts["./..."].PackagesScanned)
}

func TestAddRunHistory(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.New(os.Stderr)
	
	viper.Reset()
	viper.Set("cache.enabled", true)
	viper.Set("cache.dir", tempDir)

	manager, err := NewManager(logger)
	require.NoError(t, err)

	// Add run history entries
	for i := 0; i < 5; i++ {
		run := RunHistory{
			ID:        fmt.Sprintf("run_%d", i),
			Timestamp: time.Now(),
			Pattern:   "./...",
			Total:     100 + i,
			Passed:    90 + i,
			Failed:    10,
			Skipped:   0,
			DurationMs: int64(5000 + i*100),
		}
		err = manager.AddRunHistory(run)
		require.NoError(t, err)
	}

	// Verify history was saved
	assert.NotNil(t, manager.file.History)
	assert.Len(t, manager.file.History.Runs, 5)
	
	// Verify most recent is first
	assert.Equal(t, "run_4", manager.file.History.Runs[0].ID)
	assert.Equal(t, "run_3", manager.file.History.Runs[1].ID)
}

func TestCachePersistence(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.New(os.Stderr)
	
	viper.Reset()
	viper.Set("cache.enabled", true)
	viper.Set("cache.dir", tempDir)

	// Create and populate a cache
	manager1, err := NewManager(logger)
	require.NoError(t, err)

	err = manager1.UpdateTestCount("./...", 300, 20)
	require.NoError(t, err)

	run := RunHistory{
		ID:        "test_run",
		Timestamp: time.Now(),
		Pattern:   "./...",
		Total:     300,
		Passed:    295,
		Failed:    5,
		DurationMs: 10000,
	}
	err = manager1.AddRunHistory(run)
	require.NoError(t, err)

	// Create a new manager and verify it loads the existing cache
	manager2, err := NewManager(logger)
	require.NoError(t, err)

	// Verify test count was loaded
	count, ok := manager2.GetTestCount("./...")
	assert.True(t, ok)
	assert.Equal(t, 300, count)

	// Verify history was loaded
	assert.NotNil(t, manager2.file.History)
	assert.Len(t, manager2.file.History.Runs, 1)
	assert.Equal(t, "test_run", manager2.file.History.Runs[0].ID)
}

func TestClearCache(t *testing.T) {
	tempDir := t.TempDir()
	logger := log.New(os.Stderr)
	
	viper.Reset()
	viper.Set("cache.enabled", true)
	viper.Set("cache.dir", tempDir)

	manager, err := NewManager(logger)
	require.NoError(t, err)

	// Add some data
	err = manager.UpdateTestCount("./...", 400, 25)
	require.NoError(t, err)

	// Clear cache
	err = manager.Clear()
	require.NoError(t, err)

	// Verify cache is empty
	count, ok := manager.GetTestCount("./...")
	assert.False(t, ok)
	assert.Equal(t, 0, count)
}