package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// test configuration with flags --config and --config-path with multiple files and directories merge.
func TestLoadConfigFromCLIArgsMultipleMerge(t *testing.T) {
	// create tmp folder
	tmpDir := t.TempDir()
	// create atmos.yaml file
	atmosConfigFilePath := filepath.Join(tmpDir, "test-config.yaml")
	f, err := os.Create(atmosConfigFilePath)
	if err != nil {
		t.Fatalf("Failed to create atmos.yaml file: %v", err)
	}
	content := []string{
		"logs:\n",
		"  file: /dev/stderr\n",
		"  level: Warning",
	}

	for _, line := range content {
		if _, err := f.WriteString(line); err != nil {
			t.Fatalf("Failed to write to config file: %v", err)
		}
	}
	f.Close()
	// write another config file
	tmpDir2 := t.TempDir()
	atmosConfigFilePath2 := filepath.Join(tmpDir2, "atmos.yaml")
	f2, err := os.Create(atmosConfigFilePath2)
	if err != nil {
		t.Fatalf("Failed to create atmos.yaml file: %v", err)
	}
	content2 := []string{
		"logs:\n",
		"  file: /dev/stderr\n",
		"  level: Debug",
	}
	for _, line := range content2 {
		if _, err := f2.WriteString(line); err != nil {
			t.Fatalf("Failed to write to config file: %v", err)
		}
	}
	f2.Close()

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{atmosConfigFilePath},
		AtmosConfigDirsFromArg:  []string{tmpDir2},
	}

	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		t.Fatalf("Failed to initialize atmos config: %v", err)
	}
	assert.Equal(t, atmosConfig.Logs.Level, "Debug", "Logs level should be Debug")
	assert.Equal(t, atmosConfig.Logs.File, "/dev/stderr", "Logs file should be /dev/stderr")
	assert.Equal(t, atmosConfig.CliConfigPath, connectPaths([]string{filepath.Dir(atmosConfigFilePath), tmpDir2}), "CliConfigPath should be the concatenation of config files and directories")
}

func TestLoadConfigFromCLIArgs(t *testing.T) {
	// Setup valid configuration for base case
	validDir := t.TempDir()
	validConfig := `
logs:
  file: /dev/stderr
  level: Warning
`
	validPath := filepath.Join(validDir, "atmos.yaml")
	if err := os.WriteFile(validPath, []byte(validConfig), 0o644); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	testCases := []struct {
		name          string
		files         []string
		dirs          []string
		expectError   bool
		expectedLevel string
	}{
		{
			name:          "valid configuration paths",
			files:         []string{validPath},
			dirs:          []string{validDir},
			expectError:   false,
			expectedLevel: "Warning",
		},
		{
			name:        "invalid config file path",
			files:       []string{"/non/existent/file.yaml"},
			dirs:        []string{validDir},
			expectError: true,
		},
		{
			name:        "invalid config directory path",
			files:       []string{validPath},
			dirs:        []string{"/non/existent/directory"},
			expectError: true,
		},
		{
			name:        "both invalid file and directory",
			files:       []string{"/non/existent/file.yaml"},
			dirs:        []string{"/non/existent/directory"},
			expectError: true,
		},
		{
			name:        "mixed valid and invalid paths",
			files:       []string{"/non/existent/file.yaml", validPath},
			dirs:        []string{"/non/existent/directory", validDir},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configInfo := schema.ConfigAndStacksInfo{
				AtmosConfigFilesFromArg: tc.files,
				AtmosConfigDirsFromArg:  tc.dirs,
			}

			cfg, err := InitCliConfig(configInfo, false)

			if tc.expectError {
				require.Error(t, err, "Expected error for invalid paths")
				return
			}

			require.NoError(t, err, "Valid config should not return error")
			assert.Equal(t, tc.expectedLevel, cfg.Logs.Level, "Unexpected log level configuration")
			assert.Equal(t, "/dev/stderr", cfg.Logs.File, "Unexpected log file configuration")
		})
	}
}

func TestConnectPaths_WindowsPaths(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected string
	}{
		{
			name:     "Single path",
			paths:    []string{"C:\\Program Files"},
			expected: "C:\\Program Files",
		},
		{
			name:     "Multiple paths",
			paths:    []string{"C:\\Program Files", "D:\\Games", "E:\\Projects"},
			expected: "C:\\Program Files;D:\\Games;E:\\Projects;", // trailing semicolon present
		},
		{
			name:     "Empty slice",
			paths:    []string{},
			expected: "",
		},
		{
			name:     "Multiple empty paths",
			paths:    []string{"", "", ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := connectPaths(tt.paths)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestValidatedIsFiles_EdgeCases tests edge cases for file validation.
func TestValidatedIsFiles_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "test.yaml")
	require.NoError(t, os.WriteFile(validFile, []byte("test: value"), 0o644))

	tests := []struct {
		name      string
		files     []string
		wantError string
		checkErr  error
	}{
		{
			name:      "empty file path",
			files:     []string{""},
			wantError: "requires a non-empty file path",
			checkErr:  errUtils.ErrEmptyConfigFile,
		},
		{
			name:      "directory instead of file",
			files:     []string{tmpDir},
			wantError: "",
			checkErr:  errUtils.ErrExpectedFile,
		},
		{
			name:      "multiple files with one invalid",
			files:     []string{validFile, ""},
			wantError: "requires a non-empty file path",
			checkErr:  errUtils.ErrEmptyConfigFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatedIsFiles(tt.files)
			assert.Error(t, err)
			if tt.wantError != "" {
				assert.Contains(t, err.Error(), tt.wantError)
			}
			if tt.checkErr != nil {
				assert.ErrorIs(t, err, tt.checkErr)
			}
		})
	}
}

// TestValidatedIsDirs_EdgeCases tests edge cases for directory validation.
func TestValidatedIsDirs_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	validDir := filepath.Join(tmpDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))

	testFile := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	tests := []struct {
		name      string
		dirs      []string
		wantError string
		checkErr  error
	}{
		{
			name:      "empty directory path",
			dirs:      []string{""},
			wantError: "requires a non-empty directory path",
			checkErr:  errUtils.ErrEmptyConfigPath,
		},
		{
			name:      "file instead of directory",
			dirs:      []string{testFile},
			wantError: "requires a directory but found a file",
			checkErr:  errUtils.ErrAtmosDirConfigNotFound,
		},
		{
			name:      "non-existent directory",
			dirs:      []string{filepath.Join(tmpDir, "nonexistent")},
			wantError: "does not exist",
			checkErr:  errUtils.ErrAtmosDirConfigNotFound,
		},
		{
			name:      "multiple dirs with one invalid",
			dirs:      []string{validDir, ""},
			wantError: "requires a non-empty directory path",
			checkErr:  errUtils.ErrEmptyConfigPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatedIsDirs(tt.dirs)
			assert.Error(t, err)
			if tt.wantError != "" {
				assert.Contains(t, err.Error(), tt.wantError)
			}
			if tt.checkErr != nil {
				assert.ErrorIs(t, err, tt.checkErr)
			}
		})
	}
}

// TestMergeConfigFromDirectories_ConfigFileVariants tests finding both atmos.yaml and .atmos.yaml.
func TestMergeConfigFromDirectories_ConfigFileVariants(t *testing.T) {
	tmpDir := t.TempDir()

	// Directory with atmos.yaml
	dir1 := filepath.Join(tmpDir, "dir1")
	require.NoError(t, os.Mkdir(dir1, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir1, "atmos.yaml"),
		[]byte("base_path: /test1"),
		0o644,
	))

	// Directory with .atmos.yaml
	dir2 := filepath.Join(tmpDir, "dir2")
	require.NoError(t, os.Mkdir(dir2, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir2, ".atmos.yaml"),
		[]byte("base_path: /test2"),
		0o644,
	))

	// Directory with no config file
	emptyDir := filepath.Join(tmpDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	tests := []struct {
		name         string
		dirs         []string
		expectError  bool
		expectedDirs int
	}{
		{
			name:         "finds atmos.yaml",
			dirs:         []string{dir1},
			expectError:  false,
			expectedDirs: 1,
		},
		{
			name:         "finds .atmos.yaml",
			dirs:         []string{dir2},
			expectError:  false,
			expectedDirs: 1,
		},
		{
			name:         "finds both variants in different directories",
			dirs:         []string{dir1, dir2},
			expectError:  false,
			expectedDirs: 2,
		},
		{
			name:        "fails for directory without config",
			dirs:        []string{emptyDir},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configInfo := schema.ConfigAndStacksInfo{
				AtmosConfigDirsFromArg: tt.dirs,
			}

			_, err := InitCliConfig(configInfo, false)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestLoadConfigFromCLIArgs_ErrorPaths tests error paths in config loading.
func TestLoadConfigFromCLIArgs_ErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid YAML file
	invalidYaml := filepath.Join(tmpDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidYaml, []byte("invalid: [unclosed"), 0o644))

	tests := []struct {
		name        string
		files       []string
		dirs        []string
		expectError bool
		description string
	}{
		{
			name:        "invalid YAML syntax",
			files:       []string{invalidYaml},
			dirs:        []string{},
			expectError: true,
			description: "Should fail for invalid YAML syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configInfo := schema.ConfigAndStacksInfo{
				AtmosConfigFilesFromArg: tt.files,
				AtmosConfigDirsFromArg:  tt.dirs,
			}

			_, err := InitCliConfig(configInfo, false)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}
