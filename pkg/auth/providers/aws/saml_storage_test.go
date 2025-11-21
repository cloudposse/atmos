package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSAMLProvider_setupBrowserStorageDir(t *testing.T) {
	tests := []struct {
		name          string
		providerName  string
		setup         func(t *testing.T) string // Returns home directory.
		expectedError bool
		verifySymlink bool
		verifyXDGDir  bool
	}{
		{
			name:          "creates directory and symlink successfully",
			providerName:  "test-saml",
			expectedError: false,
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			verifySymlink: true,
			verifyXDGDir:  true,
		},
		{
			name:          "handles existing correct symlink",
			providerName:  "existing-saml",
			expectedError: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()

				// Create XDG directory structure.
				xdgCacheDir := filepath.Join(homeDir, ".cache", "atmos", "aws-saml", "existing-saml")
				require.NoError(t, os.MkdirAll(xdgCacheDir, 0o700))

				// Create ~/.aws directory.
				awsDir := filepath.Join(homeDir, ".aws")
				require.NoError(t, os.MkdirAll(awsDir, 0o700))

				// Create correct symlink.
				saml2awsPath := filepath.Join(awsDir, "saml2aws")
				require.NoError(t, os.Symlink(xdgCacheDir, saml2awsPath))

				return homeDir
			},
			verifySymlink: true,
			verifyXDGDir:  true,
		},
		{
			name:          "replaces incorrect symlink",
			providerName:  "replace-saml",
			expectedError: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()

				// Create ~/.aws directory with wrong symlink.
				awsDir := filepath.Join(homeDir, ".aws")
				require.NoError(t, os.MkdirAll(awsDir, 0o700))

				saml2awsPath := filepath.Join(awsDir, "saml2aws")
				wrongTarget := filepath.Join(homeDir, "wrong-target")
				require.NoError(t, os.MkdirAll(wrongTarget, 0o700))
				require.NoError(t, os.Symlink(wrongTarget, saml2awsPath))

				return homeDir
			},
			verifySymlink: true,
			verifyXDGDir:  true,
		},
		{
			name:          "replaces existing directory with symlink",
			providerName:  "dir-replace-saml",
			expectedError: false,
			setup: func(t *testing.T) string {
				homeDir := t.TempDir()

				// Create ~/.aws/saml2aws as regular directory.
				awsDir := filepath.Join(homeDir, ".aws")
				saml2awsPath := filepath.Join(awsDir, "saml2aws")
				require.NoError(t, os.MkdirAll(saml2awsPath, 0o700))

				// Create a file inside to verify removal works.
				testFile := filepath.Join(saml2awsPath, "test.txt")
				require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o600))

				return homeDir
			},
			verifySymlink: true,
			verifyXDGDir:  true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			homeDir := tc.setup(t)

			// Override environment variables for cross-platform compatibility.
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(homeDir, ".cache"))

			// Create provider.
			p := &samlProvider{
				name:   tc.providerName,
				config: &schema.Provider{},
			}

			// Execute setup.
			err := p.setupBrowserStorageDir()

			// Verify error expectation.
			if tc.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify XDG directory was created.
			if tc.verifyXDGDir {
				xdgCacheDir := filepath.Join(homeDir, ".cache", "atmos", "aws-saml", tc.providerName)
				info, err := os.Stat(xdgCacheDir)
				require.NoError(t, err, "XDG cache directory should exist")
				assert.True(t, info.IsDir(), "XDG cache path should be a directory")
				assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "XDG directory should have 0700 permissions")
			}

			// Verify symlink was created correctly.
			if tc.verifySymlink {
				awsDir := filepath.Join(homeDir, ".aws")
				saml2awsPath := filepath.Join(awsDir, "saml2aws")

				info, err := os.Lstat(saml2awsPath)
				require.NoError(t, err, "Symlink should exist")
				assert.True(t, info.Mode()&os.ModeSymlink != 0, "Path should be a symlink")

				target, err := os.Readlink(saml2awsPath)
				require.NoError(t, err, "Should be able to read symlink target")

				expectedTarget := filepath.Join(homeDir, ".cache", "atmos", "aws-saml", tc.providerName)
				assert.Equal(t, expectedTarget, target, "Symlink should point to correct XDG directory")
			}
		})
	}
}

func TestSAMLProvider_ensureStorageSymlink(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, symlinkPath, targetPath string)
		expectedError bool
	}{
		{
			name: "creates new symlink",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				// Ensure parent directory exists.
				require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
				// Target directory exists.
				require.NoError(t, os.MkdirAll(targetPath, 0o700))
			},
			expectedError: false,
		},
		{
			name: "idempotent when correct symlink exists",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
				require.NoError(t, os.MkdirAll(targetPath, 0o700))
				// Create correct symlink.
				require.NoError(t, os.Symlink(targetPath, symlinkPath))
			},
			expectedError: false,
		},
		{
			name: "replaces wrong symlink",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
				require.NoError(t, os.MkdirAll(targetPath, 0o700))
				// Create wrong symlink.
				wrongTarget := filepath.Join(filepath.Dir(targetPath), "wrong")
				require.NoError(t, os.MkdirAll(wrongTarget, 0o700))
				require.NoError(t, os.Symlink(wrongTarget, symlinkPath))
			},
			expectedError: false,
		},
		{
			name: "replaces regular directory",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(targetPath, 0o700))
				// Create regular directory at symlink path.
				require.NoError(t, os.MkdirAll(symlinkPath, 0o700))
				// Add file inside to verify removal.
				testFile := filepath.Join(symlinkPath, "test.txt")
				require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o600))
			},
			expectedError: false,
		},
		{
			name: "replaces regular file",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
				require.NoError(t, os.MkdirAll(targetPath, 0o700))
				// Create regular file at symlink path.
				require.NoError(t, os.WriteFile(symlinkPath, []byte("test"), 0o600))
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			homeDir := t.TempDir()
			symlinkPath := filepath.Join(homeDir, "aws", "saml2aws")
			targetPath := filepath.Join(homeDir, "cache", "saml-target")

			tc.setup(t, symlinkPath, targetPath)

			p := &samlProvider{
				name:   "test",
				config: &schema.Provider{},
			}

			err := p.ensureStorageSymlink(symlinkPath, targetPath)

			if tc.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify symlink was created correctly.
			info, err := os.Lstat(symlinkPath)
			require.NoError(t, err, "Symlink should exist")
			assert.True(t, info.Mode()&os.ModeSymlink != 0, "Path should be a symlink")

			target, err := os.Readlink(symlinkPath)
			require.NoError(t, err, "Should be able to read symlink")
			assert.Equal(t, targetPath, target, "Symlink should point to correct target")
		})
	}
}

func TestSAMLProvider_isCorrectSymlink(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T, symlinkPath, targetPath string)
		expectedResult bool
	}{
		{
			name: "returns true for correct symlink",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
				require.NoError(t, os.MkdirAll(targetPath, 0o700))
				require.NoError(t, os.Symlink(targetPath, symlinkPath))
			},
			expectedResult: true,
		},
		{
			name: "returns false for wrong symlink target",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
				wrongTarget := filepath.Join(filepath.Dir(targetPath), "wrong")
				require.NoError(t, os.MkdirAll(wrongTarget, 0o700))
				require.NoError(t, os.Symlink(wrongTarget, symlinkPath))
			},
			expectedResult: false,
		},
		{
			name: "returns false for regular directory",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(symlinkPath, 0o700))
			},
			expectedResult: false,
		},
		{
			name: "returns false for regular file",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
				require.NoError(t, os.WriteFile(symlinkPath, []byte("test"), 0o600))
			},
			expectedResult: false,
		},
		{
			name: "returns false when path does not exist",
			setup: func(t *testing.T, symlinkPath, targetPath string) {
				// No setup - path doesn't exist.
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			homeDir := t.TempDir()
			symlinkPath := filepath.Join(homeDir, "aws", "saml2aws")
			targetPath := filepath.Join(homeDir, "cache", "saml-target")

			tc.setup(t, symlinkPath, targetPath)

			p := &samlProvider{
				name:   "test",
				config: &schema.Provider{},
			}

			result := p.isCorrectSymlink(symlinkPath, targetPath)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestSAMLProvider_setupBrowserAutomation_CallsStorageSetup(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(homeDir, ".cache"))

	p, err := NewSAMLProvider("test", &schema.Provider{
		Kind:   "aws/saml",
		URL:    "https://idp.example.com/saml",
		Region: "us-east-1",
	})
	require.NoError(t, err)

	sp := p.(*samlProvider)
	sp.setupBrowserAutomation()

	// Verify that setupBrowserStorageDir was called by checking if directories exist.
	xdgCacheDir := filepath.Join(homeDir, ".cache", "atmos", "aws-saml", "test")
	info, err := os.Stat(xdgCacheDir)
	require.NoError(t, err, "XDG cache directory should be created")
	assert.True(t, info.IsDir())

	// Verify symlink was created.
	saml2awsPath := filepath.Join(homeDir, ".aws", "saml2aws")
	info, err = os.Lstat(saml2awsPath)
	require.NoError(t, err, "Symlink should be created")
	assert.True(t, info.Mode()&os.ModeSymlink != 0)
}

func TestSAMLProvider_setupBrowserAutomation_HandlesStorageSetupFailureGracefully(t *testing.T) {
	// Test that setupBrowserAutomation continues even if storage setup fails.
	// This tests the error handling path in setupBrowserAutomation.

	// Create an environment where setupBrowserStorageDir will fail.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	// Create ~/.aws as a regular file (not directory) to cause mkdir failure.
	awsPath := filepath.Join(homeDir, ".aws")
	require.NoError(t, os.WriteFile(awsPath, []byte("not a directory"), 0o600))

	p, err := NewSAMLProvider("test", &schema.Provider{
		Kind:                  "aws/saml",
		URL:                   "https://idp.example.com/saml",
		Region:                "us-east-1",
		DownloadBrowserDriver: true,
	})
	require.NoError(t, err)

	sp := p.(*samlProvider)

	// This should not panic even though storage setup will fail.
	// setupBrowserAutomation logs a warning but continues.
	sp.setupBrowserAutomation()

	// Verify the environment variable was still set (proves function continued).
	assert.Equal(t, "true", os.Getenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD"))
}
