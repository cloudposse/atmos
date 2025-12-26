package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func setupTestFiles(t *testing.T) (string, func()) {
	tempDir := t.TempDir()

	cleanup := func() {
		// t.TempDir() handles cleanup automatically
	}

	return tempDir, cleanup
}

func createTestConfig(t *testing.T, dir string, content string) string {
	configPath := filepath.Join(dir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(content), 0o644)
	assert.NoError(t, err)
	return configPath
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		configContent  string
		setupEnv       map[string]string
		expectedError  bool
		validateConfig func(*testing.T, schema.AtmosConfiguration)
	}{
		{
			name: "basic valid config",
			configContent: `
base_path: /test/path
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
`,
			expectedError: false,
			validateConfig: func(t *testing.T, config schema.AtmosConfiguration) {
				assert.Equal(t, "/test/path", config.BasePath)
				assert.Equal(t, "components/terraform", config.Components.Terraform.BasePath)
				assert.Equal(t, "stacks", config.Stacks.BasePath)
			},
		},
		{
			name: "config with invalid YAML",
			configContent: `
base_path: /test/path
components:
  terraform:
    base_path: - invalid
    - yaml
    - content
`,
			expectedError: true,
		},
		{
			name:          "empty config",
			configContent: "",
			expectedError: false, // LoadConfig will use default config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tempDir, cleanup := setupTestFiles(t)
			defer cleanup()

			// Set up environment variables
			for k, v := range tt.setupEnv {
				t.Setenv(k, v)
			}

			// Create test config file
			configPath := createTestConfig(t, tempDir, tt.configContent)

			// Create ConfigAndStacksInfo
			configInfo := &schema.ConfigAndStacksInfo{
				AtmosConfigFilesFromArg: []string{configPath},
			}

			// Test LoadConfig function
			config, err := LoadConfig(configInfo)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.validateConfig != nil {
				tt.validateConfig(t, config)
			}
		})
	}
}

func TestLoadConfigFromDifferentSources(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected func(*testing.T, schema.AtmosConfiguration, error)
	}{
		{
			name: "load from ATMOS_CLI_CONFIG_PATH",
			envVars: map[string]string{
				"ATMOS_CLI_CONFIG_PATH": "testdata/config",
			},
			expected: func(t *testing.T, config schema.AtmosConfiguration, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "with github token",
			envVars: map[string]string{
				"GITHUB_TOKEN": "test-token",
			},
			expected: func(t *testing.T, config schema.AtmosConfiguration, err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			tt.expected(t, config, err)
		})
	}
}

func TestSetEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		checkKey string
		expected string
	}{
		{
			name: "GITHUB_TOKEN",
			envVars: map[string]string{
				"GITHUB_TOKEN": "test-token",
			},
			checkKey: "settings.github_token",
			expected: "test-token",
		},
		{
			name: "ATMOS_PRO_TOKEN",
			envVars: map[string]string{
				"ATMOS_PRO_TOKEN": "pro-token",
			},
			checkKey: "settings.pro.token",
			expected: "pro-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()

			// Set environment variables
			for k, val := range tt.envVars {
				t.Setenv(k, val)
			}

			setEnv(v)

			assert.Equal(t, tt.expected, v.GetString(tt.checkKey))
		})
	}
}

func TestLoadConfigWithInvalidPath(t *testing.T) {
	configInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{"nonexistent/path/atmos.yaml"},
	}

	_, err := LoadConfig(configInfo)
	assert.Error(t, err)
}

func TestMergeDefaultImports_ExclusionLogic(t *testing.T) {
	tests := []struct {
		name         string
		dirPath      string
		excludePaths string
		shouldSkip   bool
		description  string
	}{
		{
			name:         "exact_match_excluded",
			dirPath:      "/repo/root",
			excludePaths: "/repo/root",
			shouldSkip:   true,
			description:  "Should skip when path exactly matches exclude path",
		},
		{
			name:         "different_path_not_excluded",
			dirPath:      "/repo/root/tests",
			excludePaths: "/repo/root",
			shouldSkip:   false,
			description:  "Should not skip when path is different from exclude path",
		},
		{
			name:         "multiple_exclude_paths",
			dirPath:      "/repo/root",
			excludePaths: "/other/path" + string(os.PathListSeparator) + "/repo/root",
			shouldSkip:   true,
			description:  "Should handle multiple exclude paths separated by OS path list separator",
		},
		{
			name:         "empty_exclude_path",
			dirPath:      "/repo/root",
			excludePaths: "",
			shouldSkip:   false,
			description:  "Should not skip when exclude paths is empty",
		},
		{
			name:         "empty_entry_in_exclude_paths",
			dirPath:      "/repo/root",
			excludePaths: string(os.PathListSeparator) + "/other/path",
			shouldSkip:   false,
			description:  "Should handle empty entries in exclude paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory to use as the test directory
			tempDir := t.TempDir()
			testDir := filepath.Join(tempDir, "test")
			err := os.MkdirAll(testDir, 0o755)
			assert.NoError(t, err)

			// Create sentinel file in .atmos.d to verify if import happens
			atmosDDir := filepath.Join(tempDir, ".atmos.d")
			err = os.MkdirAll(atmosDDir, 0o755)
			assert.NoError(t, err)

			sentinelContent := `sentinel_key: sentinel_value`
			sentinelPath := filepath.Join(atmosDDir, "sentinel.yaml")
			err = os.WriteFile(sentinelPath, []byte(sentinelContent), 0o644)
			assert.NoError(t, err)

			// Also create one in the test subdirectory
			testAtmosDDir := filepath.Join(testDir, ".atmos.d")
			err = os.MkdirAll(testAtmosDDir, 0o755)
			assert.NoError(t, err)

			testSentinelContent := `test_sentinel_key: test_sentinel_value`
			testSentinelPath := filepath.Join(testAtmosDDir, "sentinel.yaml")
			err = os.WriteFile(testSentinelPath, []byte(testSentinelContent), 0o644)
			assert.NoError(t, err)

			// Set up the exclude environment variable
			if tt.excludePaths != "" {
				// Convert relative paths to absolute for the test
				var absoluteExcludePaths []string
				for _, p := range filepath.SplitList(tt.excludePaths) {
					if p != "" {
						switch p {
						case "/repo/root":
							p = tempDir // Use temp dir as mock repo root
						case "/repo/root/tests":
							p = testDir
						case "/other/path":
							// Keep as is - it won't match anything
						default:
							// Keep p as is for any other path
						}
						absoluteExcludePaths = append(absoluteExcludePaths, p)
					}
				}
				if len(absoluteExcludePaths) > 0 {
					// Join multiple paths with the OS-specific path list separator
					joinedPaths := absoluteExcludePaths[0]
					for i := 1; i < len(absoluteExcludePaths); i++ {
						joinedPaths = joinedPaths + string(os.PathListSeparator) + absoluteExcludePaths[i]
					}
					t.Setenv("TEST_EXCLUDE_ATMOS_D", joinedPaths)

				}

			}

			// Adjust dirPath for the test
			actualDirPath := tt.dirPath
			expectedSentinelKey := ""
			switch tt.dirPath {
			case "/repo/root":
				actualDirPath = tempDir
				expectedSentinelKey = "sentinel_key"
			case "/repo/root/tests":
				actualDirPath = testDir
				expectedSentinelKey = "test_sentinel_key"
			}

			// Call the function
			v := viper.New()
			v.SetConfigType("yaml") // Set config type as done in production code
			err = mergeDefaultImports(actualDirPath, v)
			assert.NoError(t, err, tt.description)

			// Assert the behavior based on shouldSkip
			if tt.shouldSkip {
				// When skipped, viper should not contain the sentinel key
				assert.False(t, v.IsSet(expectedSentinelKey), "Viper should not contain sentinel key when import is skipped")
			} else if expectedSentinelKey != "" {
				// When not skipped, viper should contain the sentinel key if .atmos.d exists
				// Note: mergeDefaultImports will load the config if .atmos.d exists in the path
				assert.True(t, v.IsSet(expectedSentinelKey), "Viper should contain sentinel key when import runs")
			}
		})
	}
}

func TestMergeDefaultImports_PathCanonicalization(t *testing.T) {
	tests := []struct {
		name        string
		dirPath     string
		excludePath string
		shouldSkip  bool
		description string
	}{
		{
			name:        "relative_to_absolute",
			dirPath:     ".",
			excludePath: ".",
			shouldSkip:  true,
			description: "Should match relative paths converted to absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create and change to temp directory
			tempDir := t.TempDir()
			t.Chdir(tempDir)

			// Set the exclude environment variable using table input directly
			t.Setenv("TEST_EXCLUDE_ATMOS_D", tt.excludePath)

			// Call the function with table input directly
			v := viper.New()
			v.SetConfigType("yaml") // Set config type as done in production code
			err := mergeDefaultImports(tt.dirPath, v)

			// Check the result - should skip and return nil when shouldSkip is true
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestMergeDefaultImports_EmptyAndInvalidPaths(t *testing.T) {
	tests := []struct {
		name         string
		excludePaths string
		description  string
	}{
		{
			name:         "empty_string",
			excludePaths: "",
			description:  "Should handle empty exclude paths",
		},
		{
			name:         "only_separators",
			excludePaths: string(os.PathListSeparator) + string(os.PathListSeparator),
			description:  "Should handle exclude paths with only separators",
		},
		{
			name:         "paths_with_empty_entries",
			excludePaths: "/valid/path" + string(os.PathListSeparator) + string(os.PathListSeparator) + "/another/path",
			description:  "Should skip empty entries between separators",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir := t.TempDir()

			// Set the exclude environment variable
			if tt.excludePaths != "" {
				t.Setenv("TEST_EXCLUDE_ATMOS_D", tt.excludePaths)
			}

			// Call the function - should not panic or error on empty/invalid paths
			v := viper.New()
			v.SetConfigType("yaml") // Set config type as done in production code
			err := mergeDefaultImports(tempDir, v)

			// mergeDefaultImports returns nil even if no atmos.d exists - it just doesn't merge anything
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestShouldExcludePathForTesting(t *testing.T) {
	tests := []struct {
		name        string
		dirPath     string
		envValue    string
		expected    bool
		description string
	}{
		{
			name:        "no_env_variable_set",
			dirPath:     "/some/path",
			envValue:    "",
			expected:    false,
			description: "Should not exclude when TEST_EXCLUDE_ATMOS_D is not set",
		},
		{
			name:        "exact_match_single_path",
			dirPath:     "/excluded/path",
			envValue:    "/excluded/path",
			expected:    true,
			description: "Should exclude when path exactly matches single excluded path",
		},
		{
			name:        "no_match_single_path",
			dirPath:     "/other/path",
			envValue:    "/excluded/path",
			expected:    false,
			description: "Should not exclude when path doesn't match",
		},
		{
			name:        "exact_match_in_multiple_paths",
			dirPath:     "/excluded/path2",
			envValue:    "/excluded/path1" + string(os.PathListSeparator) + "/excluded/path2" + string(os.PathListSeparator) + "/excluded/path3",
			expected:    true,
			description: "Should exclude when path matches one of multiple excluded paths",
		},
		{
			name:        "no_match_in_multiple_paths",
			dirPath:     "/other/path",
			envValue:    "/excluded/path1" + string(os.PathListSeparator) + "/excluded/path2",
			expected:    false,
			description: "Should not exclude when path doesn't match any of multiple paths",
		},
		{
			name:        "empty_entries_in_path_list",
			dirPath:     "/excluded/path",
			envValue:    string(os.PathListSeparator) + "/excluded/path" + string(os.PathListSeparator) + string(os.PathListSeparator),
			expected:    true,
			description: "Should handle empty entries in path list correctly",
		},
		{
			name:        "relative_paths_converted_to_absolute",
			dirPath:     ".",
			envValue:    ".",
			expected:    true,
			description: "Should match relative paths after converting to absolute",
		},
		{
			name:        "malformed_path_in_list",
			dirPath:     "/valid/path",
			envValue:    "not-absolute-path" + string(os.PathListSeparator) + "/valid/path",
			expected:    true,
			description: "Should handle mix of absolute and relative paths in list",
		},
		{
			name:        "subdirectory_not_matched",
			dirPath:     "/excluded/path/subdir",
			envValue:    "/excluded/path",
			expected:    false,
			description: "Should not match subdirectories (exact match only)",
		},
		{
			name:        "parent_directory_not_matched",
			dirPath:     "/excluded",
			envValue:    "/excluded/path",
			expected:    false,
			description: "Should not match parent directories (exact match only)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For relative path tests, create and change to a temp directory
			if tt.dirPath == "." || tt.envValue == "." {
				tempDir := t.TempDir()
				t.Chdir(tempDir)
			}

			// Set the environment variable
			if tt.envValue != "" {
				t.Setenv("TEST_EXCLUDE_ATMOS_D", tt.envValue)
			}

			// Call the function
			result := shouldExcludePathForTesting(tt.dirPath)

			// Assert the result
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestShouldExcludePathForTesting_PathCanonicalization(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		dirPath  string
		envValue string
		expected bool
		setup    func()
	}{
		{
			name:     "paths_with_trailing_slashes",
			dirPath:  tempDir + string(filepath.Separator),
			envValue: tempDir,
			expected: true,
			setup:    nil,
		},
		{
			name:     "paths_with_double_slashes",
			dirPath:  filepath.Join(tempDir, ".", "subdir"),
			envValue: filepath.Join(tempDir, "subdir"),
			expected: true,
			setup: func() {
				os.MkdirAll(filepath.Join(tempDir, "subdir"), 0o755)
			},
		},
		{
			name:     "paths_with_dot_segments",
			dirPath:  filepath.Join(tempDir, "subdir", "..", "subdir"),
			envValue: filepath.Join(tempDir, "subdir"),
			expected: true,
			setup: func() {
				os.MkdirAll(filepath.Join(tempDir, "subdir"), 0o755)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			t.Setenv("TEST_EXCLUDE_ATMOS_D", tt.envValue)

			result := shouldExcludePathForTesting(tt.dirPath)
			assert.Equal(t, tt.expected, result, "Paths should be canonicalized before comparison")
		})
	}
}

func TestProcessConfigImportsAndReapply_MalformedYAML(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		description   string
	}{
		{
			name: "invalid_yaml_syntax",
			configContent: `
base_path: /test
components:
  terraform:
    - invalid: yaml
    - content: here
    base_path: components/terraform
`,
			description: "Should return error when YAML has invalid syntax",
		},
		{
			name: "unclosed_bracket",
			configContent: `
base_path: /test
components: {
  terraform: {
    base_path: components/terraform
`,
			description: "Should return error when YAML has unclosed brackets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir := t.TempDir()

			// Write malformed config file
			configPath := filepath.Join(tempDir, "atmos.yaml")
			err := os.WriteFile(configPath, []byte(tt.configContent), 0o644)
			assert.NoError(t, err)

			// Create viper instance
			v := viper.New()
			v.SetConfigFile(configPath)

			// Call the function - should return error on malformed YAML
			err = processConfigImportsAndReapply(configPath, v, []byte(tt.configContent))

			// Assert that an error was returned
			assert.Error(t, err, tt.description)
		})
	}
}

func TestInjectProvisionedIdentityImports_NoProviders(t *testing.T) {
	// Test that injectProvisionedIdentityImports does nothing when no auth providers are configured.
	src := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{},
		},
		Import: []string{"existing-import.yaml"},
	}

	err := injectProvisionedIdentityImports(src)
	assert.NoError(t, err)
	assert.Equal(t, []string{"existing-import.yaml"}, src.Import)
}

func TestInjectProvisionedIdentityImports_WithProviders(t *testing.T) {
	// Test that injectProvisionedIdentityImports prepends provisioned identity files when they exist.
	tempDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempDir)

	// Create mock provisioned identity file.
	provisioningDir := filepath.Join(tempDir, "atmos", "auth", "test-provider")
	err := os.MkdirAll(provisioningDir, 0o700)
	require.NoError(t, err)

	provisionedFile := filepath.Join(provisioningDir, "provisioned-identities.yaml")
	err = os.WriteFile(provisionedFile, []byte("identities: {}\n"), 0o600)
	require.NoError(t, err)

	src := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"test-provider": {
					Kind: "aws/iam-identity-center",
				},
			},
		},
		Import: []string{"existing-import.yaml"},
	}

	err = injectProvisionedIdentityImports(src)
	assert.NoError(t, err)

	// Should have provisioned import prepended.
	assert.Len(t, src.Import, 2)
	assert.Contains(t, src.Import[0], "test-provider")
	assert.Contains(t, src.Import[0], "provisioned-identities.yaml")
	assert.Equal(t, "existing-import.yaml", src.Import[1])
}

func TestInjectProvisionedIdentityImports_NoProvisionedFiles(t *testing.T) {
	// Test that injectProvisionedIdentityImports does nothing when provisioned files don't exist.
	tempDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempDir)

	src := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"test-provider": {
					Kind: "aws/iam-identity-center",
				},
			},
		},
		Import: []string{"existing-import.yaml"},
	}

	err := injectProvisionedIdentityImports(src)
	assert.NoError(t, err)

	// Should not modify imports when no provisioned files exist.
	assert.Equal(t, []string{"existing-import.yaml"}, src.Import)
}

func TestInjectProvisionedIdentityImports_MultipleProviders(t *testing.T) {
	// Test that injectProvisionedIdentityImports handles multiple providers.
	tempDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempDir)

	// Create provisioned files for two providers.
	for _, providerName := range []string{"provider1", "provider2"} {
		provisioningDir := filepath.Join(tempDir, "atmos", "auth", providerName)
		err := os.MkdirAll(provisioningDir, 0o700)
		require.NoError(t, err)

		provisionedFile := filepath.Join(provisioningDir, "provisioned-identities.yaml")
		err = os.WriteFile(provisionedFile, []byte("identities: {}\n"), 0o600)
		require.NoError(t, err)
	}

	src := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"provider1": {Kind: "aws/iam-identity-center"},
				"provider2": {Kind: "aws/iam-identity-center"},
			},
		},
		Import: []string{"existing-import.yaml"},
	}

	err := injectProvisionedIdentityImports(src)
	assert.NoError(t, err)

	// Should have both provisioned imports prepended.
	assert.Len(t, src.Import, 3)
	assert.Equal(t, "existing-import.yaml", src.Import[2])

	// Check that both provider imports are present.
	importPaths := strings.Join(src.Import, " ")
	assert.Contains(t, importPaths, "provider1")
	assert.Contains(t, importPaths, "provider2")
}

func TestInjectProvisionedIdentityImports_EmptyImportList(t *testing.T) {
	// Test that injectProvisionedIdentityImports works when Import list is initially empty.
	tempDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempDir)

	// Create mock provisioned identity file.
	provisioningDir := filepath.Join(tempDir, "atmos", "auth", "test-provider")
	err := os.MkdirAll(provisioningDir, 0o700)
	require.NoError(t, err)

	provisionedFile := filepath.Join(provisioningDir, "provisioned-identities.yaml")
	err = os.WriteFile(provisionedFile, []byte("identities: {}\n"), 0o600)
	require.NoError(t, err)

	src := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"test-provider": {
					Kind: "aws/iam-identity-center",
				},
			},
		},
		Import: []string{},
	}

	err = injectProvisionedIdentityImports(src)
	assert.NoError(t, err)

	// Should have only the provisioned import.
	assert.Len(t, src.Import, 1)
	assert.Contains(t, src.Import[0], "test-provider")
	assert.Contains(t, src.Import[0], "provisioned-identities.yaml")
}

func TestFindAtmosConfigInParentDirs(t *testing.T) {
	tests := []struct {
		name           string
		setupDirs      func(t *testing.T, tempDir string) string // Returns the start directory.
		expectedResult func(tempDir string) string               // Returns expected result.
	}{
		{
			name: "finds atmos.yaml in immediate parent",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create parent/child structure.
				childDir := filepath.Join(tempDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)

				// Create atmos.yaml in parent (tempDir).
				createTestConfig(t, tempDir, "base_path: .")

				return childDir
			},
			expectedResult: func(tempDir string) string {
				return tempDir
			},
		},
		{
			name: "finds atmos.yaml in grandparent",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create parent/child/grandchild structure.
				grandchildDir := filepath.Join(tempDir, "parent", "child")
				err := os.MkdirAll(grandchildDir, 0o755)
				require.NoError(t, err)

				// Create atmos.yaml in root (tempDir).
				createTestConfig(t, tempDir, "base_path: .")

				return grandchildDir
			},
			expectedResult: func(tempDir string) string {
				return tempDir
			},
		},
		{
			name: "finds .atmos.yaml in parent",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create parent/child structure.
				childDir := filepath.Join(tempDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)

				// Create .atmos.yaml (hidden) in parent.
				dotConfigPath := filepath.Join(tempDir, ".atmos.yaml")
				err = os.WriteFile(dotConfigPath, []byte("base_path: ."), 0o644)
				require.NoError(t, err)

				return childDir
			},
			expectedResult: func(tempDir string) string {
				return tempDir
			},
		},
		{
			name: "returns empty when no config found",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create child directory without any atmos config.
				childDir := filepath.Join(tempDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)

				return childDir
			},
			expectedResult: func(tempDir string) string {
				return ""
			},
		},
		{
			name: "finds closest atmos.yaml when multiple exist",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create nested structure with configs at multiple levels.
				// tempDir/atmos.yaml (grandparent).
				// tempDir/parent/atmos.yaml (parent).
				// tempDir/parent/child/ (starting dir).
				parentDir := filepath.Join(tempDir, "parent")
				childDir := filepath.Join(parentDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)

				// Create atmos.yaml in both levels.
				createTestConfig(t, tempDir, "base_path: grandparent")
				createTestConfig(t, parentDir, "base_path: parent")

				return childDir
			},
			expectedResult: func(tempDir string) string {
				return filepath.Join(tempDir, "parent")
			},
		},
		{
			name: "finds atmos.yaml in deeply nested structure",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create deeply nested component-like structure.
				// tempDir/atmos.yaml
				// tempDir/components/terraform/vpc/main.tf (start dir).
				deepDir := filepath.Join(tempDir, "components", "terraform", "vpc")
				err := os.MkdirAll(deepDir, 0o755)
				require.NoError(t, err)

				// Create atmos.yaml at root.
				createTestConfig(t, tempDir, "base_path: .")

				return deepDir
			},
			expectedResult: func(tempDir string) string {
				return tempDir
			},
		},
		{
			name: "prefers atmos.yaml over .atmos.yaml in same dir",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create parent/child structure with both configs in parent.
				childDir := filepath.Join(tempDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)

				// Create both atmos.yaml and .atmos.yaml in parent.
				createTestConfig(t, tempDir, "base_path: regular")
				dotConfigPath := filepath.Join(tempDir, ".atmos.yaml")
				err = os.WriteFile(dotConfigPath, []byte("base_path: hidden"), 0o644)
				require.NoError(t, err)

				return childDir
			},
			expectedResult: func(tempDir string) string {
				// Should find the directory - atmos.yaml is checked first.
				return tempDir
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			startDir := tt.setupDirs(t, tempDir)
			expected := tt.expectedResult(tempDir)

			result := findAtmosConfigInParentDirs(startDir)

			assert.Equal(t, expected, result)
		})
	}
}

func TestReadWorkDirConfig_ParentDirectorySearch(t *testing.T) {
	tests := []struct {
		name           string
		setupDirs      func(t *testing.T, tempDir string) string // Returns working directory.
		expectConfig   bool
		validateConfig func(t *testing.T, v *viper.Viper)
	}{
		{
			name: "loads config from current directory",
			setupDirs: func(t *testing.T, tempDir string) string {
				createTestConfig(t, tempDir, `
base_path: /test/current
`)
				return tempDir
			},
			expectConfig: true,
			validateConfig: func(t *testing.T, v *viper.Viper) {
				assert.Equal(t, "/test/current", v.GetString("base_path"))
			},
		},
		{
			name: "loads config from parent directory",
			setupDirs: func(t *testing.T, tempDir string) string {
				childDir := filepath.Join(tempDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)

				createTestConfig(t, tempDir, `
base_path: /test/parent
`)
				return childDir
			},
			expectConfig: true,
			validateConfig: func(t *testing.T, v *viper.Viper) {
				assert.Equal(t, "/test/parent", v.GetString("base_path"))
			},
		},
		{
			name: "loads config from deeply nested component directory",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Simulate component directory structure.
				componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
				err := os.MkdirAll(componentDir, 0o755)
				require.NoError(t, err)

				createTestConfig(t, tempDir, `
base_path: /test/component-root
components:
  terraform:
    base_path: components/terraform
`)
				return componentDir
			},
			expectConfig: true,
			validateConfig: func(t *testing.T, v *viper.Viper) {
				assert.Equal(t, "/test/component-root", v.GetString("base_path"))
				assert.Equal(t, "components/terraform", v.GetString("components.terraform.base_path"))
			},
		},
		{
			name: "no config found returns no error",
			setupDirs: func(t *testing.T, tempDir string) string {
				childDir := filepath.Join(tempDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)
				return childDir
			},
			expectConfig: false,
			validateConfig: func(t *testing.T, v *viper.Viper) {
				// Default values should be empty.
				assert.Empty(t, v.GetString("base_path"))
			},
		},
		{
			name: "ATMOS_CLI_CONFIG_PATH disables parent directory search",
			setupDirs: func(t *testing.T, tempDir string) string {
				childDir := filepath.Join(tempDir, "child")
				err := os.MkdirAll(childDir, 0o755)
				require.NoError(t, err)

				// Create config in parent directory.
				createTestConfig(t, tempDir, `
base_path: /test/parent-should-not-find
`)
				// Set env to disable parent search.
				t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
				return childDir
			},
			expectConfig: false,
			validateConfig: func(t *testing.T, v *viper.Viper) {
				// Should NOT find the parent config because ATMOS_CLI_CONFIG_PATH is set.
				assert.Empty(t, v.GetString("base_path"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			workDir := tt.setupDirs(t, tempDir)

			// Change to working directory using t.Chdir for automatic cleanup.
			t.Chdir(workDir)

			v := viper.New()
			v.SetConfigType("yaml")

			err := readWorkDirConfig(v)
			require.NoError(t, err)

			if tt.expectConfig {
				assert.NotEmpty(t, v.ConfigFileUsed())
				assert.Contains(t, v.ConfigFileUsed(), "atmos.yaml")
			}

			tt.validateConfig(t, v)
		})
	}
}

// TestLoadConfig_DefaultConfigWithGitRootAtmosD tests that .atmos.d at git root is loaded
// even when no atmos.yaml config file is found (using default config).
func TestLoadConfig_DefaultConfigWithGitRootAtmosD(t *testing.T) {
	tempDir := t.TempDir()

	// Create .atmos.d at "git root" with custom commands.
	atmosDDir := filepath.Join(tempDir, ".atmos.d")
	require.NoError(t, os.MkdirAll(atmosDDir, 0o755))

	commandsContent := `commands:
  - name: test-default-cmd
    description: Test command from git root with default config
    steps:
      - echo "hello from git root default config"
`
	require.NoError(t, os.WriteFile(
		filepath.Join(atmosDDir, "commands.yaml"),
		[]byte(commandsContent),
		0o644,
	))

	// Create a subdirectory with NO atmos.yaml - this will force default config.
	subDir := filepath.Join(tempDir, "no-config-subdir")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	// Mock git root to be tempDir.
	t.Setenv("TEST_GIT_ROOT", tempDir)

	// Change to subdirectory.
	t.Chdir(subDir)

	// Load config - should use default config but still find .atmos.d from git root.
	config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	// Verify that the custom command from .atmos.d was loaded.
	require.NotNil(t, config.Commands, "Commands should be loaded from .atmos.d at git root")
	require.Len(t, config.Commands, 1, "Should have one custom command")
	assert.Equal(t, "test-default-cmd", config.Commands[0].Name)
}

// TestMergeDefaultImports_GitRoot tests that .atmos.d at git root is discovered
// even when running from a subdirectory.
func TestMergeDefaultImports_GitRoot(t *testing.T) {
	tests := []struct {
		name          string
		setupDirs     func(t *testing.T, tempDir string) string
		expectedKey   string
		expectedValue string
		description   string
	}{
		{
			name: "loads_atmos_d_from_git_root_when_in_subdirectory",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create .atmos.d at "git root" (tempDir).
				atmosDDir := filepath.Join(tempDir, ".atmos.d")
				require.NoError(t, os.MkdirAll(atmosDDir, 0o755))

				commandsContent := `commands:
  - name: test-cmd
    description: Test command from git root
    steps:
      - echo "hello from git root"
`
				require.NoError(t, os.WriteFile(
					filepath.Join(atmosDDir, "commands.yaml"),
					[]byte(commandsContent),
					0o644,
				))

				// Create a subdirectory to run from.
				subDir := filepath.Join(tempDir, "test")
				require.NoError(t, os.MkdirAll(subDir, 0o755))

				return subDir
			},
			expectedKey:   "commands",
			expectedValue: "", // Just check that commands key exists
			description:   "Should load .atmos.d from git root when running from subdirectory",
		},
		{
			name: "git_root_atmos_d_has_lower_priority_than_config_dir",
			setupDirs: func(t *testing.T, tempDir string) string {
				// Create .atmos.d at "git root" (tempDir) with lower priority setting.
				gitRootAtmosDDir := filepath.Join(tempDir, ".atmos.d")
				require.NoError(t, os.MkdirAll(gitRootAtmosDDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(gitRootAtmosDDir, "settings.yaml"),
					[]byte(`settings:
  test_priority: from_git_root
`),
					0o644,
				))

				// Create config directory with atmos.yaml.
				configDir := filepath.Join(tempDir, "config")
				require.NoError(t, os.MkdirAll(configDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(configDir, "atmos.yaml"),
					[]byte(`base_path: ./`),
					0o644,
				))

				// Create .atmos.d in config dir with higher priority setting.
				configAtmosDDir := filepath.Join(configDir, ".atmos.d")
				require.NoError(t, os.MkdirAll(configAtmosDDir, 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(configAtmosDDir, "settings.yaml"),
					[]byte(`settings:
  test_priority: from_config_dir
`),
					0o644,
				))

				return configDir
			},
			expectedKey:   "settings.test_priority",
			expectedValue: "from_config_dir",
			description:   "Config dir .atmos.d should override git root .atmos.d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			workDir := tt.setupDirs(t, tempDir)

			// Mock git root to be tempDir.
			t.Setenv("TEST_GIT_ROOT", tempDir)

			// Change to working directory.
			t.Chdir(workDir)

			v := viper.New()
			v.SetConfigType("yaml")

			// Call mergeDefaultImports with the working directory.
			err := mergeDefaultImports(workDir, v)
			require.NoError(t, err, tt.description)

			// Verify the expected key is set.
			assert.True(t, v.IsSet(tt.expectedKey),
				"Expected key %q to be set. %s", tt.expectedKey, tt.description)

			if tt.expectedValue != "" {
				assert.Equal(t, tt.expectedValue, v.GetString(tt.expectedKey),
					"Expected value mismatch. %s", tt.description)
			}
		})
	}
}

func TestPreserveCaseSensitiveMaps(t *testing.T) {
	// Clear any previously tracked files at start of each test.
	resetMergedConfigFiles()

	t.Run("does nothing when no config file used", func(t *testing.T) {
		resetMergedConfigFiles()
		v := viper.New()
		atmosConfig := &schema.AtmosConfiguration{}

		preserveCaseSensitiveMaps(v, atmosConfig)
		assert.Nil(t, atmosConfig.CaseMaps)
	})

	t.Run("preserves env variable case", func(t *testing.T) {
		resetMergedConfigFiles()
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "atmos.yaml")

		// Create config with env section using mixed case.
		configContent := `
base_path: "."
env:
  GITHUB_TOKEN: "secret123"
  AWS_REGION: "us-east-1"
stacks:
  base_path: "stacks"
components:
  terraform:
    base_path: "components/terraform"
`
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		v := viper.New()
		v.SetConfigFile(configPath)
		err = v.ReadInConfig()
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{}

		preserveCaseSensitiveMaps(v, atmosConfig)
		assert.NotNil(t, atmosConfig.CaseMaps)

		// Check that case map was created for env.
		envCaseMap := atmosConfig.CaseMaps.Get("env")
		assert.NotNil(t, envCaseMap)
		assert.Equal(t, "GITHUB_TOKEN", envCaseMap["github_token"])
		assert.Equal(t, "AWS_REGION", envCaseMap["aws_region"])
	})

	t.Run("preserves auth identity case", func(t *testing.T) {
		resetMergedConfigFiles()
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "atmos.yaml")

		// Create config with auth.identities section using mixed case.
		configContent := `
base_path: "."
auth:
  identities:
    SuperAdmin:
      aws:
        profile: admin
    DevUser:
      aws:
        profile: dev
stacks:
  base_path: "stacks"
components:
  terraform:
    base_path: "components/terraform"
`
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		v := viper.New()
		v.SetConfigFile(configPath)
		err = v.ReadInConfig()
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{}

		preserveCaseSensitiveMaps(v, atmosConfig)
		assert.NotNil(t, atmosConfig.CaseMaps)

		// Check that case map was created for auth.identities.
		identityCaseMap := atmosConfig.CaseMaps.Get("auth.identities")
		assert.NotNil(t, identityCaseMap)
		assert.Equal(t, "SuperAdmin", identityCaseMap["superadmin"])
		assert.Equal(t, "DevUser", identityCaseMap["devuser"])

		// Check backward compatibility with IdentityCaseMap.
		assert.Equal(t, "SuperAdmin", atmosConfig.Auth.IdentityCaseMap["superadmin"])
		assert.Equal(t, "DevUser", atmosConfig.Auth.IdentityCaseMap["devuser"])
	})

	t.Run("merges case mappings from tracked imported files", func(t *testing.T) {
		resetMergedConfigFiles()
		tempDir := t.TempDir()

		// Create first import file with some env vars.
		importFile1 := filepath.Join(tempDir, "import1.yaml")
		import1Content := `
env:
  AWS_ACCESS_KEY_ID: "key1"
  AWS_SECRET_KEY: "secret1"
`
		err := os.WriteFile(importFile1, []byte(import1Content), 0o644)
		require.NoError(t, err)

		// Create second import file with additional env vars.
		importFile2 := filepath.Join(tempDir, "import2.yaml")
		import2Content := `
env:
  GITHUB_TOKEN: "token123"
  AWS_REGION: "us-east-1"
`
		err = os.WriteFile(importFile2, []byte(import2Content), 0o644)
		require.NoError(t, err)

		// Track both files as if they were merged during import processing.
		trackMergedConfigFile(importFile1)
		trackMergedConfigFile(importFile2)

		// Viper has no config file set, but we have tracked files.
		v := viper.New()
		atmosConfig := &schema.AtmosConfiguration{}

		preserveCaseSensitiveMaps(v, atmosConfig)
		assert.NotNil(t, atmosConfig.CaseMaps)

		// Check that case mappings from both files were merged.
		envCaseMap := atmosConfig.CaseMaps.Get("env")
		assert.NotNil(t, envCaseMap)
		assert.Equal(t, "AWS_ACCESS_KEY_ID", envCaseMap["aws_access_key_id"])
		assert.Equal(t, "AWS_SECRET_KEY", envCaseMap["aws_secret_key"])
		assert.Equal(t, "GITHUB_TOKEN", envCaseMap["github_token"])
		assert.Equal(t, "AWS_REGION", envCaseMap["aws_region"])
	})

	t.Run("later imports override earlier imports for overlapping keys", func(t *testing.T) {
		resetMergedConfigFiles()
		tempDir := t.TempDir()

		// Create first import file with an env var using one case.
		importFile1 := filepath.Join(tempDir, "import1.yaml")
		import1Content := `
env:
  my_token: "value1"
`
		err := os.WriteFile(importFile1, []byte(import1Content), 0o644)
		require.NoError(t, err)

		// Create second import file with the same key but different case.
		importFile2 := filepath.Join(tempDir, "import2.yaml")
		import2Content := `
env:
  MY_TOKEN: "value2"
`
		err = os.WriteFile(importFile2, []byte(import2Content), 0o644)
		require.NoError(t, err)

		// Track both files in order.
		trackMergedConfigFile(importFile1)
		trackMergedConfigFile(importFile2)

		v := viper.New()
		atmosConfig := &schema.AtmosConfiguration{}

		preserveCaseSensitiveMaps(v, atmosConfig)

		// The second file's case should win.
		envCaseMap := atmosConfig.CaseMaps.Get("env")
		assert.NotNil(t, envCaseMap)
		assert.Equal(t, "MY_TOKEN", envCaseMap["my_token"])
	})

	t.Run("main config file is included when not tracked", func(t *testing.T) {
		resetMergedConfigFiles()
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "atmos.yaml")

		// Create main config with env section.
		configContent := `
base_path: "."
env:
  MAIN_CONFIG_VAR: "main_value"
`
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		v := viper.New()
		v.SetConfigFile(configPath)
		err = v.ReadInConfig()
		require.NoError(t, err)

		// Don't track the main config file - it should still be included.
		atmosConfig := &schema.AtmosConfiguration{}

		preserveCaseSensitiveMaps(v, atmosConfig)

		envCaseMap := atmosConfig.CaseMaps.Get("env")
		assert.NotNil(t, envCaseMap)
		assert.Equal(t, "MAIN_CONFIG_VAR", envCaseMap["main_config_var"])
	})

	t.Run("skips unreadable files gracefully", func(t *testing.T) {
		resetMergedConfigFiles()
		tempDir := t.TempDir()

		// Create a valid import file.
		validFile := filepath.Join(tempDir, "valid.yaml")
		validContent := `
env:
  VALID_VAR: "valid_value"
`
		err := os.WriteFile(validFile, []byte(validContent), 0o644)
		require.NoError(t, err)

		// Track a non-existent file and the valid file.
		trackMergedConfigFile(filepath.Join(tempDir, "nonexistent.yaml"))
		trackMergedConfigFile(validFile)

		v := viper.New()
		atmosConfig := &schema.AtmosConfiguration{}

		// Should skip the unreadable file gracefully.
		preserveCaseSensitiveMaps(v, atmosConfig)

		// The valid file's mappings should still be preserved.
		envCaseMap := atmosConfig.CaseMaps.Get("env")
		assert.NotNil(t, envCaseMap)
		assert.Equal(t, "VALID_VAR", envCaseMap["valid_var"])
	})

	t.Run("preserves auth identities from imported files", func(t *testing.T) {
		resetMergedConfigFiles()
		tempDir := t.TempDir()

		// Create import file with auth identities.
		importFile := filepath.Join(tempDir, "auth-import.yaml")
		importContent := `
auth:
  identities:
    ImportedAdmin:
      aws:
        profile: imported-admin
    ImportedDev:
      aws:
        profile: imported-dev
`
		err := os.WriteFile(importFile, []byte(importContent), 0o644)
		require.NoError(t, err)

		trackMergedConfigFile(importFile)

		v := viper.New()
		atmosConfig := &schema.AtmosConfiguration{}

		preserveCaseSensitiveMaps(v, atmosConfig)

		// Check that auth.identities case map was created.
		identityCaseMap := atmosConfig.CaseMaps.Get("auth.identities")
		assert.NotNil(t, identityCaseMap)
		assert.Equal(t, "ImportedAdmin", identityCaseMap["importedadmin"])
		assert.Equal(t, "ImportedDev", identityCaseMap["importeddev"])

		// Check backward compatibility.
		assert.Equal(t, "ImportedAdmin", atmosConfig.Auth.IdentityCaseMap["importedadmin"])
		assert.Equal(t, "ImportedDev", atmosConfig.Auth.IdentityCaseMap["importeddev"])
	})
}

func TestParseProfilesFromOsArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no profile flag",
			args:     []string{"atmos", "describe", "stacks"},
			expected: nil,
		},
		{
			name:     "single profile with equals syntax",
			args:     []string{"atmos", "--profile=dev", "describe", "stacks"},
			expected: []string{"dev"},
		},
		{
			name:     "single profile with space syntax",
			args:     []string{"atmos", "--profile", "dev", "describe", "stacks"},
			expected: []string{"dev"},
		},
		{
			name:     "multiple profiles comma-separated",
			args:     []string{"atmos", "--profile=dev,staging", "describe", "stacks"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "multiple profile flags",
			args:     []string{"atmos", "--profile=dev", "--profile=staging", "describe", "stacks"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "profile with whitespace",
			args:     []string{"atmos", "--profile=  dev  ", "describe", "stacks"},
			expected: []string{"dev"},
		},
		{
			name:     "empty profile value",
			args:     []string{"atmos", "--profile=", "describe", "stacks"},
			expected: nil,
		},
		{
			name:     "profile with other flags",
			args:     []string{"atmos", "--stack=mystack", "--profile=dev", "--format=json", "describe", "stacks"},
			expected: []string{"dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProfilesFromOsArgs(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseViperProfilesFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		profiles []string
		expected []string
	}{
		{
			name:     "empty slice",
			profiles: []string{},
			expected: nil,
		},
		{
			name:     "single profile",
			profiles: []string{"dev"},
			expected: []string{"dev"},
		},
		{
			name:     "comma-separated as single string (Viper quirk)",
			profiles: []string{"dev,staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "whitespace-separated by Viper",
			profiles: []string{"dev", "staging", "prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "mixed comma and whitespace (Viper quirk)",
			profiles: []string{"dev", ",", "staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "profiles with whitespace",
			profiles: []string{"  dev  ", "  staging  "},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "empty strings and commas",
			profiles: []string{"", ",", "  ", "dev"},
			expected: []string{"dev"},
		},
		{
			name:     "nil input",
			profiles: nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseViperProfilesFromEnv(tt.profiles)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseProfilesFromEnvString(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "empty string",
			envValue: "",
			expected: nil,
		},
		{
			name:     "single profile",
			envValue: "dev",
			expected: []string{"dev"},
		},
		{
			name:     "comma-separated profiles",
			envValue: "dev,staging,prod",
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "profiles with whitespace",
			envValue: "  dev  ,  staging  ,  prod  ",
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "empty entries filtered",
			envValue: "dev,,staging,  ,prod",
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "only commas",
			envValue: ",,,",
			expected: nil,
		},
		{
			name:     "only whitespace",
			envValue: "   ",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProfilesFromEnvString(tt.envValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}
