package security

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a symlink for testing.
func createTestSymlink(t *testing.T, oldname, newname string) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}
	err := os.Symlink(oldname, newname)
	require.NoError(t, err, "Failed to create test symlink")
}

func TestParsePolicy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected SymlinkPolicy
	}{
		{"allow_safe lowercase", "allow_safe", PolicyAllowSafe},
		{"allow_safe with dash", "allow-safe", PolicyAllowSafe},
		{"allow_safe uppercase", "ALLOW_SAFE", PolicyAllowSafe},
		{"allow_safe with spaces", "  allow_safe  ", PolicyAllowSafe},
		{"reject_all lowercase", "reject_all", PolicyRejectAll},
		{"reject_all with dash", "reject-all", PolicyRejectAll},
		{"reject_all uppercase", "REJECT_ALL", PolicyRejectAll},
		{"allow_all lowercase", "allow_all", PolicyAllowAll},
		{"allow_all with dash", "allow-all", PolicyAllowAll},
		{"allow_all uppercase", "ALLOW_ALL", PolicyAllowAll},
		{"empty string defaults to safe", "", PolicyAllowSafe},
		{"unknown value defaults to safe", "unknown_policy", PolicyAllowSafe},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParsePolicy(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSymlinkSafe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create temporary test directory structure.
	tempDir := t.TempDir()

	// Create subdirectories.
	subDir := filepath.Join(tempDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	// Create a file in the subdirectory.
	testFile := filepath.Join(subDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	// Create external directory and file.
	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "external.txt")
	require.NoError(t, os.WriteFile(externalFile, []byte("external"), 0o644))

	tests := []struct {
		name       string
		linkTarget string
		linkPath   string
		boundary   string
		expectSafe bool
	}{
		{
			name:       "relative symlink within boundary",
			linkTarget: "test.txt",
			linkPath:   filepath.Join(subDir, "link1.txt"),
			boundary:   tempDir,
			expectSafe: true,
		},
		{
			name:       "relative symlink to subdir within boundary",
			linkTarget: "subdir/test.txt",
			linkPath:   filepath.Join(tempDir, "link2.txt"),
			boundary:   tempDir,
			expectSafe: true,
		},
		{
			name:       "absolute symlink within boundary",
			linkTarget: testFile,
			linkPath:   filepath.Join(tempDir, "link3.txt"),
			boundary:   tempDir,
			expectSafe: true,
		},
		{
			name:       "relative symlink escaping boundary",
			linkTarget: "../../external.txt",
			linkPath:   filepath.Join(subDir, "link4.txt"),
			boundary:   tempDir,
			expectSafe: false,
		},
		{
			name:       "absolute symlink outside boundary",
			linkTarget: externalFile,
			linkPath:   filepath.Join(tempDir, "link5.txt"),
			boundary:   tempDir,
			expectSafe: false,
		},
		{
			name:       "absolute symlink to system file",
			linkTarget: "/etc/passwd",
			linkPath:   filepath.Join(tempDir, "link6.txt"),
			boundary:   tempDir,
			expectSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the symlink.
			createTestSymlink(t, tt.linkTarget, tt.linkPath)
			defer os.Remove(tt.linkPath)

			// Test if symlink is safe.
			result := IsSymlinkSafe(tt.linkPath, tt.boundary)
			assert.Equal(t, tt.expectSafe, result)
		})
	}
}

func TestIsSymlinkSafe_BrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	tempDir := t.TempDir()
	brokenLink := filepath.Join(tempDir, "broken.txt")

	// Create a symlink to a non-existent file.
	createTestSymlink(t, "nonexistent.txt", brokenLink)
	defer os.Remove(brokenLink)

	// Broken symlinks pointing to non-existent files within boundary should be considered safe.
	// The target doesn't exist, but it would be within the boundary if it did.
	result := IsSymlinkSafe(brokenLink, tempDir)
	assert.True(t, result, "Broken symlink within boundary should be safe")
}

func TestIsSymlinkSafe_IntermediateDirectorySymlink(t *testing.T) {
	t.Skip("TODO: Implement detection of intermediate directory symlinks - requires path traversal validation")

	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create test directories.
	tempDir := t.TempDir()
	externalDir := t.TempDir()

	// Create a file in the external directory.
	externalFile := filepath.Join(externalDir, "external.txt")
	require.NoError(t, os.WriteFile(externalFile, []byte("sensitive data"), 0o644))

	// Create a directory symlink inside the boundary that points to externalDir.
	dirLink := filepath.Join(tempDir, "outdir")
	createTestSymlink(t, externalDir, dirLink)
	defer os.Remove(dirLink)

	// Create a file symlink whose target is the file inside that dir-symlink path.
	// This attempts to escape the boundary via an intermediate directory symlink.
	fileLink := filepath.Join(tempDir, "escape.txt")
	createTestSymlink(t, filepath.Join("outdir", "external.txt"), fileLink)
	defer os.Remove(fileLink)

	// The file symlink should be detected as unsafe because it escapes via the directory symlink.
	result := IsSymlinkSafe(fileLink, tempDir)
	assert.False(t, result, "Symlink escaping via intermediate directory symlink should be unsafe")

	// Also test the directory symlink itself.
	dirResult := IsSymlinkSafe(dirLink, tempDir)
	assert.False(t, dirResult, "Directory symlink pointing outside boundary should be unsafe")
}

func TestGetPolicyFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected SymlinkPolicy
	}{
		{
			name:     "nil config returns default policy",
			config:   nil,
			expected: PolicyAllowSafe,
		},
		{
			name: "empty policy string returns default",
			config: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "",
					},
				},
			},
			expected: PolicyAllowSafe,
		},
		{
			name: "reject_all policy",
			config: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "reject_all",
					},
				},
			},
			expected: PolicyRejectAll,
		},
		{
			name: "allow_all policy",
			config: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "allow_all",
					},
				},
			},
			expected: PolicyAllowAll,
		},
		{
			name: "allow_safe policy",
			config: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "allow_safe",
					},
				},
			},
			expected: PolicyAllowSafe,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPolicyFromConfig(tt.config)
			assert.Equal(t, tt.expected, result, "GetPolicyFromConfig should return the expected policy")
		})
	}
}

func TestCreateSymlinkHandler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	// Create test file.
	testFile := filepath.Join(subDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	// Create safe and unsafe symlinks.
	safeLink := filepath.Join(tempDir, "safe_link.txt")
	createTestSymlink(t, "subdir/test.txt", safeLink)
	defer os.Remove(safeLink)

	unsafeLink := filepath.Join(tempDir, "unsafe_link.txt")
	createTestSymlink(t, "/etc/passwd", unsafeLink)
	defer os.Remove(unsafeLink)

	tests := []struct {
		name           string
		policy         SymlinkPolicy
		symlink        string
		expectedAction cp.SymlinkAction
	}{
		{
			name:           "allow_safe with safe symlink",
			policy:         PolicyAllowSafe,
			symlink:        safeLink,
			expectedAction: cp.Deep,
		},
		{
			name:           "allow_safe with unsafe symlink",
			policy:         PolicyAllowSafe,
			symlink:        unsafeLink,
			expectedAction: cp.Skip,
		},
		{
			name:           "reject_all with safe symlink",
			policy:         PolicyRejectAll,
			symlink:        safeLink,
			expectedAction: cp.Skip,
		},
		{
			name:           "reject_all with unsafe symlink",
			policy:         PolicyRejectAll,
			symlink:        unsafeLink,
			expectedAction: cp.Skip,
		},
		{
			name:           "allow_all with safe symlink",
			policy:         PolicyAllowAll,
			symlink:        safeLink,
			expectedAction: cp.Deep,
		},
		{
			name:           "allow_all with unsafe symlink",
			policy:         PolicyAllowAll,
			symlink:        unsafeLink,
			expectedAction: cp.Deep,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := CreateSymlinkHandler(tempDir, tt.policy)
			action := handler(tt.symlink)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}

func TestValidateSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	// Create test file.
	testFile := filepath.Join(subDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	// Create various symlinks.
	safeLink := filepath.Join(tempDir, "safe_link.txt")
	createTestSymlink(t, "subdir/test.txt", safeLink)

	unsafeLink := filepath.Join(tempDir, "unsafe_link.txt")
	createTestSymlink(t, "/etc/passwd", unsafeLink)

	internalLink := filepath.Join(subDir, "internal_link.txt")
	createTestSymlink(t, "test.txt", internalLink)

	tests := []struct {
		name               string
		policy             SymlinkPolicy
		expectSafeLink     bool
		expectUnsafeLink   bool
		expectInternalLink bool
	}{
		{
			name:               "PolicyAllowAll keeps all symlinks",
			policy:             PolicyAllowAll,
			expectSafeLink:     true,
			expectUnsafeLink:   true,
			expectInternalLink: true,
		},
		{
			name:               "PolicyRejectAll removes all symlinks",
			policy:             PolicyRejectAll,
			expectSafeLink:     false,
			expectUnsafeLink:   false,
			expectInternalLink: false,
		},
		{
			name:               "PolicyAllowSafe keeps safe, removes unsafe",
			policy:             PolicyAllowSafe,
			expectSafeLink:     true,
			expectUnsafeLink:   false,
			expectInternalLink: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the directory for this test.
			testDir := filepath.Join(t.TempDir(), "test")
			err := cp.Copy(tempDir, testDir)
			require.NoError(t, err)

			// Run validation.
			err = ValidateSymlinks(testDir, tt.policy)
			require.NoError(t, err)

			// Check which symlinks still exist.
			_, safeErr := os.Lstat(filepath.Join(testDir, "safe_link.txt"))
			if tt.expectSafeLink {
				assert.NoError(t, safeErr, "Safe link should exist")
			} else {
				assert.True(t, os.IsNotExist(safeErr), "Safe link should not exist")
			}

			_, unsafeErr := os.Lstat(filepath.Join(testDir, "unsafe_link.txt"))
			if tt.expectUnsafeLink {
				assert.NoError(t, unsafeErr, "Unsafe link should exist")
			} else {
				assert.True(t, os.IsNotExist(unsafeErr), "Unsafe link should not exist")
			}

			_, internalErr := os.Lstat(filepath.Join(testDir, "subdir", "internal_link.txt"))
			if tt.expectInternalLink {
				assert.NoError(t, internalErr, "Internal link should exist")
			} else {
				assert.True(t, os.IsNotExist(internalErr), "Internal link should not exist")
			}
		})
	}
}

// TestCVE_2025_8959_Protection tests protection against the specific CVE vulnerability.
func TestCVE_2025_8959_Protection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create a structure simulating a malicious repository.
	maliciousRepo := t.TempDir()
	componentsDir := filepath.Join(maliciousRepo, "components")
	require.NoError(t, os.Mkdir(componentsDir, 0o755))

	// Create attack symlinks.
	attacks := []struct {
		name   string
		target string
		link   string
	}{
		{
			name:   "directory_traversal_attack",
			target: "../../../../etc/passwd",
			link:   filepath.Join(componentsDir, "passwd_link"),
		},
		{
			name:   "absolute_path_attack",
			target: "/etc/shadow",
			link:   filepath.Join(componentsDir, "shadow_link"),
		},
		{
			name:   "home_directory_attack",
			target: filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa"),
			link:   filepath.Join(componentsDir, "ssh_key_link"),
		},
	}

	for _, attack := range attacks {
		createTestSymlink(t, attack.target, attack.link)
	}

	// Test each policy.
	t.Run("PolicyAllowSafe_blocks_attacks", func(t *testing.T) {
		err := ValidateSymlinks(maliciousRepo, PolicyAllowSafe)
		require.NoError(t, err)

		// All attack symlinks should be removed.
		for _, attack := range attacks {
			_, err := os.Lstat(attack.link)
			assert.True(t, os.IsNotExist(err), "Attack symlink %s should be removed", attack.name)
		}
	})

	// Recreate symlinks for next test.
	for _, attack := range attacks {
		if _, err := os.Lstat(attack.link); os.IsNotExist(err) {
			createTestSymlink(t, attack.target, attack.link)
		}
	}

	t.Run("PolicyRejectAll_blocks_attacks", func(t *testing.T) {
		err := ValidateSymlinks(maliciousRepo, PolicyRejectAll)
		require.NoError(t, err)

		// All symlinks should be removed.
		for _, attack := range attacks {
			_, err := os.Lstat(attack.link)
			assert.True(t, os.IsNotExist(err), "Attack symlink %s should be removed", attack.name)
		}
	})
}
