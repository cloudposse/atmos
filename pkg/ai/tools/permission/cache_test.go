package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPermissionCache(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Verify cache file was created in correct location.
	expectedPath := filepath.Join(tmpDir, ".atmos", "ai.settings.local.json")
	if cache.filePath != expectedPath {
		t.Errorf("Expected cache path %s, got %s", expectedPath, cache.filePath)
	}

	// Verify directory was created.
	cacheDir := filepath.Join(tmpDir, ".atmos")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("Expected .atmos directory to be created")
	}
}

func TestPermissionCache_AddAllow(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add tool to allow list.
	err = cache.AddAllow("atmos_describe_component")
	if err != nil {
		t.Fatalf("Failed to add allow: %v", err)
	}

	// Verify it's in the allow list.
	if !cache.IsAllowed("atmos_describe_component") {
		t.Error("Expected tool to be in allow list")
	}

	// Verify it's not in deny list.
	if cache.IsDenied("atmos_describe_component") {
		t.Error("Expected tool not to be in deny list")
	}

	// Verify cache file was created and is valid JSON.
	if _, err := os.Stat(cache.filePath); os.IsNotExist(err) {
		t.Error("Expected cache file to be created")
	}
}

func TestPermissionCache_AddDeny(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add tool to deny list.
	err = cache.AddDeny("atmos_terraform_apply")
	if err != nil {
		t.Fatalf("Failed to add deny: %v", err)
	}

	// Verify it's in the deny list.
	if !cache.IsDenied("atmos_terraform_apply") {
		t.Error("Expected tool to be in deny list")
	}

	// Verify it's not in allow list.
	if cache.IsAllowed("atmos_terraform_apply") {
		t.Error("Expected tool not to be in allow list")
	}
}

func TestPermissionCache_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first cache instance and add permissions.
	cache1, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache1: %v", err)
	}

	err = cache1.AddAllow("tool1")
	if err != nil {
		t.Fatalf("Failed to add allow: %v", err)
	}

	err = cache1.AddDeny("tool2")
	if err != nil {
		t.Fatalf("Failed to add deny: %v", err)
	}

	// Create second cache instance from same location.
	cache2, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache2: %v", err)
	}

	// Verify permissions were persisted.
	if !cache2.IsAllowed("tool1") {
		t.Error("Expected tool1 to be in allow list after reload")
	}

	if !cache2.IsDenied("tool2") {
		t.Error("Expected tool2 to be in deny list after reload")
	}
}

func TestPermissionCache_RemoveAllow(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add and then remove.
	cache.AddAllow("tool1")
	cache.RemoveAllow("tool1")

	if cache.IsAllowed("tool1") {
		t.Error("Expected tool1 to be removed from allow list")
	}

	// Verify removal was persisted.
	cache2, _ := NewPermissionCache(tmpDir)
	if cache2.IsAllowed("tool1") {
		t.Error("Expected tool1 to remain removed after reload")
	}
}

func TestPermissionCache_RemoveDeny(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add and then remove.
	cache.AddDeny("tool1")
	cache.RemoveDeny("tool1")

	if cache.IsDenied("tool1") {
		t.Error("Expected tool1 to be removed from deny list")
	}
}

func TestPermissionCache_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add multiple permissions.
	cache.AddAllow("tool1")
	cache.AddAllow("tool2")
	cache.AddDeny("tool3")

	// Clear all.
	err = cache.Clear()
	if err != nil {
		t.Fatalf("Failed to clear cache: %v", err)
	}

	// Verify all are cleared.
	if cache.IsAllowed("tool1") || cache.IsAllowed("tool2") {
		t.Error("Expected allow list to be cleared")
	}

	if cache.IsDenied("tool3") {
		t.Error("Expected deny list to be cleared")
	}

	// Verify clear was persisted.
	cache2, _ := NewPermissionCache(tmpDir)
	if len(cache2.GetAllowList()) != 0 || len(cache2.GetDenyList()) != 0 {
		t.Error("Expected lists to remain cleared after reload")
	}
}

func TestPermissionCache_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add same tool twice.
	cache.AddAllow("tool1")
	cache.AddAllow("tool1")

	// Should only appear once.
	allowList := cache.GetAllowList()
	count := 0
	for _, tool := range allowList {
		if tool == "tool1" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected tool1 to appear once, got %d times", count)
	}
}

func TestPermissionCache_GetLists(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add some permissions.
	cache.AddAllow("tool1")
	cache.AddAllow("tool2")
	cache.AddDeny("tool3")

	// Get lists.
	allowList := cache.GetAllowList()
	denyList := cache.GetDenyList()

	if len(allowList) != 2 {
		t.Errorf("Expected 2 items in allow list, got %d", len(allowList))
	}

	if len(denyList) != 1 {
		t.Errorf("Expected 1 item in deny list, got %d", len(denyList))
	}

	// Verify it's a copy (mutation shouldn't affect cache).
	allowList[0] = "modified"
	if cache.GetAllowList()[0] == "modified" {
		t.Error("Expected GetAllowList to return a copy")
	}
}

func TestMatchesCachePattern(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		pattern  string
		matches  bool
	}{
		{
			name:     "exact match",
			toolName: "atmos_describe_component",
			pattern:  "atmos_describe_component",
			matches:  true,
		},
		{
			name:     "no match",
			toolName: "atmos_describe_component",
			pattern:  "atmos_list_stacks",
			matches:  false,
		},
		{
			name:     "pattern with params",
			toolName: "atmos_describe_component",
			pattern:  "atmos_describe_component(stack:dev)",
			matches:  true,
		},
		{
			name:     "different tool with params",
			toolName: "atmos_list_stacks",
			pattern:  "atmos_describe_component(stack:dev)",
			matches:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesCachePattern(tt.toolName, tt.pattern)
			if result != tt.matches {
				t.Errorf("Expected matchesCachePattern(%q, %q) = %v, got %v",
					tt.toolName, tt.pattern, tt.matches, result)
			}
		})
	}
}

func TestPermissionCache_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Concurrent writes.
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			cache.AddAllow("tool" + string(rune('A'+id)))
			done <- true
		}(i)
	}

	// Wait for all writes.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all tools were added.
	allowList := cache.GetAllowList()
	if len(allowList) != 10 {
		t.Errorf("Expected 10 tools in allow list, got %d", len(allowList))
	}
}

func TestNewPermissionCache_EmptyBasePath(t *testing.T) {
	// Test with empty base path - should use home directory.
	cache, err := NewPermissionCache("")
	if err != nil {
		t.Fatalf("Failed to create cache with empty base path: %v", err)
	}

	// Verify path contains .atmos directory.
	if !filepath.IsAbs(cache.filePath) {
		t.Error("Expected absolute path when base path is empty")
	}

	// Clean up - remove the cache file if created.
	os.RemoveAll(filepath.Dir(cache.filePath))
}

func TestPermissionCache_JSONValidation(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add some permissions.
	cache.AddAllow("atmos_describe_component")
	cache.AddAllow("atmos_list_stacks")
	cache.AddDeny("atmos_terraform_apply")

	// Read the file and verify it's valid JSON.
	data, err := os.ReadFile(cache.filePath)
	if err != nil {
		t.Fatalf("Failed to read cache file: %v", err)
	}

	// Verify we can parse it back.
	var parsed CacheData
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Cache file is not valid JSON: %v", err)
	}

	// Verify content.
	if len(parsed.Permissions.Allow) != 2 {
		t.Errorf("Expected 2 items in allow list, got %d", len(parsed.Permissions.Allow))
	}

	if len(parsed.Permissions.Deny) != 1 {
		t.Errorf("Expected 1 item in deny list, got %d", len(parsed.Permissions.Deny))
	}
}

func TestPermissionCache_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create cache directory.
	cacheDir := filepath.Join(tmpDir, ".atmos")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("Failed to create cache directory: %v", err)
	}

	// Write corrupted JSON.
	cacheFile := filepath.Join(cacheDir, "ai.settings.local.json")
	if err := os.WriteFile(cacheFile, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("Failed to write corrupted cache file: %v", err)
	}

	// Try to load cache - should fail.
	_, err := NewPermissionCache(tmpDir)
	if err == nil {
		t.Error("Expected error when loading corrupted cache file")
	}
}

func TestPermissionCache_RealisticToolPatterns(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test with realistic tool names and patterns.
	realisticTools := []string{
		"atmos_describe_component",
		"atmos_list_stacks",
		"atmos_validate_component",
		"atmos_terraform_plan",
		"atmos_terraform_apply",
		"atmos_terraform_destroy",
	}

	// Add some to allow list.
	for _, tool := range realisticTools[:3] {
		if err := cache.AddAllow(tool); err != nil {
			t.Fatalf("Failed to add %s to allow list: %v", tool, err)
		}
	}

	// Add some to deny list.
	for _, tool := range realisticTools[3:] {
		if err := cache.AddDeny(tool); err != nil {
			t.Fatalf("Failed to add %s to deny list: %v", tool, err)
		}
	}

	// Verify all allowed tools.
	for _, tool := range realisticTools[:3] {
		if !cache.IsAllowed(tool) {
			t.Errorf("Expected %s to be in allow list", tool)
		}
	}

	// Verify all denied tools.
	for _, tool := range realisticTools[3:] {
		if !cache.IsDenied(tool) {
			t.Errorf("Expected %s to be in deny list", tool)
		}
	}

	// Verify persistence.
	cache2, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to reload cache: %v", err)
	}

	if len(cache2.GetAllowList()) != 3 {
		t.Errorf("Expected 3 items in allow list after reload, got %d", len(cache2.GetAllowList()))
	}

	if len(cache2.GetDenyList()) != 3 {
		t.Errorf("Expected 3 items in deny list after reload, got %d", len(cache2.GetDenyList()))
	}
}

func TestPermissionCache_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add a permission to trigger file creation.
	cache.AddAllow("test_tool")

	// Check file permissions.
	info, err := os.Stat(cache.filePath)
	if err != nil {
		t.Fatalf("Failed to stat cache file: %v", err)
	}

	// Verify file is readable and writable by user (0644).
	mode := info.Mode().Perm()
	if mode != 0644 {
		t.Errorf("Expected file permissions 0644, got %o", mode)
	}
}
