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
		expectedColumns []string
		expectedSort    string
	}{
		{
			name: "all options populated",
			opts: &VendorOptions{
				Format:  "json",
				Stack:   "prod-*",
				Columns: []string{"component", "type"},
				Sort:    "component:asc",
			},
			expectedFormat:  "json",
			expectedStack:   "prod-*",
			expectedColumns: []string{"component", "type"},
			expectedSort:    "component:asc",
		},
		{
			name:            "empty options",
			opts:            &VendorOptions{},
			expectedFormat:  "",
			expectedStack:   "",
			expectedColumns: nil,
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
			expectedColumns: nil,
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

// TestBuildVendorFilters_Tags verifies --tags produces a filter against the
// raw tags_list field, alone and combined with the --stack component glob.
func TestBuildVendorFilters_Tags(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		filters := buildVendorFilters(&VendorOptions{})
		assert.Empty(t, filters)
	})

	t.Run("tags only", func(t *testing.T) {
		filters := buildVendorFilters(&VendorOptions{Tags: []string{"networking"}})
		assert.Len(t, filters, 1)
	})

	t.Run("stack glob plus tags", func(t *testing.T) {
		filters := buildVendorFilters(&VendorOptions{Stack: "vpc*", Tags: []string{"networking"}})
		assert.Len(t, filters, 2)
	})

	t.Run("tags filter narrows rows on tags_list", func(t *testing.T) {
		rows := []map[string]any{
			{"component": "vpc", "tags": "networking, aws", "tags_list": []string{"networking", "aws"}},
			{"component": "rds", "tags": "database", "tags_list": []string{"database"}},
		}
		filters := buildVendorFilters(&VendorOptions{Tags: []string{"networking"}})

		result := any(rows)
		var err error
		for _, f := range filters {
			result, err = f.Apply(result)
			assert.NoError(t, err)
		}

		filtered, ok := result.([]map[string]any)
		assert.True(t, ok)
		assert.Len(t, filtered, 1)
		assert.Equal(t, "vpc", filtered[0]["component"])
	})
}

// TestVendorCmd_LabelsFlagNotRegistered confirms --labels is rejected at flag
// parsing (unknown flag) rather than silently ignored: vendor/component
// manifests have no labels concept.
func TestVendorCmd_LabelsFlagNotRegistered(t *testing.T) {
	assert.Nil(t, vendorCmd.Flags().Lookup("labels"), "--labels must not be registered on list vendor")
	assert.NotNil(t, vendorCmd.Flags().Lookup("tags"), "--tags must be registered on list vendor")
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
