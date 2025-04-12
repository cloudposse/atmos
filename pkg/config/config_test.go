package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddConfig_AllTypes(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue interface{}
		flagValue    string
		envVar       string
		envValue     string
		expected     interface{}
	}{
		{
			name:         "string flag",
			key:          "app.name",
			defaultValue: "default-app",
			flagValue:    "cli-app",
			envVar:       "APP_NAME_ENV",
			envValue:     "env-app",
			expected:     "cli-app",
		},
		{
			name:         "int flag",
			key:          "app.port",
			defaultValue: 8080,
			flagValue:    "9090",
			envVar:       "APP_PORT_ENV",
			envValue:     "7070",
			expected:     9090,
		},
		{
			name:         "bool flag",
			key:          "app.debug",
			defaultValue: false,
			flagValue:    "true",
			envVar:       "APP_DEBUG_ENV",
			envValue:     "true",
			expected:     true,
		},
		{
			name:         "string slice flag",
			key:          "app.tags",
			defaultValue: []string{"dev"},
			flagValue:    "prod,staging",
			envVar:       "APP_TAGS_ENV",
			envValue:     "qa,uat",
			expected:     []string{"prod", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			cmd := &cobra.Command{Use: "test"}
			handler := &ConfigHandler{v: v}

			// Set env var
			if tt.envVar != "" {
				os.Setenv(tt.envVar, tt.envValue)
				defer os.Unsetenv(tt.envVar)
			}

			opts := &ConfigOptions{
				Key:          tt.key,
				DefaultValue: tt.defaultValue,
				Description:  "test",
				EnvVar:       tt.envVar,
			}

			handler.AddConfig(cmd, opts)

			// CLI flag
			flagName := strings.ReplaceAll(tt.key, ".", "-")
			cmd.SetArgs([]string{"--" + flagName + "=" + tt.flagValue})
			cmd.Execute()

			switch expected := tt.expected.(type) {
			case string:
				assert.Equal(t, expected, v.GetString(tt.key))
			case int:
				assert.Equal(t, expected, v.GetInt(tt.key))
			case bool:
				assert.Equal(t, expected, v.GetBool(tt.key))
			case []string:
				assert.Equal(t, expected, v.GetStringSlice(tt.key))
			default:
				t.Fatalf("unsupported type in test")
			}
		})
	}
}

// TestInitCliConfig should initialize atmos configuration with the correct base path and atmos Config File Path.
// It should also check that the base path and atmos Config File Path are correctly set and directory.
func TestInitCliConfig(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	assert.NoError(t, err, "Unset 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")
	err = os.Unsetenv("ATMOS_BASE_PATH")
	assert.NoError(t, err, "Unset 'ATMOS_BASE_PATH' environment variable should execute without error")
	err = os.Unsetenv("ATMOS_LOGS_LEVEL")
	assert.NoError(t, err, "Unset 'ATMOS_LOGS_LEVEL' environment variable should execute without error")
	log.SetLevel(log.DebugLevel)
	configContent := `
base_path: ./
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: true
    deploy_run_init: true
    init_run_reconfigure: true
    auto_generate_backend_file: false
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  excluded_paths:
    - "**/_defaults.yaml"
  name_pattern: "{stage}"	
vendor:  
  base_path: "./test-vendor.yaml"
logs:
  file: /dev/stderr
  level: Info
`
	type testCase struct {
		name           string
		configFileName string
		configContent  string
		envSetup       func(t *testing.T) func()
		setup          func(t *testing.T, dir string, tc testCase)
		assertions     func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error)
		processStacks  bool
	}

	testCases := []testCase{
		{
			name:           "invalid config file name. Should fallback to default configuration",
			configFileName: "config.yaml",
			configContent:  configContent,
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, dir)
				DefaultConfigHandler = New()
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				// check if the atmos config path is set to empty
				assert.Equal(t, "", cfg.CliConfigPath)
				// check if the base path is set correctly from the default value
				assert.Equal(t, ".", cfg.BasePath)
				// check if the apply auto approve is set correctly from the default value
				assert.Equal(t, false, cfg.Components.Terraform.ApplyAutoApprove)
			},
		},
		{
			name:           "valid configuration file name atmos.yaml extension yaml",
			configFileName: "atmos.yaml",
			configContent:  configContent,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
				DefaultConfigHandler = New()
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "./", cfg.BasePath)
				assert.Contains(t, cfg.CliConfigPath, tempDirPath)
				configPathInfo, err := os.Stat(cfg.CliConfigPath)
				require.NoError(t, err)
				assert.True(t, configPathInfo.IsDir())
				baseInfo, err := os.Stat(cfg.BasePath)
				require.NoError(t, err)
				assert.True(t, baseInfo.IsDir())
				// check if the vendor path is set correctly
				assert.Equal(t, "./test-vendor.yaml", cfg.Vendor.BasePath)
				// check if the apply auto approve is set correctly
				assert.Equal(t, true, cfg.Components.Terraform.ApplyAutoApprove)
			},
		},
		{
			name:           "valid configuration file name atmos.yml extension yml",
			configFileName: "atmos.yml",
			configContent:  configContent,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
				DefaultConfigHandler = New()
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "./", cfg.BasePath)
				assert.Contains(t, cfg.CliConfigPath, tempDirPath)
				configPathInfo, err := os.Stat(cfg.CliConfigPath)
				require.NoError(t, err)
				assert.True(t, configPathInfo.IsDir())
				baseInfo, err := os.Stat(cfg.BasePath)
				require.NoError(t, err)
				assert.True(t, baseInfo.IsDir())
			},
		},
		{
			name: "valid process Stacks",
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, "../../examples/demo-stacks")
				DefaultConfigHandler = New()
			},
			processStacks: true,
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "./", cfg.BasePath)
				assert.Contains(t, cfg.CliConfigPath, filepath.Join("examples", "demo-stacks"))
				baseInfo, err := os.Stat(cfg.BasePath)
				require.NoError(t, err)
				assert.True(t, baseInfo.IsDir())
			},
		},
		{
			name:           "environment variable interpolation",
			configFileName: "atmos.yaml",
			configContent:  `base_path: !env TEST_ATMOS_BASE_PATH`,
			envSetup: func(t *testing.T) func() {
				os.Setenv("TEST_ATMOS_BASE_PATH", "env/test/path")
				return func() { os.Unsetenv("TEST_ATMOS_BASE_PATH") }
			},
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
				DefaultConfigHandler = New()
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "env/test/path", cfg.BasePath)
			},
		},
		{
			name: "valid import .atmos.d",
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, "../../tests/fixtures/scenarios/atmos-configuration")
				DefaultConfigHandler = New()
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "./", cfg.BasePath)
				assert.Contains(t, cfg.CliConfigPath, filepath.Join("fixtures", "scenarios", "atmos-configuration"))
				baseInfo, err := os.Stat(cfg.BasePath)
				require.NoError(t, err)
				assert.True(t, baseInfo.IsDir())
				assert.Equal(t, "{dev}", cfg.Stacks.NamePattern)
			},
		},
		{
			name: "valid import custom",
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, "../../tests/fixtures/scenarios/atmos-cli-imports")
				DefaultConfigHandler = New()
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "./", cfg.BasePath)
				assert.Contains(t, cfg.CliConfigPath, filepath.Join("fixtures", "scenarios", "atmos-cli-imports"))
				baseInfo, err := os.Stat(cfg.BasePath)
				require.NoError(t, err)
				assert.True(t, baseInfo.IsDir())
				assert.Equal(t, "Debug", cfg.Logs.Level)
			},
		},
		{
			name:           "invalid process Stacks,should return error",
			configFileName: "atmos.yaml",
			configContent:  configContent,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
				DefaultConfigHandler = New()
			},
			processStacks: true,
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup temp directory
			tmpDir := t.TempDir()

			// Environment setup
			if tc.envSetup != nil {
				cleanup := tc.envSetup(t)
				defer cleanup()
			}

			// Test-specific setup
			if tc.setup != nil {
				tc.setup(t, tmpDir, tc)
			}

			// Run test
			cfg, err := InitCliConfig(schema.ConfigAndStacksInfo{}, tc.processStacks)

			// Assertions
			if tc.assertions != nil {
				tc.assertions(t, tmpDir, &cfg, err)
			}
		})
	}
}

// Helper functions.
func createConfigFile(t *testing.T, dir string, fileName string, content string) {
	path := filepath.Join(dir, fileName)
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err, "Failed to create config file")
}

func changeWorkingDir(t *testing.T, dir string) {
	cwd, err := os.Getwd()
	require.NoError(t, err, "Failed to get current directory")

	t.Cleanup(func() {
		err := os.Chdir(cwd)
		require.NoError(t, err, "Failed to restore working directory")
	})

	err = os.Chdir(dir)
	require.NoError(t, err, "Failed to change working directory")
	t.Log("Changed working directory to", dir)
}

func TestMergeConfig_ConfigFileNotFound(t *testing.T) {
	tempDir := t.TempDir() // Empty directory, no config file

	v := viper.New()
	err := mergeConfig(v, tempDir, CliConfigFileName, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Config File \"atmos\" Not Found")
}

func TestMergeConfig_MultipleConfigFilesMerge(t *testing.T) {
	tempDir := t.TempDir()
	content := `
base_path: ./
vendor:  
  base_path: "./test-vendor.yaml"
logs:
  file: /dev/stderr
  level: Debug`
	createConfigFile(t, tempDir, "atmos.yaml", content)
	v := viper.New()
	v.SetConfigType("yaml")
	err := mergeConfig(v, tempDir, CliConfigFileName, false)
	assert.NoError(t, err)
	assert.Equal(t, "./", v.GetString("base_path"))
	content2 := `
base_path: ./test
vendor:  
  base_path: "./test2-vendor.yaml"
`
	tempDir2 := t.TempDir()
	createConfigFile(t, tempDir2, "atmos.yml", content2)
	err = mergeConfig(v, tempDir2, CliConfigFileName, false)
	assert.NoError(t, err)
	assert.Equal(t, "./test", v.GetString("base_path"))
	assert.Equal(t, "./test2-vendor.yaml", v.GetString("vendor.base_path"))
	assert.Equal(t, "Debug", v.GetString("logs.level"))
	assert.Equal(t, filepath.Join(tempDir2, "atmos.yml"), v.ConfigFileUsed())
}

func TestMergeDefaultConfig(t *testing.T) {
	v := viper.New()

	err := mergeDefaultConfig(v)
	assert.Error(t, err, "cannot decode configuration: unable to determine config type")
	v.SetConfigType("yaml")
	err = mergeDefaultConfig(v)
	assert.NoError(t, err, "should not return error if config type is yaml")
}
