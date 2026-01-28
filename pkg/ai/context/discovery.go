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
	config     schema.AIContextSettings
	gitignore  *GitignoreFilter
	cache      *DiscoveryCache
	cacheMutex sync.RWMutex
}

// NewDiscoverer creates a new context discoverer.
func NewDiscoverer(basePath string, config schema.AIContextSettings) (*Discoverer, error) {
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

// discoverFiles performs the actual file discovery.
//
//nolint:unparam // error return is kept for future error handling (file system errors, permissions, etc.)
func (d *Discoverer) discoverFiles() (*DiscoveryResult, error) {
	if !d.config.Enabled {
		return &DiscoveryResult{Files: []*DiscoveredFile{}}, nil
	}

	if len(d.config.AutoInclude) == 0 {
		return &DiscoveryResult{Files: []*DiscoveredFile{}}, nil
	}

	result := &DiscoveryResult{
		Files: make([]*DiscoveredFile, 0),
	}

	maxSize := int64(d.config.MaxSizeMB) * MBToBytes
	seen := make(map[string]bool)

	// Process each auto_include pattern.
	for _, pattern := range d.config.AutoInclude {
		if result.TotalSize >= maxSize {
			result.Reason = fmt.Sprintf("size limit reached (%dMB)", d.config.MaxSizeMB)
			break
		}

		if len(result.Files) >= d.config.MaxFiles {
			result.Reason = fmt.Sprintf("file count limit reached (%d files)", d.config.MaxFiles)
			break
		}

		// Handle absolute vs relative patterns.
		searchPattern := pattern
		if !filepath.IsAbs(pattern) {
			searchPattern = filepath.Join(d.basePath, pattern)
		}

		// Find matching files.
		matches, err := doublestar.FilepathGlob(searchPattern)
		if err != nil {
			// Skip invalid patterns.
			continue
		}

		for _, match := range matches {
			// Check limits.
			if result.TotalSize >= maxSize {
				result.Reason = fmt.Sprintf("size limit reached (%dMB)", d.config.MaxSizeMB)
				break
			}

			if len(result.Files) >= d.config.MaxFiles {
				result.Reason = fmt.Sprintf("file count limit reached (%d files)", d.config.MaxFiles)
				break
			}

			// Skip if already seen.
			if seen[match] {
				continue
			}

			// Get file info.
			info, err := os.Stat(match)
			if err != nil {
				continue
			}

			// Skip directories.
			if info.IsDir() {
				continue
			}

			// Get relative path.
			relPath, err := filepath.Rel(d.basePath, match)
			if err != nil {
				relPath = match
			}

			// Check if excluded.
			if d.isExcluded(relPath) {
				result.FilesSkipped++
				continue
			}

			// Check gitignore.
			if d.gitignore != nil && d.gitignore.IsIgnored(relPath) {
				result.FilesSkipped++
				continue
			}

			// Check if adding this file would exceed size limit.
			if result.TotalSize+info.Size() > maxSize {
				result.Reason = fmt.Sprintf("size limit would be exceeded (%dMB)", d.config.MaxSizeMB)
				result.FilesSkipped++
				continue
			}

			// Read file content.
			content, err := os.ReadFile(match)
			if err != nil {
				result.FilesSkipped++
				continue
			}

			// Add to results.
			file := &DiscoveredFile{
				Path:         match,
				RelativePath: relPath,
				Size:         info.Size(),
				ModTime:      info.ModTime(),
				Content:      content,
			}

			result.Files = append(result.Files, file)
			result.TotalSize += info.Size()
			seen[match] = true
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
	sb.WriteString(fmt.Sprintf("Auto-discovered %d files (%s total):\n\n", len(result.Files), formatSize(result.TotalSize)))

	for _, file := range result.Files {
		sb.WriteString(fmt.Sprintf("## File: %s\n\n", file.RelativePath))
		sb.WriteString("```\n")
		sb.Write(file.Content)
		if !strings.HasSuffix(string(file.Content), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")
	}

	if result.FilesSkipped > 0 {
		sb.WriteString(fmt.Sprintf("*Note: %d files were skipped", result.FilesSkipped))
		if result.Reason != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", result.Reason))
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
