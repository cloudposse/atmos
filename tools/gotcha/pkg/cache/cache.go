package cache

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	pkgErrors "github.com/cloudposse/atmos/tools/gotcha/pkg/errors"
)

const (
	// DefaultCacheDir is the default directory for cache files.
	DefaultCacheDir = ".gotcha"
	// CacheFileName is the name of the cache file.
	CacheFileName = "cache.yaml"
	// DefaultMaxAge is the default maximum age for cache entries.
	DefaultMaxAge = 24 * time.Hour
	// CurrentCacheVersion is the current cache format version.
	CurrentCacheVersion = "1.0"
)

// Manager handles cache operations.
type Manager struct {
	mu     sync.RWMutex
	file   *CacheFile
	path   string
	logger *log.Logger
}

// NewManager creates a new cache manager.
func NewManager(logger *log.Logger) (*Manager, error) {
	// Use viper for configuration with sensible defaults
	viper.SetDefault("cache.dir", DefaultCacheDir)
	viper.SetDefault("cache.enabled", true)
	viper.SetDefault("cache.max_age", DefaultMaxAge)

	// Check if cache is explicitly disabled
	if viper.IsSet("cache.enabled") && !viper.GetBool("cache.enabled") {
		return nil, pkgErrors.ErrCacheDisabled
	}

	cacheDir := viper.GetString("cache.dir")
	cachePath := filepath.Join(cacheDir, CacheFileName)

	m := &Manager{
		path:   cachePath,
		logger: logger,
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		logger.Warn("Failed to create cache directory", "dir", cacheDir, "error", err)
		return m, nil // Return manager anyway, it will work without persistence
	}

	// Try to load existing cache
	if err := m.load(); err != nil {
		// Check if it's a file not found error
		if os.IsNotExist(err) {
			logger.Info("No existing cache file found, will create new cache", "path", cachePath)
		} else {
			logger.Debug("Could not load cache file", "path", cachePath, "error", err)
		}
		// Initialize new cache
		m.file = m.newCacheFile()
		logger.Debug("Initialized new cache structure")
	} else {
		logger.Info("Loaded existing cache file", "path", cachePath)
	}

	return m, nil
}

// newCacheFile creates a new cache file structure.
func (m *Manager) newCacheFile() *CacheFile {
	return &CacheFile{
		Version: CurrentCacheVersion,
		Metadata: CacheMetadata{
			LastUpdated:   time.Now(),
			GotchaVersion: viper.GetString("version"), // Assuming version is set in viper
		},
		Discovery: DiscoveryCache{
			TestCounts:     make(map[string]TestCountEntry),
			TestLists:      make(map[string]TestListEntry),
			PackageDetails: make(map[string]PackageDetail),
		},
	}
}

// load reads the cache file from disk.
func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		// Return the original error so we can check if it's file not found
		return err
	}

	var cache CacheFile
	if err := yaml.Unmarshal(data, &cache); err != nil {
		return fmt.Errorf("failed to unmarshal cache: %w", err)
	}

	// No schema version check needed - we only have one version

	m.file = &cache
	return nil
}

// save writes the cache file to disk.
// Note: This method assumes the mutex is already held by the caller.
func (m *Manager) saveUnlocked() error {
	if m.file == nil {
		m.logger.Warn("Cannot save cache: no cache file initialized")
		return pkgErrors.ErrNoCacheToSave
	}

	// Update metadata
	m.file.Metadata.LastUpdated = time.Now()

	// Create YAML encoder with 2-space indentation
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2) // Use 2 spaces for indentation

	if err := encoder.Encode(m.file); err != nil {
		m.logger.Error("Failed to marshal cache", "error", err)
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	data := buf.Bytes()

	// Log the path we're trying to write to
	m.logger.Debug("Saving cache file", "path", m.path, "size", len(data))

	// Write atomically by writing to temp file first
	tempPath := m.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		m.logger.Error("Failed to write temp cache file", "path", tempPath, "error", err)
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	// Rename temp file to actual cache file
	if err := os.Rename(tempPath, m.path); err != nil {
		// Clean up temp file on error
		os.Remove(tempPath)
		m.logger.Error("Failed to rename cache file", "from", tempPath, "to", m.path, "error", err)
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	m.logger.Info("Cache file saved successfully", "path", m.path)
	return nil
}

// save writes the cache file to disk with proper locking.
func (m *Manager) save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveUnlocked()
}

// isValidEntry checks if a cache entry is still valid based on age and go.mod time.
func (m *Manager) isValidEntry(timestamp, goModTime time.Time) bool {
	// Check if cache entry is still valid
	maxAge := viper.GetDuration("cache.max_age")
	if time.Since(timestamp) > maxAge {
		m.logger.Debug("Cache entry expired", "age", time.Since(timestamp))
		return false
	}

	// Check if go.mod has been modified since cache entry
	currentGoModTime := getGoModTime()
	if !currentGoModTime.IsZero() && currentGoModTime.After(goModTime) {
		m.logger.Debug("go.mod modified since cache entry")
		return false
	}

	return true
}

// GetTestCount retrieves the cached test count for a pattern.
func (m *Manager) GetTestCount(pattern string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.file == nil || m.file.Discovery.TestCounts == nil {
		return 0, false
	}

	entry, exists := m.file.Discovery.TestCounts[pattern]
	if !exists {
		return 0, false
	}

	// Check if cache entry is still valid
	maxAge := viper.GetDuration("cache.max_age")
	if time.Since(entry.Timestamp) > maxAge {
		m.logger.Debug("Cache entry expired", "pattern", pattern, "age", time.Since(entry.Timestamp))
		return 0, false
	}

	// Check if go.mod has been modified since cache entry
	if goModTime := getGoModTime(); !goModTime.IsZero() && goModTime.After(entry.GoModTime) {
		m.logger.Debug("go.mod modified since cache entry", "pattern", pattern)
		return 0, false
	}

	m.logger.Debug("Using cached test count", "pattern", pattern, "count", entry.Count)
	return entry.Count, true
}

// UpdateTestCount updates the cached test count for a pattern.
func (m *Manager) UpdateTestCount(pattern string, count int, packagesScanned int) error {
	m.mu.Lock()
	if m.file == nil {
		m.file = m.newCacheFile()
	}
	if m.file.Discovery.TestCounts == nil {
		m.file.Discovery.TestCounts = make(map[string]TestCountEntry)
	}

	goModTime := getGoModTime()
	m.file.Discovery.TestCounts[pattern] = TestCountEntry{
		Count:           count,
		Timestamp:       time.Now(),
		GoModTime:       goModTime,
		PackagesScanned: packagesScanned,
	}

	// Save to disk (while still holding the lock)
	if err := m.saveUnlocked(); err != nil {
		m.logger.Warn("Failed to save cache", "error", err)
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	m.logger.Debug("Updated test count cache", "pattern", pattern, "count", count)
	return nil
}

// UpdateTestList updates the cached test list for a pattern.
// This should be used instead of UpdateTestCount when we have unfiltered test results.
func (m *Manager) UpdateTestList(pattern string, testNames []string, packagesScanned int) error {
	m.mu.Lock()

	if m.file == nil {
		m.file = m.newCacheFile()
	}

	if m.file.Discovery.TestLists == nil {
		m.file.Discovery.TestLists = make(map[string]TestListEntry)
	}

	goModTime := getGoModTime()
	m.file.Discovery.TestLists[pattern] = TestListEntry{
		Tests:           testNames,
		Timestamp:       time.Now(),
		GoModTime:       goModTime,
		PackagesScanned: packagesScanned,
	}

	// Also update the count for backward compatibility
	if m.file.Discovery.TestCounts == nil {
		m.file.Discovery.TestCounts = make(map[string]TestCountEntry)
	}
	m.file.Discovery.TestCounts[pattern] = TestCountEntry{
		Count:           len(testNames),
		Timestamp:       time.Now(),
		GoModTime:       goModTime,
		PackagesScanned: packagesScanned,
	}

	// Save to disk (while still holding the lock)
	if err := m.saveUnlocked(); err != nil {
		m.logger.Warn("Failed to save cache", "error", err)
		m.mu.Unlock()
		return err
	}

	m.mu.Unlock()
	m.logger.Debug("Updated test list cache", "pattern", pattern, "count", len(testNames))
	return nil
}

// UpdatePackageDetails updates the cached details for multiple packages.
func (m *Manager) UpdatePackageDetails(packages map[string]PackageDetail) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.file == nil {
		return fmt.Errorf("cache not initialized")
	}

	// Update package details
	for pkg, details := range packages {
		m.file.Discovery.PackageDetails[pkg] = details
	}

	// Save to disk
	if err := m.saveUnlocked(); err != nil {
		m.logger.Warn("Failed to save package details cache", "error", err)
		return err
	}

	m.logger.Debug("Updated package details cache", "packages", len(packages))
	return nil
}

// GetTestCountForFilter gets estimated test count for a pattern with an optional filter.
func (m *Manager) GetTestCountForFilter(pattern, filter string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.file == nil || m.file.Discovery.TestLists == nil {
		// Fallback to old count-based cache
		return m.getTestCountLegacy(pattern)
	}

	entry, exists := m.file.Discovery.TestLists[pattern]
	if !exists || !m.isValidEntry(entry.Timestamp, entry.GoModTime) {
		// Fallback to old count-based cache
		return m.getTestCountLegacy(pattern)
	}

	// If no filter, return total count
	if filter == "" {
		return len(entry.Tests), true
	}

	// Apply filter to test names
	count := 0
	for _, testName := range entry.Tests {
		// Simple substring match for now (could use regex later)
		if strings.Contains(testName, filter) {
			count++
		}
	}

	return count, true
}

// getTestCountLegacy gets test count from the old count-based cache.
func (m *Manager) getTestCountLegacy(pattern string) (int, bool) {
	if m.file == nil || m.file.Discovery.TestCounts == nil {
		return 0, false
	}

	entry, exists := m.file.Discovery.TestCounts[pattern]
	if !exists || !m.isValidEntry(entry.Timestamp, entry.GoModTime) {
		return 0, false
	}

	return entry.Count, true
}

// AddRunHistory adds a test run to the history.
func (m *Manager) AddRunHistory(run RunHistory) error {
	m.mu.Lock()
	if m.file == nil {
		m.file = m.newCacheFile()
	}
	if m.file.History == nil {
		m.file.History = &HistoryCache{
			MaxEntries: 100,
		}
	}

	// Add new run to the beginning
	m.file.History.Runs = append([]RunHistory{run}, m.file.History.Runs...)

	// Trim to max entries
	if len(m.file.History.Runs) > m.file.History.MaxEntries {
		m.file.History.Runs = m.file.History.Runs[:m.file.History.MaxEntries]
	}

	// Save to disk (while still holding the lock)
	err := m.saveUnlocked()
	m.mu.Unlock()
	return err
}

// UpdatePerformanceMetrics updates performance metrics for tests and packages.
func (m *Manager) UpdatePerformanceMetrics(slowestTests []TestPerformance, slowestPackages []PackagePerformance) error {
	m.mu.Lock()
	if m.file == nil {
		m.file = m.newCacheFile()
	}
	if m.file.Performance == nil {
		m.file.Performance = &PerformanceCache{}
	}

	m.file.Performance.SlowestTests = slowestTests
	m.file.Performance.SlowestPackages = slowestPackages
	m.mu.Unlock()

	return m.save()
}

// Clear removes all cached data.
func (m *Manager) Clear() error {
	m.mu.Lock()
	m.file = m.newCacheFile()
	m.mu.Unlock()

	return m.save()
}

// getGoModTime returns the modification time of go.mod file.
func getGoModTime() time.Time {
	info, err := os.Stat("go.mod")
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
