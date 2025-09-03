package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/security"
)

// TestCVE_2025_8959_VendorProtection tests protection against CVE-2025-8959.
// This test verifies that the symlink security feature prevents unauthorized
// access to files outside vendor boundaries during vendor operations.
func TestCVE_2025_8959_VendorProtection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create a test directory structure.
	testDir := t.TempDir()
	
	// Create a sensitive file outside the vendor boundary.
	sensitiveDir := t.TempDir()
	sensitiveFile := filepath.Join(sensitiveDir, "sensitive.txt")
	require.NoError(t, os.WriteFile(sensitiveFile, []byte("SENSITIVE DATA"), 0644))
	
	// Create a malicious vendor source with attack symlinks.
	maliciousSource := filepath.Join(testDir, "malicious-source")
	require.NoError(t, os.MkdirAll(maliciousSource, 0755))
	
	// Create various attack symlinks.
	attacks := []struct {
		name   string
		link   string
		target string
	}{
		{
			name:   "absolute_path_attack",
			link:   filepath.Join(maliciousSource, "absolute_attack.txt"),
			target: sensitiveFile,
		},
		{
			name:   "directory_traversal_attack",
			link:   filepath.Join(maliciousSource, "traversal_attack.txt"),
			target: filepath.Join("..", "..", filepath.Base(sensitiveDir), "sensitive.txt"),
		},
		{
			name:   "etc_passwd_attack",
			link:   filepath.Join(maliciousSource, "passwd_attack.txt"),
			target: "/etc/passwd",
		},
	}
	
	// Create the attack symlinks.
	for _, attack := range attacks {
		require.NoError(t, os.Symlink(attack.target, attack.link), 
			"Failed to create attack symlink: %s", attack.name)
	}
	
	// Create a legitimate symlink within the source.
	legitimateTarget := filepath.Join(maliciousSource, "legitimate.txt")
	require.NoError(t, os.WriteFile(legitimateTarget, []byte("LEGITIMATE DATA"), 0644))
	legitimateLink := filepath.Join(maliciousSource, "legitimate_link.txt")
	require.NoError(t, os.Symlink("legitimate.txt", legitimateLink))

	// Test different security policies.
	testCases := []struct {
		name               string
		policy             security.SymlinkPolicy
		expectAttackBlocked bool
		expectLegitimateOK  bool
	}{
		{
			name:               "PolicyAllowSafe_blocks_attacks_allows_legitimate",
			policy:             security.PolicyAllowSafe,
			expectAttackBlocked: true,
			expectLegitimateOK:  true,
		},
		{
			name:               "PolicyRejectAll_blocks_everything",
			policy:             security.PolicyRejectAll,
			expectAttackBlocked: true,
			expectLegitimateOK:  false,
		},
		{
			name:               "PolicyAllowAll_allows_everything",
			policy:             security.PolicyAllowAll,
			expectAttackBlocked: false,
			expectLegitimateOK:  true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create destination directory.
			destDir := filepath.Join(testDir, "dest", tc.name)
			require.NoError(t, os.MkdirAll(destDir, 0755))
			
			// Test copying with the security policy.
			handler := security.CreateSymlinkHandler(maliciousSource, tc.policy)
			copyOptions := cp.Options{
				OnSymlink: handler,
			}
			
			err := cp.Copy(maliciousSource, destDir, copyOptions)
			require.NoError(t, err, "Copy operation failed")
			
			// Check if attack symlinks were blocked.
			for _, attack := range attacks {
				destLink := filepath.Join(destDir, filepath.Base(attack.link))
				_, err := os.Lstat(destLink)
				
				if tc.expectAttackBlocked {
					assert.True(t, os.IsNotExist(err), 
						"Attack symlink should be blocked: %s", attack.name)
				} else {
					assert.NoError(t, err, 
						"Attack symlink should be allowed: %s", attack.name)
				}
			}
			
			// Check if legitimate symlink was handled correctly.
			destLegitimate := filepath.Join(destDir, "legitimate_link.txt")
			_, err = os.Stat(destLegitimate)
			
			if tc.expectLegitimateOK {
				assert.NoError(t, err, 
					"Legitimate symlink should work with policy %s", tc.policy)
				
				// If it's a deep copy, check the content.
				if tc.policy != security.PolicyRejectAll {
					content, err := os.ReadFile(destLegitimate)
					if err == nil {
						assert.Equal(t, "LEGITIMATE DATA", string(content),
							"Content should match for legitimate symlink")
					}
				}
			} else {
				assert.True(t, os.IsNotExist(err), 
					"Legitimate symlink should be blocked with policy %s", tc.policy)
			}
		})
	}
}

// TestVendorSymlinkPolicyIntegration tests the symlink policy integration
// with the actual vendor operations.
func TestVendorSymlinkPolicyIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create test directories.
	testDir := t.TempDir()
	sourceDir := filepath.Join(testDir, "source")
	targetDir := filepath.Join(testDir, "target")
	
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.MkdirAll(targetDir, 0755))
	
	// Create a file and a symlink in the source.
	testFile := filepath.Join(sourceDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))
	
	// Create an internal symlink (should be allowed with allow_safe).
	internalLink := filepath.Join(sourceDir, "internal_link.txt")
	require.NoError(t, os.Symlink("test.txt", internalLink))
	
	// Create an external symlink (should be blocked with allow_safe).
	externalLink := filepath.Join(sourceDir, "external_link.txt")
	require.NoError(t, os.Symlink("/etc/passwd", externalLink))
	
	// Test with different configurations.
	configs := []struct {
		name              string
		atmosConfig       *schema.AtmosConfiguration
		expectInternal    bool
		expectExternal    bool
	}{
		{
			name: "default_allow_safe",
			atmosConfig: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "",  // Empty defaults to allow_safe
					},
				},
			},
			expectInternal: true,
			expectExternal: false,
		},
		{
			name: "explicit_allow_safe",
			atmosConfig: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "allow_safe",
					},
				},
			},
			expectInternal: true,
			expectExternal: false,
		},
		{
			name: "reject_all",
			atmosConfig: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "reject_all",
					},
				},
			},
			expectInternal: false,
			expectExternal: false,
		},
		{
			name: "allow_all",
			atmosConfig: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{
					Policy: schema.VendorPolicy{
						Symlinks: "allow_all",
					},
				},
			},
			expectInternal: true,
			expectExternal: true,
		},
	}
	
	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			// Create a unique target for this test.
			testTarget := filepath.Join(targetDir, cfg.name)
			require.NoError(t, os.MkdirAll(testTarget, 0755))
			
			// Get the policy and create handler.
			policy := security.GetPolicyFromConfig(cfg.atmosConfig)
			handler := security.CreateSymlinkHandler(sourceDir, policy)
			
			// Copy with the policy.
			copyOptions := cp.Options{
				OnSymlink: handler,
			}
			
			err := cp.Copy(sourceDir, testTarget, copyOptions)
			require.NoError(t, err)
			
			// Check internal symlink.
			internalDest := filepath.Join(testTarget, "internal_link.txt")
			_, err = os.Stat(internalDest)
			if cfg.expectInternal {
				assert.NoError(t, err, 
					"Internal symlink should be present for %s", cfg.name)
			} else {
				assert.True(t, os.IsNotExist(err), 
					"Internal symlink should not be present for %s", cfg.name)
			}
			
			// Check external symlink.
			externalDest := filepath.Join(testTarget, "external_link.txt")
			_, err = os.Lstat(externalDest)
			if cfg.expectExternal {
				assert.NoError(t, err, 
					"External symlink should be present for %s", cfg.name)
			} else {
				assert.True(t, os.IsNotExist(err), 
					"External symlink should not be present for %s", cfg.name)
			}
		})
	}
}

// TestSymlinkPolicyValidation tests that the policy validation works correctly.
func TestSymlinkPolicyValidation(t *testing.T) {
	// Test policy parsing.
	testCases := []struct {
		input    string
		expected security.SymlinkPolicy
	}{
		{"allow_safe", security.PolicyAllowSafe},
		{"ALLOW_SAFE", security.PolicyAllowSafe},
		{"allow-safe", security.PolicyAllowSafe},
		{"reject_all", security.PolicyRejectAll},
		{"REJECT_ALL", security.PolicyRejectAll},
		{"reject-all", security.PolicyRejectAll},
		{"allow_all", security.PolicyAllowAll},
		{"ALLOW_ALL", security.PolicyAllowAll},
		{"allow-all", security.PolicyAllowAll},
		{"", security.PolicyAllowSafe},  // Default
		{"invalid", security.PolicyAllowSafe},  // Unknown defaults to safe
	}
	
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := security.ParsePolicy(tc.input)
			assert.Equal(t, tc.expected, result,
				"Policy parsing failed for input: %s", tc.input)
		})
	}
}

// TestCopyWithSymlinkPolicy tests that copy operations respect the symlink policy.
func TestCopyWithSymlinkPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create test structure.
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	
	// Create a normal file.
	normalFile := filepath.Join(sourceDir, "normal.txt")
	require.NoError(t, os.WriteFile(normalFile, []byte("normal"), 0644))
	
	// Create an internal symlink.
	internalLink := filepath.Join(sourceDir, "internal.txt")
	require.NoError(t, os.Symlink("normal.txt", internalLink))
	
	// Create an external symlink.
	externalLink := filepath.Join(sourceDir, "external.txt")
	require.NoError(t, os.Symlink("/etc/hosts", externalLink))
	
	// Test with allow_safe policy.
	atmosConfig := &schema.AtmosConfiguration{
		Vendor: schema.Vendor{
			Policy: schema.VendorPolicy{
				Symlinks: "allow_safe",
			},
		},
	}
	
	// Get policy and create handler.
	policy := security.GetPolicyFromConfig(atmosConfig)
	handler := security.CreateSymlinkHandler(sourceDir, policy)
	
	// Copy with the security policy.
	copyOptions := cp.Options{
		OnSymlink: handler,
	}
	
	err := cp.Copy(sourceDir, targetDir, copyOptions)
	require.NoError(t, err)
	
	// Check that normal file was copied.
	_, err = os.Stat(filepath.Join(targetDir, "normal.txt"))
	assert.NoError(t, err, "Normal file should be copied")
	
	// Check that internal symlink was processed.
	_, err = os.Stat(filepath.Join(targetDir, "internal.txt"))
	assert.NoError(t, err, "Internal symlink should be processed")
	
	// Check that external symlink was blocked.
	_, err = os.Lstat(filepath.Join(targetDir, "external.txt"))
	assert.True(t, os.IsNotExist(err), "External symlink should be blocked")
}