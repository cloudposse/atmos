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
		{
			name: "valid import custom override",
			setup: func(t *testing.T, dir string, tc testCase) {
				changeWorkingDir(t, "../../tests/fixtures/scenarios/atmos-cli-imports-override")
			},
			assertions: func(t *testing.T, tempDirPath string, cfg *schema.AtmosConfiguration, err error) {
				assert.Equal(t, "foo", cfg.Commands[0].Name)
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
func TestParseFlagsForPager(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedPager   string
		expectedNoColor bool
	}{
		{
			name:            "no pager flag",
			args:            []string{"atmos", "describe", "config"},
			expectedPager:   "",
			expectedNoColor: false,
		},
		{
			name:            "pager flag without value",
			args:            []string{"atmos", "describe", "config", "--pager"},
			expectedPager:   "true",
			expectedNoColor: false,
		},
		{
			name:            "pager flag with true",
			args:            []string{"atmos", "describe", "config", "--pager=true"},
			expectedPager:   "true",
			expectedNoColor: false,
		},
		{
			name:            "pager flag with false",
			args:            []string{"atmos", "describe", "config", "--pager=false"},
			expectedPager:   "false",
			expectedNoColor: false,
		},
		{
			name:            "pager flag with less",
			args:            []string{"atmos", "describe", "config", "--pager=less"},
			expectedPager:   "less",
			expectedNoColor: false,
		},
		{
			name:            "no-color flag",
			args:            []string{"atmos", "describe", "config", "--no-color"},
			expectedPager:   "",
			expectedNoColor: true,
		},
		{
			name:            "both pager and no-color flags",
			args:            []string{"atmos", "describe", "config", "--pager=more", "--no-color"},
			expectedPager:   "more",
			expectedNoColor: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			// Set test args
			os.Args = tt.args

			// Parse flags
			flags := parseFlags()

			// Check pager flag
			if tt.expectedPager != "" {
				assert.Equal(t, tt.expectedPager, flags["pager"])
			} else {
				assert.Empty(t, flags["pager"])
			}

			// Check no-color flag
			if tt.expectedNoColor {
				assert.Equal(t, "true", flags["no-color"])
			} else {
				_, hasNoColor := flags["no-color"]
				assert.False(t, hasNoColor)
			}
		})
	}
}

func TestSetLogConfigWithPager(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedPager   string
		expectedNoColor bool
		expectedColor   bool
	}{
		{
			name:            "pager from flag",
			args:            []string{"atmos", "--pager=less"},
			expectedPager:   "less",
			expectedNoColor: false,
		},
		{
			name:            "no-color flag sets both NoColor and Color",
			args:            []string{"atmos", "--no-color"},
			expectedPager:   "",
			expectedNoColor: true,
			expectedColor:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original state
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			// Set test args
			if tt.args != nil {
				os.Args = tt.args
			}

			// Create a test config
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{},
				},
			}

			// Apply config
			setLogConfig(atmosConfig)

			// Verify results
			if tt.expectedPager != "" {
				assert.Equal(t, tt.expectedPager, atmosConfig.Settings.Terminal.Pager)
			}
			assert.Equal(t, tt.expectedNoColor, atmosConfig.Settings.Terminal.NoColor)
			if tt.expectedNoColor {
				assert.Equal(t, false, atmosConfig.Settings.Terminal.Color)
			}
		})
	}
}

func TestEnvironmentVariableHandling(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		args            []string
		expectedPager   string
		expectedNoColor bool
		expectedColor   bool
	}{
		{
			name: "ATMOS_PAGER environment variable",
			envVars: map[string]string{
				"ATMOS_PAGER": "more",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "more",
		},
		{
			name: "NO_COLOR environment variable",
			envVars: map[string]string{
				"NO_COLOR": "1",
			},
			args:            []string{"atmos", "describe", "config"},
			expectedNoColor: true,
			expectedColor:   false,
		},
		{
			name: "ATMOS_NO_COLOR environment variable",
			envVars: map[string]string{
				"ATMOS_NO_COLOR": "true",
			},
			args:            []string{"atmos", "describe", "config"},
			expectedNoColor: true,
			expectedColor:   false,
		},
		{
			name: "COLOR environment variable",
			envVars: map[string]string{
				"COLOR": "true",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedColor: true,
		},
		{
			name: "ATMOS_COLOR environment variable",
			envVars: map[string]string{
				"ATMOS_COLOR": "true",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedColor: true,
		},
		{
			name: "CLI flag overrides environment variable",
			envVars: map[string]string{
				"ATMOS_PAGER": "more",
			},
			args:          []string{"atmos", "--pager=less", "describe", "config"},
			expectedPager: "less",
		},
		{
			name: "Multiple environment variables with precedence",
			envVars: map[string]string{
				"COLOR":       "true",
				"NO_COLOR":    "1",
				"ATMOS_PAGER": "cat",
			},
			args:            []string{"atmos", "describe", "config"},
			expectedPager:   "cat",
			expectedNoColor: true,
			expectedColor:   false, // NO_COLOR takes precedence
		},
		{
			name: "PAGER environment variable fallback",
			envVars: map[string]string{
				"PAGER": "less -R",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "less -R",
		},
		{
			name: "ATMOS_PAGER takes precedence over PAGER",
			envVars: map[string]string{
				"PAGER":       "less",
				"ATMOS_PAGER": "more",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "more",
		},
		{
			name: "CLI flag --no-color=false overrides NO_COLOR env var",
			envVars: map[string]string{
				"NO_COLOR": "1",
			},
			args:            []string{"atmos", "--no-color=false", "describe", "config"},
			expectedNoColor: false,
			expectedColor:   true,
		},
		{
			name: "--pager=false overrides PAGER env var",
			envVars: map[string]string{
				"PAGER": "less",
			},
			args:          []string{"atmos", "--pager=false", "describe", "config"},
			expectedPager: "false",
		},
		{
			name: "--no-color=true explicitly sets NoColor",
			envVars: map[string]string{
				"COLOR": "true",
			},
			args:            []string{"atmos", "--no-color=true", "describe", "config"},
			expectedNoColor: true,
			expectedColor:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original state
			originalArgs := os.Args
			originalEnvVars := make(map[string]string)

			// Clear and save relevant environment variables
			envVarsToCheck := []string{"ATMOS_PAGER", "PAGER", "NO_COLOR", "ATMOS_NO_COLOR", "COLOR", "ATMOS_COLOR"}
			for _, envVar := range envVarsToCheck {
				if val, exists := os.LookupEnv(envVar); exists {
					originalEnvVars[envVar] = val
				}
				os.Unsetenv(envVar)
			}

			defer func() {
				// Restore original state
				os.Args = originalArgs
				// Restore environment variables
				for _, envVar := range envVarsToCheck {
					os.Unsetenv(envVar)
				}
				for envVar, val := range originalEnvVars {
					os.Setenv(envVar, val)
				}
			}()

			// Set test environment variables
			for envVar, val := range tt.envVars {
				os.Setenv(envVar, val)
			}

			// Set test args
			if tt.args != nil {
				os.Args = tt.args
			}

			// Create a test viper instance and bind environment variables
			v := viper.New()
			v.SetEnvPrefix("ATMOS")
			v.AutomaticEnv()

			// Bind specific environment variables
			v.BindEnv("settings.terminal.pager", "ATMOS_PAGER", "PAGER")
			v.BindEnv("settings.terminal.no_color", "ATMOS_NO_COLOR", "NO_COLOR")
			v.BindEnv("settings.terminal.color", "ATMOS_COLOR", "COLOR")

			// Create a test config
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{},
				},
			}

			// Apply environment variables to config
			if envPager := v.GetString("settings.terminal.pager"); envPager != "" {
				atmosConfig.Settings.Terminal.Pager = envPager
			}
			if v.IsSet("settings.terminal.no_color") {
				atmosConfig.Settings.Terminal.NoColor = v.GetBool("settings.terminal.no_color")
				// When NoColor is set, Color should be false
				if atmosConfig.Settings.Terminal.NoColor {
					atmosConfig.Settings.Terminal.Color = false
				}
			}
			if v.IsSet("settings.terminal.color") && !atmosConfig.Settings.Terminal.NoColor {
				// Only set Color if NoColor is not true
				atmosConfig.Settings.Terminal.Color = v.GetBool("settings.terminal.color")
			}

			// Apply CLI flags (simulating what setLogConfig does)
			setLogConfig(atmosConfig)

			// Verify results
			if tt.expectedPager != "" {
				assert.Equal(t, tt.expectedPager, atmosConfig.Settings.Terminal.Pager, "Pager setting mismatch")
			}
			assert.Equal(t, tt.expectedNoColor, atmosConfig.Settings.Terminal.NoColor, "NoColor setting mismatch")
			if tt.expectedColor || tt.expectedNoColor {
				// Only check Color field if we expect it to be set
				expectedColorValue := tt.expectedColor && !tt.expectedNoColor
				assert.Equal(t, expectedColorValue, atmosConfig.Settings.Terminal.Color, "Color setting mismatch")
			}
		})
	}
}
