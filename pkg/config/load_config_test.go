package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test configuration with flags --config and --config-path with multiple files and directories merge.
func TestLoadConfigFromCLIArgsMultipleMerge(t *testing.T) {
	// create tmp folder
	os.Unsetenv("ATMOS_LOGS_LEVEL")
	tmpDir, err := os.MkdirTemp("", "atmos-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	// create atmos.yaml file
	atmosConfigFilePath := filepath.Join(tmpDir, "test-config.yaml")
	f, err := os.Create(atmosConfigFilePath)
	if err != nil {
		t.Fatalf("Failed to create atmos.yaml file: %v", err)
	}
	content := []string{
		"logs:\n",
		"  file: /dev/stderr\n",
		"  level: Info",
	}

	for _, line := range content {
		if _, err := f.WriteString(line); err != nil {
			t.Fatalf("Failed to write to config file: %v", err)
		}
	}
	f.Close()
	// write another config file
	tmpDir2, err := os.MkdirTemp("", "atmos-config-test-2")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir2)
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
  level: Info
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
			expectedLevel: "Info",
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

func TestConnectPaths(t *testing.T) {
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
