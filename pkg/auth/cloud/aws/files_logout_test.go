package aws

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAWSFileManager_Cleanup(t *testing.T) {
	tests := []struct {
		name         string
		setupFunc    func(t *testing.T, baseDir string) string
		providerName string
		wantErr      bool
	}{
		{
			name: "cleanup existing provider directory",
			setupFunc: func(t *testing.T, baseDir string) string {
				providerDir := filepath.Join(baseDir, "test-provider")
				if err := os.MkdirAll(providerDir, 0o700); err != nil {
					t.Fatalf("failed to create provider directory: %v", err)
				}
				// Create test files.
				credFile := filepath.Join(providerDir, "credentials")
				if err := os.WriteFile(credFile, []byte("test"), 0o600); err != nil {
					t.Fatalf("failed to create credentials file: %v", err)
				}
				return baseDir
			},
			providerName: "test-provider",
			wantErr:      false,
		},
		{
			name: "cleanup non-existent provider directory",
			setupFunc: func(t *testing.T, baseDir string) string {
				return baseDir
			},
			providerName: "non-existent",
			wantErr:      false, // Should not error on missing directory
		},
		{
			name: "cleanup with nested directories",
			setupFunc: func(t *testing.T, baseDir string) string {
				providerDir := filepath.Join(baseDir, "test-provider")
				nestedDir := filepath.Join(providerDir, "nested", "deep")
				if err := os.MkdirAll(nestedDir, 0o700); err != nil {
					t.Fatalf("failed to create nested directory: %v", err)
				}
				// Create test files in nested directory.
				testFile := filepath.Join(nestedDir, "test.txt")
				if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return baseDir
			},
			providerName: "test-provider",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test.
			tempDir := t.TempDir()
			baseDir := tt.setupFunc(t, tempDir)

			// Create file manager with test base directory.
			fm := &AWSFileManager{
				baseDir: baseDir,
			}

			// Perform cleanup.
			err := fm.Cleanup(tt.providerName)

			// Check error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("Cleanup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify directory was removed.
			if !tt.wantErr {
				providerDir := filepath.Join(baseDir, tt.providerName)
				if _, err := os.Stat(providerDir); !os.IsNotExist(err) {
					t.Errorf("provider directory still exists after cleanup: %s", providerDir)
				}
			}
		})
	}
}

func TestAWSFileManager_CleanupAll(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, baseDir string) string
		wantErr   bool
	}{
		{
			name: "cleanup all providers",
			setupFunc: func(t *testing.T, baseDir string) string {
				// Create multiple provider directories.
				providers := []string{"provider1", "provider2", "provider3"}
				for _, provider := range providers {
					providerDir := filepath.Join(baseDir, provider)
					if err := os.MkdirAll(providerDir, 0o700); err != nil {
						t.Fatalf("failed to create provider directory: %v", err)
					}
					// Create test files.
					credFile := filepath.Join(providerDir, "credentials")
					if err := os.WriteFile(credFile, []byte("test"), 0o600); err != nil {
						t.Fatalf("failed to create credentials file: %v", err)
					}
				}
				return baseDir
			},
			wantErr: false,
		},
		{
			name: "cleanup empty base directory",
			setupFunc: func(t *testing.T, baseDir string) string {
				// Create empty base directory.
				if err := os.MkdirAll(baseDir, 0o700); err != nil {
					t.Fatalf("failed to create base directory: %v", err)
				}
				return baseDir
			},
			wantErr: false,
		},
		{
			name: "cleanup non-existent base directory",
			setupFunc: func(t *testing.T, baseDir string) string {
				// Don't create the base directory.
				return baseDir
			},
			wantErr: false, // Should not error on missing directory
		},
		{
			name: "cleanup with complex nested structure",
			setupFunc: func(t *testing.T, baseDir string) string {
				// Create complex directory structure.
				dirs := []string{
					"provider1/nested/deep",
					"provider2/another/path",
					"provider3",
				}
				for _, dir := range dirs {
					fullDir := filepath.Join(baseDir, dir)
					if err := os.MkdirAll(fullDir, 0o700); err != nil {
						t.Fatalf("failed to create directory: %v", err)
					}
					// Create test files.
					testFile := filepath.Join(fullDir, "test.txt")
					if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
						t.Fatalf("failed to create test file: %v", err)
					}
				}
				return baseDir
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test.
			tempDir := t.TempDir()
			baseDir := tt.setupFunc(t, tempDir)

			// Create file manager with test base directory.
			fm := &AWSFileManager{
				baseDir: baseDir,
			}

			// Perform cleanup.
			err := fm.CleanupAll()

			// Check error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("CleanupAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify base directory was removed.
			if !tt.wantErr {
				if _, err := os.Stat(baseDir); !os.IsNotExist(err) {
					t.Errorf("base directory still exists after cleanup: %s", baseDir)
				}
			}
		})
	}
}

func TestAWSFileManager_Cleanup_Idempotency(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	providerDir := filepath.Join(tempDir, "test-provider")

	// Create provider directory with files.
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatalf("failed to create provider directory: %v", err)
	}
	credFile := filepath.Join(providerDir, "credentials")
	if err := os.WriteFile(credFile, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to create credentials file: %v", err)
	}

	// Create file manager.
	fm := &AWSFileManager{
		baseDir: tempDir,
	}

	// First cleanup should succeed.
	if err := fm.Cleanup("test-provider"); err != nil {
		t.Errorf("First Cleanup() failed: %v", err)
	}

	// Verify directory was removed.
	if _, err := os.Stat(providerDir); !os.IsNotExist(err) {
		t.Errorf("provider directory still exists after first cleanup")
	}

	// Second cleanup should also succeed (idempotent).
	if err := fm.Cleanup("test-provider"); err != nil {
		t.Errorf("Second Cleanup() failed (should be idempotent): %v", err)
	}

	// Third cleanup should also succeed.
	if err := fm.Cleanup("test-provider"); err != nil {
		t.Errorf("Third Cleanup() failed (should be idempotent): %v", err)
	}
}

func TestAWSFileManager_CleanupAll_IdempotencyTest(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()

	// Create multiple provider directories.
	providers := []string{"provider1", "provider2"}
	for _, provider := range providers {
		providerDir := filepath.Join(tempDir, provider)
		if err := os.MkdirAll(providerDir, 0o700); err != nil {
			t.Fatalf("failed to create provider directory: %v", err)
		}
	}

	// Create file manager.
	fm := &AWSFileManager{
		baseDir: tempDir,
	}

	// First cleanup should succeed.
	if err := fm.CleanupAll(); err != nil {
		t.Errorf("First CleanupAll() failed: %v", err)
	}

	// Verify base directory was removed.
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("base directory still exists after first cleanup")
	}

	// Second cleanup should also succeed (idempotent).
	if err := fm.CleanupAll(); err != nil {
		t.Errorf("Second CleanupAll() failed (should be idempotent): %v", err)
	}
}
