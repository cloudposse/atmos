package workdir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper functions.

func TestNormalizeURI_Extended(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "https URL",
			input:    "HTTPS://github.com/cloudposse/terraform-aws-vpc",
			expected: "https://github.com/cloudposse/terraform-aws-vpc",
		},
		{
			name:     "http URL",
			input:    "HTTP://example.com/path",
			expected: "http://example.com/path",
		},
		{
			name:     "git URL",
			input:    "GIT::https://github.com/cloudposse/terraform-aws-vpc",
			expected: "git::https://github.com/cloudposse/terraform-aws-vpc",
		},
		{
			name:     "file URL",
			input:    "FILE:///path/to/file",
			expected: "file:///path/to/file",
		},
		{
			name:     "preserves case after scheme",
			input:    "https://github.com/CloudPosse/Terraform-AWS-VPC",
			expected: "https://github.com/CloudPosse/Terraform-AWS-VPC",
		},
		{
			name:     "trims whitespace",
			input:    "  https://github.com/cloudposse/terraform-aws-vpc  ",
			expected: "https://github.com/cloudposse/terraform-aws-vpc",
		},
		{
			name:     "no scheme",
			input:    "github.com/cloudposse/terraform-aws-vpc",
			expected: "github.com/cloudposse/terraform-aws-vpc",
		},
		{
			name:     "s3 URL",
			input:    "S3::https://s3.amazonaws.com/bucket/path",
			expected: "s3::https://s3.amazonaws.com/bucket/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSemver_Extended(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid semver.
		{"v1.0.0", "v1.0.0", true},
		{"1.0.0", "1.0.0", true},
		{"v1.2.3", "v1.2.3", true},
		{"v0.1.0", "v0.1.0", true},
		{"v10.20.30", "v10.20.30", true},
		{"v1.0.0-rc1", "v1.0.0-rc1", true},
		{"v1.0.0-alpha", "v1.0.0-alpha", true},
		{"v1.0.0-beta.1", "v1.0.0-beta.1", true},
		{"1.2.3-rc1", "1.2.3-rc1", true},

		// Invalid semver.
		{"v1.0", "v1.0", false},
		{"v1", "v1", false},
		{"main", "main", false},
		{"develop", "develop", false},
		{"feature/branch", "feature/branch", false},
		{"abc123", "abc123", false},
		{"", "", false},
		{"v", "v", false},
		{"1.2.3.4", "1.2.3.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSemver(tt.input)
			assert.Equal(t, tt.expected, result, "isSemver(%q)", tt.input)
		})
	}
}

func TestIsCommitSHA_Extended(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid SHA.
		{"40 char lowercase", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0", true},
		{"40 char mixed case", "A1B2C3D4E5F6A7B8C9D0E1F2A3B4C5D6E7F8A9B0", true},
		{"7 char short SHA", "abc1234", true},
		{"8 char short SHA", "abcd1234", true},
		{"12 char SHA", "abcdef123456", true},

		// Invalid SHA.
		{"6 char (too short)", "abc123", false},
		{"41 char (too long)", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b01", false},
		{"contains non-hex", "ghijkl1234", false},
		{"branch name", "main", false},
		{"semver", "v1.0.0", false},
		{"empty", "", false},
		{"7 char with non-hex", "abc123g", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCommitSHA(tt.input)
			assert.Equal(t, tt.expected, result, "isCommitSHA(%q)", tt.input)
		})
	}
}

func TestExtractRefFromURI_Extended(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ref in query string",
			input:    "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0",
			expected: "v1.0.0",
		},
		{
			name:     "ref with other params before",
			input:    "github.com/repo?depth=1&ref=main",
			expected: "main",
		},
		{
			name:     "ref with other params after",
			input:    "github.com/repo?ref=develop&depth=1",
			expected: "develop",
		},
		{
			name:     "ref with fragment",
			input:    "github.com/repo?ref=v1.0.0#readme",
			expected: "v1.0.0",
		},
		{
			name:     "no ref parameter",
			input:    "github.com/cloudposse/terraform-aws-vpc",
			expected: "",
		},
		{
			name:     "ref at end of URL",
			input:    "github.com/repo?ref=feature/branch-name",
			expected: "feature/branch-name",
		},
		{
			name:     "empty URL",
			input:    "",
			expected: "",
		},
		{
			name:     "commit SHA ref",
			input:    "github.com/repo?ref=abc1234567890def",
			expected: "abc1234567890def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRefFromURI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultCache_GenerateKey(t *testing.T) {
	cache := NewDefaultCache()

	tests := []struct {
		name    string
		source  *SourceConfig
		checkFn func(t *testing.T, key string)
	}{
		{
			name: "same input produces same key",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "v1.0.0",
			},
			checkFn: func(t *testing.T, key string) {
				// Generate again and verify determinism.
				key2 := cache.GenerateKey(&SourceConfig{
					URI:     "github.com/cloudposse/terraform-aws-vpc",
					Version: "v1.0.0",
				})
				assert.Equal(t, key, key2, "same input should produce same key")
			},
		},
		{
			name: "different version produces different key",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "v1.0.0",
			},
			checkFn: func(t *testing.T, key string) {
				key2 := cache.GenerateKey(&SourceConfig{
					URI:     "github.com/cloudposse/terraform-aws-vpc",
					Version: "v2.0.0",
				})
				assert.NotEqual(t, key, key2, "different version should produce different key")
			},
		},
		{
			name: "different URI produces different key",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "v1.0.0",
			},
			checkFn: func(t *testing.T, key string) {
				key2 := cache.GenerateKey(&SourceConfig{
					URI:     "github.com/cloudposse/terraform-aws-s3-bucket",
					Version: "v1.0.0",
				})
				assert.NotEqual(t, key, key2, "different URI should produce different key")
			},
		},
		{
			name: "key is valid hex string",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "v1.0.0",
			},
			checkFn: func(t *testing.T, key string) {
				assert.Len(t, key, 64, "SHA256 hex should be 64 chars")
				assert.Regexp(t, "^[0-9a-f]+$", key, "key should be lowercase hex")
			},
		},
		{
			name: "source without version",
			source: &SourceConfig{
				URI: "github.com/cloudposse/terraform-aws-vpc",
			},
			checkFn: func(t *testing.T, key string) {
				assert.NotEmpty(t, key)
				// Verify different from versioned.
				key2 := cache.GenerateKey(&SourceConfig{
					URI:     "github.com/cloudposse/terraform-aws-vpc",
					Version: "v1.0.0",
				})
				assert.NotEqual(t, key, key2, "with/without version should differ")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := cache.GenerateKey(tt.source)
			tt.checkFn(t, key)
		})
	}
}

func TestDefaultCache_GetPolicy(t *testing.T) {
	cache := NewDefaultCache()

	tests := []struct {
		name     string
		source   *SourceConfig
		expected CachePolicy
	}{
		// Permanent policy cases.
		{
			name:     "semver version",
			source:   &SourceConfig{URI: "github.com/repo", Version: "v1.0.0"},
			expected: CachePolicyPermanent,
		},
		{
			name:     "semver without v prefix",
			source:   &SourceConfig{URI: "github.com/repo", Version: "1.2.3"},
			expected: CachePolicyPermanent,
		},
		{
			name:     "semver with prerelease",
			source:   &SourceConfig{URI: "github.com/repo", Version: "v1.0.0-rc1"},
			expected: CachePolicyPermanent,
		},
		{
			name:     "commit SHA 40 chars",
			source:   &SourceConfig{URI: "github.com/repo", Version: "abc1234567890def1234567890abc1234567890"},
			expected: CachePolicyPermanent,
		},
		{
			name:     "short commit SHA version",
			source:   &SourceConfig{URI: "github.com/repo", Version: "abc1234"},
			expected: CachePolicyPermanent,
		},
		{
			name:     "semver ref in URI",
			source:   &SourceConfig{URI: "github.com/repo?ref=v1.0.0"},
			expected: CachePolicyPermanent,
		},
		{
			name:     "commit SHA ref in URI",
			source:   &SourceConfig{URI: "github.com/repo?ref=abc1234"},
			expected: CachePolicyPermanent,
		},

		// TTL policy cases.
		{
			name:     "branch version",
			source:   &SourceConfig{URI: "github.com/repo", Version: "main"},
			expected: CachePolicyTTL,
		},
		{
			name:     "develop branch",
			source:   &SourceConfig{URI: "github.com/repo", Version: "develop"},
			expected: CachePolicyTTL,
		},
		{
			name:     "feature branch",
			source:   &SourceConfig{URI: "github.com/repo", Version: "feature/my-feature"},
			expected: CachePolicyTTL,
		},
		{
			name:     "branch ref in URI",
			source:   &SourceConfig{URI: "github.com/repo?ref=main"},
			expected: CachePolicyTTL,
		},
		{
			name:     "no version or ref",
			source:   &SourceConfig{URI: "github.com/repo"},
			expected: CachePolicyTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := cache.GetPolicy(tt.source)
			assert.Equal(t, tt.expected, policy)
		})
	}
}

// Test cache core operations.

func TestDefaultCache_NewDefaultCache(t *testing.T) {
	cache := NewDefaultCache()
	assert.NotNil(t, cache)
	assert.NotNil(t, cache.index)
	assert.NotNil(t, cache.index.Entries)
}

func TestDefaultCache_PutAndGet(t *testing.T) {
	// Create temp directories for cache and source.
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))

	// Create a test file in source.
	testFile := filepath.Join(sourceDir, "main.tf")
	require.NoError(t, os.WriteFile(testFile, []byte("# test terraform"), 0o644))

	// Create cache with explicit base path.
	cache := &DefaultCache{
		basePath: filepath.Join(tmpDir, "cache"),
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	require.NoError(t, os.MkdirAll(cache.basePath, 0o755))

	key := "testkey1234567890abcdef"
	entry := &CacheEntry{
		Key:            key,
		URI:            "github.com/test/repo",
		Version:        "v1.0.0",
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
		TTL:            0,
		ContentHash:    "abc123",
	}

	// Put entry.
	err := cache.Put(key, sourceDir, entry)
	require.NoError(t, err)

	// Get entry.
	retrieved, err := cache.Get(key)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, key, retrieved.Key)
	assert.Equal(t, "github.com/test/repo", retrieved.URI)
	assert.Equal(t, "v1.0.0", retrieved.Version)
	assert.NotEmpty(t, retrieved.Path)

	// Verify content was copied.
	copiedFile := filepath.Join(retrieved.Path, "main.tf")
	data, err := os.ReadFile(copiedFile)
	require.NoError(t, err)
	assert.Equal(t, "# test terraform", string(data))
}

func TestDefaultCache_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &DefaultCache{
		basePath: filepath.Join(tmpDir, "cache"),
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	require.NoError(t, os.MkdirAll(cache.basePath, 0o755))

	entry, err := cache.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, entry)
}

func TestDefaultCache_Get_Expired(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &DefaultCache{
		basePath: filepath.Join(tmpDir, "cache"),
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	require.NoError(t, os.MkdirAll(cache.basePath, 0o755))

	// Add entry with expired TTL.
	key := "expiredkey1234567890abcd"
	cache.index.Entries[key] = &CacheEntry{
		Key:       key,
		CreatedAt: time.Now().Add(-2 * time.Hour), // 2 hours ago.
		TTL:       1 * time.Hour,                  // 1 hour TTL.
	}

	entry, err := cache.Get(key)
	require.NoError(t, err)
	assert.Nil(t, entry, "expired entry should return nil")
}

func TestDefaultCache_Get_MissingContent(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &DefaultCache{
		basePath: filepath.Join(tmpDir, "cache"),
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	require.NoError(t, os.MkdirAll(cache.basePath, 0o755))

	// Add entry pointing to non-existent path.
	key := "missingcontent1234567890"
	cache.index.Entries[key] = &CacheEntry{
		Key:       key,
		Path:      filepath.Join(tmpDir, "nonexistent"),
		CreatedAt: time.Now(),
		TTL:       0, // Permanent.
	}

	entry, err := cache.Get(key)
	require.NoError(t, err)
	assert.Nil(t, entry, "entry with missing content should return nil")
}

func TestDefaultCache_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	// Create cache with basePath already set and sync.Once "completed".
	cache := &DefaultCache{
		basePath: cacheDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	// "Complete" the sync.Once by calling Do with empty func.
	cache.basePathOnce.Do(func() {})

	// Create content directory.
	key := "removetest1234567890abcd"
	contentPath := cache.contentPath(key)
	require.NoError(t, os.MkdirAll(contentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contentPath, "test.txt"), []byte("test"), 0o644))

	// Add to index.
	cache.index.Entries[key] = &CacheEntry{
		Key:  key,
		Path: contentPath,
	}

	// Remove.
	err := cache.Remove(key)
	require.NoError(t, err)

	// Verify removed from index.
	_, exists := cache.index.Entries[key]
	assert.False(t, exists, "entry should be removed from index")

	// Verify content directory removed.
	_, err = os.Stat(contentPath)
	assert.True(t, os.IsNotExist(err), "content directory should be removed")
}

func TestDefaultCache_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	cache := &DefaultCache{
		basePath: cacheDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	// "Complete" the sync.Once by calling Do with empty func.
	cache.basePathOnce.Do(func() {})

	// Create some content.
	require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "blobs", "ab", "abcd1234"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "test.txt"), []byte("test"), 0o644))
	cache.index.Entries["testkey"] = &CacheEntry{Key: "testkey"}

	// Clear.
	err := cache.Clear()
	require.NoError(t, err)

	// Verify cache directory removed.
	_, err = os.Stat(cacheDir)
	assert.True(t, os.IsNotExist(err), "cache directory should be removed")

	// Verify index cleared.
	assert.Empty(t, cache.index.Entries)
	assert.Empty(t, cache.basePath)
}

func TestDefaultCache_BasePath(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	cache := &DefaultCache{
		basePath: cacheDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	// "Complete" the sync.Once by calling Do with empty func.
	cache.basePathOnce.Do(func() {})

	path, err := cache.BasePath()
	require.NoError(t, err)
	assert.Equal(t, cacheDir, path)
}

func TestDefaultCache_Path(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	cache := &DefaultCache{
		basePath: cacheDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}
	// "Complete" the sync.Once by calling Do with empty func.
	cache.basePathOnce.Do(func() {})

	// Create content directory.
	key := "pathtest12345678901234567890"
	contentPath := cache.contentPath(key)
	require.NoError(t, os.MkdirAll(contentPath, 0o755))

	// Add to index.
	cache.index.Entries[key] = &CacheEntry{
		Key:       key,
		Path:      contentPath,
		CreatedAt: time.Now(),
		TTL:       0,
	}

	// Get path.
	path := cache.Path(key)
	assert.Equal(t, contentPath, path)

	// Non-existent key.
	path = cache.Path("nonexistent")
	assert.Empty(t, path)
}

func TestDefaultCache_contentPath(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	cache := &DefaultCache{
		basePath: cacheDir,
	}

	key := "abcdef1234567890"
	path := cache.contentPath(key)

	// Should use first 2 chars for sharding.
	expected := filepath.Join(cacheDir, "blobs", "ab", "abcdef1234567890", "content")
	assert.Equal(t, expected, path)
}

func TestDefaultCache_loadAndSaveIndex(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &DefaultCache{
		basePath: tmpDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}

	// Add entries.
	cache.index.Entries["key1"] = &CacheEntry{
		Key:       "key1",
		URI:       "github.com/test/repo1",
		CreatedAt: time.Now(),
	}
	cache.index.Entries["key2"] = &CacheEntry{
		Key:       "key2",
		URI:       "github.com/test/repo2",
		CreatedAt: time.Now(),
	}

	// Save index.
	err := cache.saveIndex()
	require.NoError(t, err)

	// Verify file exists.
	indexPath := filepath.Join(tmpDir, "index.json")
	_, err = os.Stat(indexPath)
	require.NoError(t, err)

	// Create new cache and load.
	cache2 := &DefaultCache{
		basePath: tmpDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}

	err = cache2.loadIndex()
	require.NoError(t, err)
	assert.Len(t, cache2.index.Entries, 2)
	assert.Contains(t, cache2.index.Entries, "key1")
	assert.Contains(t, cache2.index.Entries, "key2")
}

func TestDefaultCache_loadIndex_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &DefaultCache{
		basePath: tmpDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}

	err := cache.loadIndex()
	assert.Error(t, err, "should error when index file doesn't exist")
}

func TestDefaultCache_loadIndex_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.json")
	require.NoError(t, os.WriteFile(indexPath, []byte("invalid json"), 0o644))

	cache := &DefaultCache{
		basePath: tmpDir,
		index:    &cacheIndex{Entries: make(map[string]*CacheEntry)},
	}

	err := cache.loadIndex()
	assert.Error(t, err, "should error on invalid JSON")
}

// Test copy functions.

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	// Create source structure.
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0o644))

	// Copy.
	err := copyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify structure.
	data1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(data1))

	data2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(data2))
}

func TestCopyDir_NonExistent(t *testing.T) {
	err := copyDir("/nonexistent/path", t.TempDir())
	assert.Error(t, err)
}

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "test.txt")
	dstFile := filepath.Join(dstDir, "test.txt")

	// Create source file.
	content := "test content for copy"
	require.NoError(t, os.WriteFile(srcFile, []byte(content), 0o644))

	// Copy.
	err := copyFile(srcFile, dstFile)
	require.NoError(t, err)

	// Verify.
	data, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestCopyFile_Large(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "large.bin")
	dstFile := filepath.Join(dstDir, "large.bin")

	// Create large file (larger than buffer size).
	content := make([]byte, 100*1024) // 100KB.
	for i := range content {
		content[i] = byte(i % 256)
	}
	require.NoError(t, os.WriteFile(srcFile, content, 0o644))

	// Copy.
	err := copyFile(srcFile, dstFile)
	require.NoError(t, err)

	// Verify.
	data, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestCopySymlink(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a file and symlink to it.
	targetFile := filepath.Join(srcDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("target content"), 0o644))

	symlinkPath := filepath.Join(srcDir, "link.txt")
	require.NoError(t, os.Symlink("target.txt", symlinkPath))

	// Copy symlink.
	dstSymlink := filepath.Join(dstDir, "link.txt")
	err := copySymlink(symlinkPath, dstSymlink)
	require.NoError(t, err)

	// Verify it's a symlink pointing to same target.
	target, err := os.Readlink(dstSymlink)
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
}

func TestCopyDir_WithSymlink(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	// Create file and symlink.
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644))
	require.NoError(t, os.Symlink("file.txt", filepath.Join(srcDir, "link.txt")))

	// Copy directory.
	err := copyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify symlink preserved.
	target, err := os.Readlink(filepath.Join(dstDir, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, "file.txt", target)
}

// Test index JSON structure.

func TestCacheIndex_JSONFormat(t *testing.T) {
	index := &cacheIndex{
		Entries: map[string]*CacheEntry{
			"key1": {
				Key:       "key1",
				URI:       "github.com/test/repo",
				Version:   "v1.0.0",
				Path:      "/cache/blobs/ke/key1/content",
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := json.MarshalIndent(index, "", "  ")
	require.NoError(t, err)

	// Verify structure.
	var parsed cacheIndex
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed.Entries, 1)
	assert.Equal(t, "key1", parsed.Entries["key1"].Key)
}
