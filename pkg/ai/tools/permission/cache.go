package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PermissionCache stores persistent permission decisions.
type PermissionCache struct {
	filePath string
	mu       sync.RWMutex
	cache    *CacheData
}

// CacheData represents the structure of the permission cache file.
type CacheData struct {
	Permissions PermissionSet `json:"permissions"`
}

// PermissionSet contains allow/deny lists similar to Claude Code's format.
type PermissionSet struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// NewPermissionCache creates a new permission cache.
func NewPermissionCache(basePath string) (*PermissionCache, error) {
	// Default to .atmos/ai.settings.local.json if basePath is empty.
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		basePath = home
	}

	cacheDir := filepath.Join(basePath, ".atmos")
	cacheFile := filepath.Join(cacheDir, "ai.settings.local.json")

	cache := &PermissionCache{
		filePath: cacheFile,
		cache: &CacheData{
			Permissions: PermissionSet{
				Allow: []string{},
				Deny:  []string{},
			},
		},
	}

	// Create directory if it doesn't exist.
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Load existing cache if present.
	if err := cache.load(); err != nil {
		// If file doesn't exist, that's okay - we'll create it on first save.
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load cache: %w", err)
		}
	}

	return cache, nil
}

// IsAllowed checks if a tool is in the allow list.
func (c *PermissionCache) IsAllowed(toolName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, pattern := range c.cache.Permissions.Allow {
		if matchesCachePattern(toolName, pattern) {
			return true
		}
	}
	return false
}

// IsDenied checks if a tool is in the deny list.
func (c *PermissionCache) IsDenied(toolName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, pattern := range c.cache.Permissions.Deny {
		if matchesCachePattern(toolName, pattern) {
			return true
		}
	}
	return false
}

// AddAllow adds a tool to the allow list and saves.
func (c *PermissionCache) AddAllow(pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already in list.
	for _, existing := range c.cache.Permissions.Allow {
		if existing == pattern {
			return nil // Already exists
		}
	}

	c.cache.Permissions.Allow = append(c.cache.Permissions.Allow, pattern)
	return c.save()
}

// AddDeny adds a tool to the deny list and saves.
func (c *PermissionCache) AddDeny(pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already in list.
	for _, existing := range c.cache.Permissions.Deny {
		if existing == pattern {
			return nil // Already exists
		}
	}

	c.cache.Permissions.Deny = append(c.cache.Permissions.Deny, pattern)
	return c.save()
}

// RemoveAllow removes a pattern from the allow list.
func (c *PermissionCache) RemoveAllow(pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	newList := []string{}
	for _, existing := range c.cache.Permissions.Allow {
		if existing != pattern {
			newList = append(newList, existing)
		}
	}

	c.cache.Permissions.Allow = newList
	return c.save()
}

// RemoveDeny removes a pattern from the deny list.
func (c *PermissionCache) RemoveDeny(pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	newList := []string{}
	for _, existing := range c.cache.Permissions.Deny {
		if existing != pattern {
			newList = append(newList, existing)
		}
	}

	c.cache.Permissions.Deny = newList
	return c.save()
}

// Clear removes all cached permissions.
func (c *PermissionCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache.Permissions.Allow = []string{}
	c.cache.Permissions.Deny = []string{}
	return c.save()
}

// GetAllowList returns a copy of the allow list.
func (c *PermissionCache) GetAllowList() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]string, len(c.cache.Permissions.Allow))
	copy(result, c.cache.Permissions.Allow)
	return result
}

// GetDenyList returns a copy of the deny list.
func (c *PermissionCache) GetDenyList() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]string, len(c.cache.Permissions.Deny))
	copy(result, c.cache.Permissions.Deny)
	return result
}

// load reads the cache from disk.
func (c *PermissionCache) load() error {
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, c.cache)
}

// save writes the cache to disk.
func (c *PermissionCache) save() error {
	data, err := json.MarshalIndent(c.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(c.filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// matchesCachePattern checks if a tool name matches a cached pattern.
// Similar to checker.go's matchesPattern but handles both exact matches and patterns.
func matchesCachePattern(toolName, pattern string) bool {
	// Exact match.
	if toolName == pattern {
		return true
	}

	// Pattern matching format: "ToolName(param:value)"
	// Extract tool name from pattern if it has parameters.
	if idx := findPatternSeparator(pattern); idx != -1 {
		patternTool := pattern[:idx]
		// patternParam := pattern[idx+1:] // Future: Could add parameter matching.

		// Check if tool name matches.
		if toolName != patternTool {
			return false
		}

		// For now, we match tool name only.
		return true
	}

	return false
}

// findPatternSeparator finds the index of '(' in pattern.
func findPatternSeparator(pattern string) int {
	for i, ch := range pattern {
		if ch == '(' {
			return i
		}
	}
	return -1
}
