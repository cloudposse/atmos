package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestApplyGitRootBasePath(t *testing.T) {
	tests := []struct {
		name           string
		config         schema.AtmosConfiguration
		env            map[string]string
		createLocalCfg bool   // Create atmos.yaml in current directory
		createLocalDir string // Create directory in current directory (e.g., ".atmos.d")
		expectChange   bool
		expectedPath   string
	}{
		{
			name: "local atmos.yaml exists - skip git root discovery",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: ".",
			},
			createLocalCfg: true,
			expectChange:   false,
			expectedPath:   ".",
		},
		{
			name: "local .atmos.d/ exists - skip git root discovery",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: ".",
			},
			createLocalDir: ".atmos.d",
			expectChange:   false,
			expectedPath:   ".",
		},
		{
			name: "local .atmos/ exists - skip git root discovery",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: ".",
			},
			createLocalDir: ".atmos",
			expectChange:   false,
			expectedPath:   ".",
		},
		{
			name: "local atmos.d/ exists - skip git root discovery",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: ".",
			},
			createLocalDir: "atmos.d",
			expectChange:   false,
			expectedPath:   ".",
		},
		{
			name: "local .atmos.yaml exists - skip git root discovery",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: ".",
			},
			createLocalCfg: false, // We'll create .atmos.yaml manually in test
			createLocalDir: "",
			expectChange:   false,
			expectedPath:   ".",
		},
		{
			name: "explicit base_path preserved",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: "/custom/path",
			},
			expectChange: false,
			expectedPath: "/custom/path",
		},
		{
			name: "disabled via environment variable",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: ".",
			},
			env: map[string]string{
				"ATMOS_GIT_ROOT_BASEPATH": "false",
			},
			expectChange: false,
			expectedPath: ".",
		},
		{
			name: "non-default config skipped",
			config: schema.AtmosConfiguration{
				Default:  false,
				BasePath: ".",
			},
			expectChange: false,
			expectedPath: ".",
		},
		{
			name: "empty base_path treated as default",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: "",
			},
			env: map[string]string{
				"ATMOS_GIT_ROOT_BASEPATH": "false",
			},
			expectChange: false,
			expectedPath: "",
		},
		{
			name: "git root discovery succeeds - happy path",
			config: schema.AtmosConfiguration{
				Default:  true,
				BasePath: ".",
			},
			env: map[string]string{
				"TEST_GIT_ROOT": "/mock/git/repo/root",
			},
			expectChange: true,
			expectedPath: "/mock/git/repo/root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tmpDir := t.TempDir()
			t.Chdir(tmpDir)

			// Create local atmos.yaml if test requires it
			if tt.createLocalCfg {
				err := os.WriteFile("atmos.yaml", []byte("base_path: .\n"), 0o644)
				require.NoError(t, err, "Failed to create local atmos.yaml")
			}

			// Special case for .atmos.yaml test
			if tt.name == "local .atmos.yaml exists - skip git root discovery" {
				err := os.WriteFile(".atmos.yaml", []byte("base_path: .\n"), 0o644)
				require.NoError(t, err, "Failed to create local .atmos.yaml")
			}

			// Create local directory if test requires it
			if tt.createLocalDir != "" {
				err := os.Mkdir(tt.createLocalDir, 0o755)
				require.NoError(t, err, "Failed to create local directory")
			}

			// Set environment variables
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			originalPath := tt.config.BasePath
			err := applyGitRootBasePath(&tt.config)
			assert.NoError(t, err)

			if tt.expectChange {
				assert.NotEqual(t, originalPath, tt.config.BasePath,
					"Expected base path to change from default")
			} else {
				assert.Equal(t, tt.expectedPath, tt.config.BasePath,
					"Base path should not change")
			}
		})
	}
}

func TestHasLocalAtmosConfig(t *testing.T) {
	tests := []struct {
		name          string
		createFiles   []string // Files to create
		createDirs    []string // Directories to create
		expectedFound bool
	}{
		{
			name:          "no config - returns false",
			expectedFound: false,
		},
		{
			name:          "atmos.yaml exists",
			createFiles:   []string{"atmos.yaml"},
			expectedFound: true,
		},
		{
			name:          ".atmos.yaml exists",
			createFiles:   []string{".atmos.yaml"},
			expectedFound: true,
		},
		{
			name:          ".atmos directory exists",
			createDirs:    []string{".atmos"},
			expectedFound: true,
		},
		{
			name:          ".atmos.d directory exists",
			createDirs:    []string{".atmos.d"},
			expectedFound: true,
		},
		{
			name:          "atmos.d directory exists",
			createDirs:    []string{"atmos.d"},
			expectedFound: true,
		},
		{
			name:          "multiple indicators exist",
			createFiles:   []string{"atmos.yaml"},
			createDirs:    []string{".atmos", ".atmos.d"},
			expectedFound: true,
		},
		{
			name:          "unrelated files don't trigger",
			createFiles:   []string{"README.md", "config.yaml"},
			createDirs:    []string{"components", "stacks"},
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tmpDir := t.TempDir()

			// Create files
			for _, file := range tt.createFiles {
				filePath := filepath.Join(tmpDir, file)
				err := os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err, "Failed to create file: %s", file)
			}

			// Create directories
			for _, dir := range tt.createDirs {
				dirPath := filepath.Join(tmpDir, dir)
				err := os.Mkdir(dirPath, 0o755)
				require.NoError(t, err, "Failed to create directory: %s", dir)
			}

			// Test hasLocalAtmosConfig
			result := hasLocalAtmosConfig(tmpDir)
			assert.Equal(t, tt.expectedFound, result,
				"hasLocalAtmosConfig returned unexpected result")
		})
	}
}

func TestHasLocalAtmosConfigWithInvalidPath(t *testing.T) {
	// Test with non-existent directory - should return false without error
	result := hasLocalAtmosConfig("/nonexistent/directory/that/does/not/exist")
	assert.False(t, result, "Should return false for non-existent directory")
}
