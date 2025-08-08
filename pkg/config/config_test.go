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

// TestInitCliConfig should initialize atmos configuration with the correct base path and atmos Config File Path.
// It should also check that the base path and atmos Config File Path are correctly set and directory.
func TestInitCliConfig(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	assert.NoError(t, err, "Unset 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")
	err = os.Unsetenv("ATMOS_BASE_PATH")
	assert.NoError(t, err, "Unset 'ATMOS_BASE_PATH' environment variable should execute without error")
	err = os.Unsetenv("ATMOS_LOGS_LEVEL")
	assert.NoError(t, err, "Unset 'ATMOS_LOGS_LEVEL' environment variable should execute without error")

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
	includeConfig := `
base_path: "./"

components: !include config/component.yaml

logs:
  file: "/dev/stderr"
  level: Info`
	componentContent := `
terraform:
  base_path: "components/terraform"
  apply_auto_approve: true
  append_user_agent: test !include config/component.yaml
  deploy_run_init: true
  init_run_reconfigure: true
  auto_generate_backend_file: true`
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
			name:           "valid configuration file name atmos.yaml extension yaml",
			configFileName: "atmos.yaml",
			configContent:  configContent,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
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
			name:           "invalid config file name. Should fallback to default configuration",
			configFileName: "config.yaml",
			configContent:  configContent,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
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
			name: "valid process Stacks",
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, "../../examples/demo-stacks")
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
			name:           "invalid process Stacks,should return error",
			configFileName: "atmos.yaml",
			configContent:  configContent,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
			},
			processStacks: true,
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.Error(t, err)
			},
		},
		{
			name:           "environment variable interpolation YAML function env (AtmosYamlFuncEnv)",
			configFileName: "atmos.yaml",
			configContent:  `base_path: !env TEST_ATMOS_BASE_PATH`,
			envSetup: func(t *testing.T) func() {
				os.Setenv("TEST_ATMOS_BASE_PATH", "env/test/path")
				return func() { os.Unsetenv("TEST_ATMOS_BASE_PATH") }
			},
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "env/test/path", cfg.BasePath)
			},
		},
		{
			name:           "environment variable AtmosYamlFuncEnv return default value",
			configFileName: "atmos.yaml",
			configContent:  `base_path: !env NOT_EXIST_VAR env/test/path`,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "env/test/path", cfg.BasePath)
			},
		},
		{
			name:           "environment variable AtmosYamlFuncEnv should return error",
			configFileName: "atmos.yaml",
			configContent:  `base_path: !env `,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrExecuteYamlFunctions)
			},
		},
		{
			name:           "shell command execution YAML function exec (AtmosYamlFuncExec)",
			configFileName: "atmos.yaml",
			configContent:  `base_path: !exec echo Hello, World!`,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "Hello, World!", strings.TrimSpace(cfg.BasePath))
			},
		},
		{
			name:           "execution YAML function include (AtmosYamlFuncInclude)",
			configFileName: "atmos.yaml",
			configContent:  includeConfig,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				err := os.Mkdir(filepath.Join(dir, "config"), 0o777)
				if err != nil {
					t.Fatal(err)
				}
				createConfigFile(t, filepath.Join(dir, "config"), "component.yaml", componentContent)
				changeWorkingDir(t, dir)
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "test !include config/component.yaml", cfg.Components.Terraform.AppendUserAgent)
				assert.Equal(t, true, cfg.Components.Terraform.ApplyAutoApprove)
				assert.Equal(t, true, cfg.Components.Terraform.DeployRunInit)
				assert.Equal(t, true, cfg.Components.Terraform.InitRunReconfigure)
				assert.Equal(t, true, cfg.Components.Terraform.AutoGenerateBackendFile)
				assert.Equal(t, "components/terraform", cfg.Components.Terraform.BasePath)
			},
		},
		{
			name:           "execution YAML function !repo-root AtmosYamlFuncGitRoot return default value",
			configFileName: "atmos.yaml",
			configContent:  `base_path: !repo-root /test/path`,
			setup: func(t *testing.T, dir string, tc testCase) {
				createConfigFile(t, dir, tc.configFileName, tc.configContent)
				changeWorkingDir(t, dir)
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				require.NoError(t, err)
				assert.Equal(t, "/test/path", cfg.BasePath)
			},
		},
		{
			name:           "execution YAML function !repo-root AtmosYamlFuncGitRoot return root path",
			configFileName: "atmos.yaml",
			configContent:  `base_path: !repo-root`,
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, "../../tests/fixtures/scenarios/atmos-repo-root-yaml-function")
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				cwd, errDir := os.Getwd()
				// expect dir four levels up of the current dir to resolve to the root of the git repo
				fourLevelsUp := filepath.Join(cwd, "..", "..", "..", "..")

				// Clean and get the absolute path
				absPath, errPath := filepath.Abs(fourLevelsUp)
				if errPath != nil {
					require.NoError(t, err)
				}
				require.NoError(t, errDir)
				require.NoError(t, err)
				assert.Equal(t, absPath, cfg.BasePath)
			},
		},
		{
			name: "valid import .atmos.d",
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, "../../tests/fixtures/scenarios/atmos-configuration")
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

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name:     "no flags",
			args:     []string{},
			expected: map[string]string{},
		},
		{
			name:     "single flag",
			args:     []string{"--key=value"},
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "multiple flags",
			args:     []string{"--key1=value1", "--key2=value2"},
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = test.args
			result := parseFlags()
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestSetLogConfig(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedLogLevel string
		expectetdNoColor bool
	}{
		{
			name:             "valid log level",
			args:             []string{"--logs-level", "Debug"},
			expectedLogLevel: "Debug",
		},
		{
			name:             "invalid log level",
			args:             []string{"--logs-level", "InvalidLevel"},
			expectedLogLevel: "InvalidLevel",
		},
		{
			name:             "No color flag",
			args:             []string{"--no-color"},
			expectedLogLevel: "",
			expectetdNoColor: true,
		},
		{
			name:             "No color flag with log level",
			args:             []string{"--no-color", "--logs-level", "Debug"},
			expectedLogLevel: "Debug",
			expectetdNoColor: true,
		},
		{
			name:             "No color flag disable with log level",
			args:             []string{"--no-color=false", "--logs-level", "Debug"},
			expectedLogLevel: "Debug",
			expectetdNoColor: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = test.args
			atmosConfig := &schema.AtmosConfiguration{}
			setLogConfig(atmosConfig)
			assert.Equal(t, test.expectedLogLevel, atmosConfig.Logs.Level)
		})
	}
}

func TestAtmosConfigAbsolutePaths(t *testing.T) {
	t.Run("should handle empty base paths", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{},
				Helmfile:  schema.Helmfile{},
			},
			Stacks: schema.Stacks{},
		}

		err := atmosConfigAbsolutePaths(config)
		assert.NoError(t, err)
	})

	t.Run("should handle absolute paths", func(t *testing.T) {
		absPath := filepath.Join(os.TempDir(), "atmos-test")
		config := &schema.AtmosConfiguration{
			BasePath: absPath,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: absPath,
				},
				Helmfile: schema.Helmfile{
					BasePath: absPath,
				},
			},
			Stacks: schema.Stacks{
				BasePath: absPath,
			},
		}

		err := atmosConfigAbsolutePaths(config)
		assert.NoError(t, err)

		// Check if absolute paths remain unchanged
		assert.Equal(t, absPath, config.Components.Terraform.BasePath)
		assert.Equal(t, absPath, config.Components.Helmfile.BasePath)
		assert.Equal(t, absPath, config.Stacks.BasePath)
	})
}
