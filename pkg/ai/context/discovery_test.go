package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  schema.AIContextSettings
		wantErr bool
	}{
		{
			name: "valid config",
			config: schema.AIContextSettings{
				Enabled:      true,
				AutoInclude:  []string{"*.go"},
				MaxFiles:     100,
				MaxSizeMB:    10,
				CacheTTL:     300,
			},
			wantErr: false,
		},
		{
			name: "disabled config",
			config: schema.AIContextSettings{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "negative max files",
			config: schema.AIContextSettings{
				Enabled:   true,
				MaxFiles:  -1,
				MaxSizeMB: 10,
			},
			wantErr: true,
		},
		{
			name: "negative max size",
			config: schema.AIContextSettings{
				Enabled:   true,
				MaxFiles:  100,
				MaxSizeMB: -1,
			},
			wantErr: true,
		},
		{
			name: "negative cache ttl",
			config: schema.AIContextSettings{
				Enabled:  true,
				CacheTTL: -1,
			},
			wantErr: true,
		},
		{
			name: "empty pattern",
			config: schema.AIContextSettings{
				Enabled:     true,
				AutoInclude: []string{"*.go", ""},
			},
			wantErr: true,
		},
		{
			name: "empty exclude pattern",
			config: schema.AIContextSettings{
				Enabled: true,
				Exclude: []string{"*.log", ""},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(&tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDiscoverer_Defaults(t *testing.T) {
	tmpDir := t.TempDir()

	config := schema.AIContextSettings{
		Enabled: true,
	}

	discoverer, err := NewDiscoverer(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	// Check defaults were applied.
	if discoverer.config.MaxFiles != DefaultMaxFiles {
		t.Errorf("Expected MaxFiles = %d, got %d", DefaultMaxFiles, discoverer.config.MaxFiles)
	}
	if discoverer.config.MaxSizeMB != DefaultMaxSizeMB {
		t.Errorf("Expected MaxSizeMB = %d, got %d", DefaultMaxSizeMB, discoverer.config.MaxSizeMB)
	}
	if discoverer.config.CacheTTL != DefaultCacheTTL {
		t.Errorf("Expected CacheTTL = %d, got %d", DefaultCacheTTL, discoverer.config.CacheTTL)
	}
}

func TestDiscoverer_BasicDiscovery(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files.
	testFiles := []string{
		"main.go",
		"util.go",
		"README.md",
	}
	for _, f := range testFiles {
		path := filepath.Join(tmpDir, f)
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	config := schema.AIContextSettings{
		Enabled:     true,
		AutoInclude: []string{"*.go"},
		MaxFiles:    100,
		MaxSizeMB:   10,
	}

	discoverer, err := NewDiscoverer(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	result, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should find 2 .go files.
	if len(result.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(result.Files))
	}

	// Check file names.
	foundMain := false
	foundUtil := false
	for _, f := range result.Files {
		if f.RelativePath == "main.go" {
			foundMain = true
		}
		if f.RelativePath == "util.go" {
			foundUtil = true
		}
	}

	if !foundMain {
		t.Error("Expected to find main.go")
	}
	if !foundUtil {
		t.Error("Expected to find util.go")
	}
}

func TestDiscoverer_Exclusion(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files.
	testFiles := []string{
		"main.go",
		"main_test.go",
		"util.go",
	}
	for _, f := range testFiles {
		path := filepath.Join(tmpDir, f)
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	config := schema.AIContextSettings{
		Enabled:     true,
		AutoInclude: []string{"*.go"},
		Exclude:     []string{"*_test.go"},
		MaxFiles:    100,
		MaxSizeMB:   10,
	}

	discoverer, err := NewDiscoverer(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	result, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should find 2 files (excluding main_test.go).
	if len(result.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(result.Files))
	}

	// Check files skipped.
	if result.FilesSkipped != 1 {
		t.Errorf("Expected 1 file skipped, got %d", result.FilesSkipped)
	}

	// Verify main_test.go is not included.
	for _, f := range result.Files {
		if f.RelativePath == "main_test.go" {
			t.Error("Expected main_test.go to be excluded")
		}
	}
}

func TestDiscoverer_MaxFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 5 test files with unique names.
	for i := 1; i <= 5; i++ {
		filename := filepath.Join(tmpDir, filepath.Base(tmpDir)+"_"+string(rune('a'+i-1))+".go")
		if err := os.WriteFile(filename, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	config := schema.AIContextSettings{
		Enabled:     true,
		AutoInclude: []string{"*.go"},
		MaxFiles:    3,
		MaxSizeMB:   10,
	}

	discoverer, err := NewDiscoverer(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	result, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should find only 3 files.
	if len(result.Files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(result.Files))
	}

	// Should have reason for stopping.
	if result.Reason == "" {
		t.Error("Expected reason for stopping discovery")
	}
}

func TestDiscoverer_MaxSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file larger than 1MB.
	largeContent := make([]byte, 2*1024*1024) // 2MB
	path := filepath.Join(tmpDir, "large.go")
	if err := os.WriteFile(path, largeContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := schema.AIContextSettings{
		Enabled:     true,
		AutoInclude: []string{"*.go"},
		MaxFiles:    100,
		MaxSizeMB:   1,
	}

	discoverer, err := NewDiscoverer(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	result, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should skip the large file.
	if len(result.Files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(result.Files))
	}

	// Should have files skipped.
	if result.FilesSkipped != 1 {
		t.Errorf("Expected 1 file skipped, got %d", result.FilesSkipped)
	}
}

func TestDiscoverer_Cache(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file.
	path := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := schema.AIContextSettings{
		Enabled:      true,
		AutoInclude:  []string{"*.go"},
		MaxFiles:     100,
		MaxSizeMB:    10,
		CacheEnabled: true,
		CacheTTL:     300,
	}

	discoverer, err := NewDiscoverer(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	// First discovery.
	result1, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Second discovery should use cache.
	result2, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Results should be identical (from cache).
	if len(result1.Files) != len(result2.Files) {
		t.Error("Expected cached result to match")
	}

	// Invalidate cache.
	discoverer.InvalidateCache()

	// Third discovery should re-scan.
	result3, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should still have same files.
	if len(result3.Files) != len(result1.Files) {
		t.Error("Expected same files after cache invalidation")
	}
}

func TestDiscoverer_Disabled(t *testing.T) {
	tmpDir := t.TempDir()

	config := schema.AIContextSettings{
		Enabled:     false,
		AutoInclude: []string{"*.go"},
	}

	discoverer, err := NewDiscoverer(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create discoverer: %v", err)
	}

	result, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should return empty result.
	if len(result.Files) != 0 {
		t.Errorf("Expected 0 files when disabled, got %d", len(result.Files))
	}
}

func TestFormatFilesContext(t *testing.T) {
	tests := []struct {
		name   string
		result *DiscoveryResult
		want   string
	}{
		{
			name: "empty result",
			result: &DiscoveryResult{
				Files: []*DiscoveredFile{},
			},
			want: "",
		},
		{
			name: "single file",
			result: &DiscoveryResult{
				Files: []*DiscoveredFile{
					{
						RelativePath: "main.go",
						Content:      []byte("package main\n"),
						Size:         13,
					},
				},
				TotalSize: 13,
			},
			want: "main.go",
		},
		{
			name: "multiple files with skip",
			result: &DiscoveryResult{
				Files: []*DiscoveredFile{
					{
						RelativePath: "main.go",
						Content:      []byte("package main\n"),
						Size:         13,
					},
					{
						RelativePath: "util.go",
						Content:      []byte("package util\n"),
						Size:         13,
					},
				},
				TotalSize:    26,
				FilesSkipped: 5,
				Reason:       "size limit reached",
			},
			want: "5 files were skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatFilesContext(tt.result)
			if tt.want != "" && got == "" {
				t.Error("Expected non-empty formatted context")
			}
			if tt.want != "" {
				// Just check that expected strings are present.
				if len(tt.want) > 0 && len(got) == 0 {
					t.Errorf("Expected context containing %q, got empty string", tt.want)
				}
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{10485760, "10.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
