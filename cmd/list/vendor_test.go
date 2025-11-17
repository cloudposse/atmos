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
		name            string
		opts            *VendorOptions
		expectedFormat  string
		expectedStack   string
		expectedColumns string
		expectedSort    string
	}{
		{
			name: "all options populated",
			opts: &VendorOptions{
				Format:  "json",
				Stack:   "prod-*",
				Columns: "component,type",
				Sort:    "component:asc",
			},
			expectedFormat:  "json",
			expectedStack:   "prod-*",
			expectedColumns: "component,type",
			expectedSort:    "component:asc",
		},
		{
			name:            "empty options",
			opts:            &VendorOptions{},
			expectedFormat:  "",
			expectedStack:   "",
			expectedColumns: "",
			expectedSort:    "",
		},
		{
			name: "yaml format with stack filter",
			opts: &VendorOptions{
				Format: "yaml",
				Stack:  "*-staging-*",
			},
			expectedFormat:  "yaml",
			expectedStack:   "*-staging-*",
			expectedColumns: "",
			expectedSort:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedFormat, tc.opts.Format)
			assert.Equal(t, tc.expectedStack, tc.opts.Stack)
			assert.Equal(t, tc.expectedColumns, tc.opts.Columns)
			assert.Equal(t, tc.expectedSort, tc.opts.Sort)
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
		name              string
		input             string
		expected          string
		shouldContainHome bool // true if the result is expected to contain homeDir (e.g., prefix cases)
	}{
		{
			name:              "absolute path with home directory",
			input:             filepath.Join(homeDir, "path", "to", "file"),
			expected:          filepath.Join("~", "path", "to", "file"),
			shouldContainHome: false,
		},
		{
			name:              "home directory only",
			input:             homeDir,
			expected:          "~",
			shouldContainHome: false,
		},
		{
			name:              "path without home directory",
			input:             "/var/lib/atmos/vendor",
			expected:          "/var/lib/atmos/vendor",
			shouldContainHome: false,
		},
		{
			name:              "mixed content with home directory",
			input:             "Component: vpc\nManifest: " + filepath.Join(homeDir, ".atmos", "vendor.yaml"),
			expected:          "Component: vpc\nManifest: " + filepath.Join("~", ".atmos", "vendor.yaml"),
			shouldContainHome: false,
		},
		{
			name:              "multiple occurrences of home directory",
			input:             homeDir + "/path1 and " + homeDir + "/path2",
			expected:          "~/path1 and ~/path2",
			shouldContainHome: false,
		},
		{
			name:              "homeDir as prefix of another path should not be replaced",
			input:             homeDir + "name/file",
			expected:          homeDir + "name/file",
			shouldContainHome: true, // We expect homeDir to remain in this case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := obfuscateHomeDirInOutput(tt.input)
			if result != tt.expected {
				t.Errorf("obfuscateHomeDirInOutput() = %q, want %q", result, tt.expected)
			}

			// Verify home directory is not present in output (unless it's expected to be there).
			if !tt.shouldContainHome && strings.Contains(result, homeDir) {
				t.Errorf("obfuscateHomeDirInOutput() still contains home directory: %q", result)
			}
		})
	}
}
