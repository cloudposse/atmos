package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxFiles is the default maximum number of files to include.
	DefaultMaxFiles = 100

	// DefaultMaxSizeMB is the default maximum total size in MB.
	DefaultMaxSizeMB = 10

	// DefaultCacheTTL is the default cache TTL in seconds.
	DefaultCacheTTL = 300

	// MBToBytes conversion factor.
	MBToBytes = 1024 * 1024
)

// DiscoveredFile represents a file discovered by the context loader.
type DiscoveredFile struct {
	Path         string
	RelativePath string
	Size         int64
	ModTime      time.Time
	Content      []byte
}

// DiscoveryResult contains the results of context discovery.
type DiscoveryResult struct {
	Files        []*DiscoveredFile
	TotalSize    int64
	FilesSkipped int
	Reason       string // Reason for skipping files (if any)
}

// Discoverer handles automatic context file discovery.
type Discoverer struct {
	basePath   string
	config     *schema.AIContextSettings
	gitignore  *GitignoreFilter
	cache      *DiscoveryCache
	cacheMutex sync.RWMutex
}

// NewDiscoverer creates a new context discoverer.
func NewDiscoverer(basePath string, config *schema.AIContextSettings) (*Discoverer, error) {
	// Set defaults.
	if config.MaxFiles == 0 {
		config.MaxFiles = DefaultMaxFiles
	}
	if config.MaxSizeMB == 0 {
		config.MaxSizeMB = DefaultMaxSizeMB
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = DefaultCacheTTL
	}

	// Initialize gitignore filter if enabled.
	var gitignore *GitignoreFilter
	if config.FollowGitignore {
		var err error
		gitignore, err = NewGitignoreFilter(basePath)
		if err != nil {
			// Non-fatal: continue without gitignore filtering.
			gitignore = nil
		}
	}

	// Initialize cache if enabled.
	var cache *DiscoveryCache
	if config.CacheEnabled {
		cache = NewDiscoveryCache(time.Duration(config.CacheTTL) * time.Second)
	}

	return &Discoverer{
		basePath:  basePath,
		config:    config,
		gitignore: gitignore,
		cache:     cache,
	}, nil
}

// Discover finds and loads files based on configuration.
func (d *Discoverer) Discover() (*DiscoveryResult, error) {
	// Check cache first.
	if d.cache != nil {
		d.cacheMutex.RLock()
		if cached := d.cache.Get(); cached != nil {
			d.cacheMutex.RUnlock()
			return cached, nil
		}
		d.cacheMutex.RUnlock()
	}

	// Discover files.
	result, err := d.discoverFiles()
	if err != nil {
		return nil, err
	}

	// Cache result.
	if d.cache != nil {
		d.cacheMutex.Lock()
		d.cache.Set(result)
		d.cacheMutex.Unlock()
	}

	return result, nil
}

// limitsReached checks whether the result has hit the file count or size limit,
// and sets the reason on the result if so. Returns true if a limit was reached.
func (d *Discoverer) limitsReached(result *DiscoveryResult, maxSize int64) bool {
	if result.TotalSize >= maxSize {
		result.Reason = fmt.Sprintf("size limit reached (%dMB)", d.config.MaxSizeMB)
		return true
	}

	if len(result.Files) >= d.config.MaxFiles {
		result.Reason = fmt.Sprintf("file count limit reached (%d files)", d.config.MaxFiles)
		return true
	}

	return false
}

// resolvePattern resolves a glob pattern to absolute path and returns matches.
func (d *Discoverer) resolvePattern(pattern string) []string {
	searchPattern := pattern
	if !filepath.IsAbs(pattern) {
		searchPattern = filepath.Join(d.basePath, pattern)
	}

	matches, err := doublestar.FilepathGlob(searchPattern)
	if err != nil {
		return nil
	}

	return matches
}

// processMatch processes a single file match and adds it to the result if valid.
// Returns true if the file was added, false if it was skipped.
func (d *Discoverer) processMatch(match string, maxSize int64, seen map[string]bool, result *DiscoveryResult) {
	if seen[match] {
		return
	}

	info, err := os.Stat(match)
	if err != nil || info.IsDir() {
		return
	}

	relPath, err := filepath.Rel(d.basePath, match)
	if err != nil {
		relPath = match
	}

	if d.shouldSkipFile(relPath) {
		result.FilesSkipped++
		return
	}

	if result.TotalSize+info.Size() > maxSize {
		result.Reason = fmt.Sprintf("size limit would be exceeded (%dMB)", d.config.MaxSizeMB)
		result.FilesSkipped++
		return
	}

	content, err := os.ReadFile(match)
	if err != nil {
		result.FilesSkipped++
		return
	}

	result.Files = append(result.Files, &DiscoveredFile{
		Path:         match,
		RelativePath: relPath,
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		Content:      content,
	})
	result.TotalSize += info.Size()
	seen[match] = true
}

// shouldSkipFile checks if a file should be skipped based on exclusion rules and gitignore.
func (d *Discoverer) shouldSkipFile(relPath string) bool {
	if d.isExcluded(relPath) {
		return true
	}

	if d.gitignore != nil && d.gitignore.IsIgnored(relPath) {
		return true
	}

	return false
}

// discoverFiles performs the actual file discovery.
//
//nolint:unparam // error return is kept for future error handling (file system errors, permissions, etc.)
func (d *Discoverer) discoverFiles() (*DiscoveryResult, error) {
	if !d.config.Enabled || len(d.config.AutoInclude) == 0 {
		return &DiscoveryResult{Files: []*DiscoveredFile{}}, nil
	}

	result := &DiscoveryResult{
		Files: make([]*DiscoveredFile, 0),
	}

	maxSize := int64(d.config.MaxSizeMB) * MBToBytes
	seen := make(map[string]bool)

	for _, pattern := range d.config.AutoInclude {
		if d.limitsReached(result, maxSize) {
			break
		}

		matches := d.resolvePattern(pattern)
		for _, match := range matches {
			if d.limitsReached(result, maxSize) {
				break
			}

			d.processMatch(match, maxSize, seen, result)
		}
	}

	return result, nil
}

// isExcluded checks if a file matches any exclude pattern.
func (d *Discoverer) isExcluded(relPath string) bool {
	for _, pattern := range d.config.Exclude {
		// Use doublestar for gitignore-style matching.
		matched, err := doublestar.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}

		// Also try matching against the basename.
		matched, err = doublestar.Match(pattern, filepath.Base(relPath))
		if err == nil && matched {
			return true
		}

		// Try matching with path separators normalized.
		normalizedPath := filepath.ToSlash(relPath)
		matched, err = doublestar.Match(pattern, normalizedPath)
		if err == nil && matched {
			return true
		}
	}

	return false
}

// InvalidateCache clears the discovery cache.
func (d *Discoverer) InvalidateCache() {
	if d.cache != nil {
		d.cacheMutex.Lock()
		d.cache.Invalidate()
		d.cacheMutex.Unlock()
	}
}

// FormatFilesContext formats discovered files as context string for AI.
func FormatFilesContext(result *DiscoveryResult) string {
	if len(result.Files) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("\n\n# Project Files Context\n\n")
	fmt.Fprintf(&sb, "Auto-discovered %d files (%s total):\n\n", len(result.Files), formatSize(result.TotalSize))

	for _, file := range result.Files {
		fmt.Fprintf(&sb, "## File: %s\n\n", file.RelativePath)
		sb.WriteString("```\n")
		sb.Write(file.Content)
		if !strings.HasSuffix(string(file.Content), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")
	}

	if result.FilesSkipped > 0 {
		fmt.Fprintf(&sb, "*Note: %d files were skipped", result.FilesSkipped)
		if result.Reason != "" {
			fmt.Fprintf(&sb, " (%s)", result.Reason)
		}
		sb.WriteString("*\n")
	}

	return sb.String()
}

// formatSize formats bytes as human-readable string.
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ValidateConfig validates context discovery configuration.
func ValidateConfig(config *schema.AIContextSettings) error {
	if !config.Enabled {
		return nil
	}

	if config.MaxFiles < 0 {
		return fmt.Errorf("%w: max_files cannot be negative", errUtils.ErrAIInvalidConfiguration)
	}

	if config.MaxSizeMB < 0 {
		return fmt.Errorf("%w: max_size_mb cannot be negative", errUtils.ErrAIInvalidConfiguration)
	}

	if config.CacheTTL < 0 {
		return fmt.Errorf("%w: cache_ttl_seconds cannot be negative", errUtils.ErrAIInvalidConfiguration)
	}

	// Validate patterns (basic check).
	for _, pattern := range config.AutoInclude {
		if pattern == "" {
			return fmt.Errorf("%w: empty auto_include pattern", errUtils.ErrAIInvalidConfiguration)
		}
	}

	for _, pattern := range config.Exclude {
		if pattern == "" {
			return fmt.Errorf("%w: empty exclude pattern", errUtils.ErrAIInvalidConfiguration)
		}
	}

	return nil
}
