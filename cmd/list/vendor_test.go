package list

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestVendorOptions tests the VendorOptions structure.
func TestVendorOptions(t *testing.T) {
	testCases := []struct {
		name              string
		opts              *VendorOptions
		expectedFormat    string
		expectedStack     string
		expectedDelimiter string
	}{
		{
			name: "all options populated",
			opts: &VendorOptions{
				Format:    "json",
				Stack:     "prod-*",
				Delimiter: ",",
			},
			expectedFormat:    "json",
			expectedStack:     "prod-*",
			expectedDelimiter: ",",
		},
		{
			name:              "empty options",
			opts:              &VendorOptions{},
			expectedFormat:    "",
			expectedStack:     "",
			expectedDelimiter: "",
		},
		{
			name: "yaml format with stack filter",
			opts: &VendorOptions{
				Format: "yaml",
				Stack:  "*-staging-*",
			},
			expectedFormat:    "yaml",
			expectedStack:     "*-staging-*",
			expectedDelimiter: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedFormat, tc.opts.Format)
			assert.Equal(t, tc.expectedStack, tc.opts.Stack)
			assert.Equal(t, tc.expectedDelimiter, tc.opts.Delimiter)
		})
	}
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
