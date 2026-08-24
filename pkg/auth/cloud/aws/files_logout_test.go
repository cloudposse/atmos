package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				// New path structure: baseDir/aws/providerName
				providerDir := filepath.Join(baseDir, "aws", "test-provider")
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
				// New path structure: baseDir/aws/providerName
				providerDir := filepath.Join(baseDir, "aws", "test-provider")
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
			// New path structure: baseDir/aws/providerName
			if !tt.wantErr {
				providerDir := filepath.Join(baseDir, "aws", tt.providerName)
				if _, err := os.Stat(providerDir); !os.IsNotExist(err) {
					t.Errorf("provider directory still exists after cleanup: %s", providerDir)
				}
			}
		})
	}
}

func TestAWSFileManager_CleanupAll(t *testing.T) {
	t.Run("cleanup with realm removes realm directory", func(t *testing.T) {
		tempDir := t.TempDir()
		realmDir := filepath.Join(tempDir, "test-realm", "aws")
		require.NoError(t, os.MkdirAll(filepath.Join(realmDir, "provider1"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(realmDir, "provider1", "credentials"), []byte("test"), 0o600))

		fm := &AWSFileManager{baseDir: tempDir, realm: "test-realm"}
		err := fm.CleanupAll()
		require.NoError(t, err)

		// Realm directory should be removed.
		_, err = os.Stat(filepath.Join(tempDir, "test-realm"))
		assert.True(t, os.IsNotExist(err), "realm directory should be removed")
		// Base directory should still exist.
		_, err = os.Stat(tempDir)
		assert.NoError(t, err, "base directory should still exist")
	})

	t.Run("cleanup without realm removes aws directory", func(t *testing.T) {
		tempDir := t.TempDir()
		awsDir := filepath.Join(tempDir, "aws")
		require.NoError(t, os.MkdirAll(filepath.Join(awsDir, "provider1"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(awsDir, "provider1", "credentials"), []byte("test"), 0o600))

		fm := &AWSFileManager{baseDir: tempDir, realm: ""}
		err := fm.CleanupAll()
		require.NoError(t, err)

		// AWS directory should be removed.
		_, err = os.Stat(awsDir)
		assert.True(t, os.IsNotExist(err), "aws directory should be removed")
		// Base directory should still exist (not accidentally deleted).
		_, err = os.Stat(tempDir)
		assert.NoError(t, err, "base directory should still exist when realm is empty")
	})

	t.Run("cleanup non-existent directory does not error", func(t *testing.T) {
		tempDir := t.TempDir()
		fm := &AWSFileManager{baseDir: tempDir, realm: "nonexistent"}
		err := fm.CleanupAll()
		require.NoError(t, err, "should not error on missing directory")
	})

	t.Run("cleanup with complex nested structure", func(t *testing.T) {
		tempDir := t.TempDir()
		realmDir := filepath.Join(tempDir, "my-realm")
		dirs := []string{
			filepath.Join("aws", "provider1", "nested", "deep"),
			filepath.Join("aws", "provider2", "another", "path"),
			filepath.Join("aws", "provider3"),
		}
		for _, dir := range dirs {
			fullDir := filepath.Join(realmDir, dir)
			require.NoError(t, os.MkdirAll(fullDir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(fullDir, "test.txt"), []byte("test"), 0o600))
		}

		fm := &AWSFileManager{baseDir: tempDir, realm: "my-realm"}
		err := fm.CleanupAll()
		require.NoError(t, err)

		// Realm directory should be removed.
		_, err = os.Stat(realmDir)
		assert.True(t, os.IsNotExist(err), "realm directory should be removed")
	})
}

func TestAWSFileManager_Cleanup_Idempotency(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	// New path structure: baseDir/aws/providerName
	providerDir := filepath.Join(tempDir, "aws", "test-provider")

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

	// Create provider directories under realm.
	realmDir := filepath.Join(tempDir, "test-realm")
	providers := []string{"provider1", "provider2"}
	for _, provider := range providers {
		providerDir := filepath.Join(realmDir, "aws", provider)
		require.NoError(t, os.MkdirAll(providerDir, 0o700))
	}

	// Create file manager with realm.
	fm := &AWSFileManager{
		baseDir: tempDir,
		realm:   "test-realm",
	}

	// First cleanup should succeed.
	require.NoError(t, fm.CleanupAll(), "First CleanupAll() failed")

	// Verify realm directory was removed.
	_, err := os.Stat(realmDir)
	assert.True(t, os.IsNotExist(err), "realm directory should be removed after first cleanup")

	// Second cleanup should also succeed (idempotent).
	require.NoError(t, fm.CleanupAll(), "Second CleanupAll() should be idempotent")
}
