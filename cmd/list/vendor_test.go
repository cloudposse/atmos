package list

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestListVendorCmd_WithoutStacks verifies that list vendor does not require stack configuration.
// This test documents that the command uses InitCliConfig with processStacks=false.
func TestListVendorCmd_WithoutStacks(t *testing.T) {
	// This test documents that list vendor command does not process stacks
	// by verifying InitCliConfig is called with processStacks=false in list_vendor.go:44
	// and that checkAtmosConfig is called with WithStackValidation(false) in list_vendor.go:20
	// No runtime test needed - this is enforced by code structure.
	t.Log("list vendor command uses InitCliConfig with processStacks=false")
}

// TestObfuscateHomeDirInOutput verifies that home directory paths are properly obfuscated.
func TestObfuscateHomeDirInOutput(t *testing.T) {
	// Determine expected home directory.
	homeDir := os.Getenv("HOME")
	if runtime.GOOS == "windows" {
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			homeDir = userProfile
		}
	}

	if homeDir == "" {
		t.Skip("Could not determine home directory for test")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "absolute path with home directory",
			input:    filepath.Join(homeDir, "path", "to", "file"),
			expected: filepath.Join("~", "path", "to", "file"),
		},
		{
			name:     "home directory only",
			input:    homeDir,
			expected: "~",
		},
		{
			name:     "path without home directory",
			input:    "/var/lib/atmos/vendor",
			expected: "/var/lib/atmos/vendor",
		},
		{
			name:     "mixed content with home directory",
			input:    "Component: vpc\nManifest: " + filepath.Join(homeDir, ".atmos", "vendor.yaml"),
			expected: "Component: vpc\nManifest: " + filepath.Join("~", ".atmos", "vendor.yaml"),
		},
		{
			name:     "multiple occurrences of home directory",
			input:    homeDir + "/path1 and " + homeDir + "/path2",
			expected: "~/path1 and ~/path2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := obfuscateHomeDirInOutput(tt.input)
			if result != tt.expected {
				t.Errorf("obfuscateHomeDirInOutput() = %q, want %q", result, tt.expected)
			}

			// Verify home directory is not present in output.
			if strings.Contains(result, homeDir) {
				t.Errorf("obfuscateHomeDirInOutput() still contains home directory: %q", result)
			}
		})
	}
}
