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
				t.Setenv("TEST_ATMOS_BASE_PATH", "env/test/path")
				return func() {} // t.Setenv automatically restores the value
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
			// Use parseFlagsFromArgs to avoid os.Args manipulation.
			result := parseFlagsFromArgs(test.args)
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
			// setLogConfig() calls parseFlags() which reads os.Args.
			// We manipulate os.Args here to test the integration.
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

		err := AtmosConfigAbsolutePaths(config)
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

		err := AtmosConfigAbsolutePaths(config)
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
	t.Chdir(dir)
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
			// Use parseFlagsFromArgs to avoid os.Args manipulation.
			flags := parseFlagsFromArgs(tt.args)

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
			// setLogConfig() calls parseFlags() which reads os.Args.
			// We manipulate os.Args here to test the integration.
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
		{
			name: "NO_PAGER environment variable disables pager",
			envVars: map[string]string{
				"NO_PAGER": "1",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "false",
		},
		{
			name: "NO_PAGER=true disables pager",
			envVars: map[string]string{
				"NO_PAGER": "true",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "false",
		},
		{
			name: "NO_PAGER=any_value disables pager",
			envVars: map[string]string{
				"NO_PAGER": "yes",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "false",
		},
		{
			name: "--pager flag overrides NO_PAGER env var",
			envVars: map[string]string{
				"NO_PAGER": "1",
			},
			args:          []string{"atmos", "--pager=less", "describe", "config"},
			expectedPager: "less",
		},
		{
			name: "NO_PAGER takes precedence over PAGER env var",
			envVars: map[string]string{
				"PAGER":    "more",
				"NO_PAGER": "1",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "false",
		},
		{
			name: "NO_PAGER takes precedence over ATMOS_PAGER env var",
			envVars: map[string]string{
				"ATMOS_PAGER": "less",
				"NO_PAGER":    "1",
			},
			args:          []string{"atmos", "describe", "config"},
			expectedPager: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setLogConfig() calls parseFlags() which reads os.Args.
			// We manipulate os.Args here to test the integration.
			originalArgs := os.Args
			originalEnvVars := make(map[string]string)

			// Clear and save relevant environment variables
			envVarsToCheck := []string{"ATMOS_PAGER", "PAGER", "NO_PAGER", "NO_COLOR", "ATMOS_NO_COLOR", "COLOR", "ATMOS_COLOR"}
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
				t.Setenv(envVar, val)
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

// TestResolveAbsolutePath tests the path resolution logic for different scenarios.
func TestResolveAbsolutePath(t *testing.T) {
	// Create a temp directory structure for testing.
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config", "subdir")
	err := os.MkdirAll(configDir, 0o755)
	require.NoError(t, err)

	// Create platform-neutral absolute paths for testing.
	absPath1 := filepath.Join(tmpDir, "absolute", "path")
	absPath2 := filepath.Join(tmpDir, "another", "absolute")

	tests := []struct {
		name          string
		path          string
		cliConfigPath string
		expectedBase  string // Expected base directory for resolution
	}{
		// Absolute paths - should always remain unchanged.
		{
			name:          "absolute path remains unchanged",
			path:          absPath1,
			cliConfigPath: configDir,
			expectedBase:  "", // N/A - absolute paths don't need base
		},
		{
			name:          "absolute path with empty config path remains unchanged",
			path:          absPath2,
			cliConfigPath: "",
			expectedBase:  "", // N/A - absolute paths don't need base
		},

		// Simple relative paths - should resolve to CWD for backward compatibility.
		{
			name:          "simple relative path resolves to CWD",
			path:          "stacks",
			cliConfigPath: configDir,
			expectedBase:  "cwd",
		},
		{
			name:          "complex relative path without ./ prefix resolves to CWD",
			path:          "components/terraform",
			cliConfigPath: configDir,
			expectedBase:  "cwd",
		},
		{
			name:          "deeply nested relative path resolves to CWD",
			path:          "a/b/c/d/e",
			cliConfigPath: configDir,
			expectedBase:  "cwd",
		},
		{
			name:          "simple relative path with empty config path resolves to CWD",
			path:          "stacks",
			cliConfigPath: "",
			expectedBase:  "cwd",
		},

		// Empty path - should resolve to CWD for backward compatibility (issue #1858).
		{
			name:          "empty path resolves to CWD",
			path:          "",
			cliConfigPath: configDir,
			expectedBase:  "cwd",
		},
		{
			name:          "empty path with empty config path resolves to CWD",
			path:          "",
			cliConfigPath: "",
			expectedBase:  "cwd",
		},

		// Single dot (.) - should resolve to config dir (explicit current directory reference).
		{
			name:          "dot path resolves to config dir (explicit current directory reference)",
			path:          ".",
			cliConfigPath: configDir,
			expectedBase:  "config",
		},
		{
			name:          "dot path with empty config path resolves to CWD",
			path:          ".",
			cliConfigPath: "",
			expectedBase:  "cwd",
		},

		// Paths starting with "./" - should resolve to config dir.
		{
			name:          "path starting with ./ resolves to config dir",
			path:          "./subpath",
			cliConfigPath: configDir,
			expectedBase:  "config",
		},
		{
			name:          "path ./ alone resolves to config dir",
			path:          "./",
			cliConfigPath: configDir,
			expectedBase:  "config",
		},
		{
			name:          "path starting with ./ and nested dirs resolves to config dir",
			path:          "./a/b/c",
			cliConfigPath: configDir,
			expectedBase:  "config",
		},
		{
			name:          "path starting with ./ with empty config path resolves to CWD",
			path:          "./subpath",
			cliConfigPath: "",
			expectedBase:  "cwd",
		},

		// Paths starting with ".." - should resolve to config dir.
		{
			name:          "path with parent dir (..) resolves to config dir",
			path:          "..",
			cliConfigPath: configDir,
			expectedBase:  "config",
		},
		{
			name:          "path starting with ../ resolves to config dir",
			path:          "../sibling",
			cliConfigPath: configDir,
			expectedBase:  "config",
		},
		{
			name:          "path with multiple parent traversals resolves to config dir",
			path:          "../../grandparent",
			cliConfigPath: configDir,
			expectedBase:  "config",
		},
		{
			name:          "path starting with .. with empty config path resolves to CWD",
			path:          "../sibling",
			cliConfigPath: "",
			expectedBase:  "cwd",
		},

		// Edge cases - paths that look like they might start with . or .. but don't.
		{
			name:          "path starting with dot but not ./ or .. resolves to CWD",
			path:          ".hidden",
			cliConfigPath: configDir,
			expectedBase:  "cwd",
		},
		{
			name:          "path starting with ..foo resolves to CWD (not parent traversal)",
			path:          "..foo",
			cliConfigPath: configDir,
			expectedBase:  "cwd", // "..foo" is NOT ".." or "../" so it resolves to CWD
		},
		{
			name:          "path with dots in middle resolves to CWD",
			path:          "foo/bar/../baz",
			cliConfigPath: configDir,
			expectedBase:  "cwd",
		},
		{
			name:          "path starting with ... resolves to CWD (not parent traversal)",
			path:          ".../something",
			cliConfigPath: configDir,
			expectedBase:  "cwd", // ".../something" is NOT "../" so it resolves to CWD
		},
	}

	// Add platform-specific test cases for Windows-style paths.
	// On Windows, backslash is the path separator so .\\ and ..\\ are explicit relative paths.
	// On Unix, backslash is a literal character so .\\ paths are just regular paths (resolve to CWD).
	if filepath.Separator == '\\' {
		// Windows: backslash paths should resolve to config dir.
		windowsTests := []struct {
			name          string
			path          string
			cliConfigPath string
			expectedBase  string
		}{
			{
				name:          "Windows-style .\\subpath resolves to config dir",
				path:          ".\\subpath",
				cliConfigPath: configDir,
				expectedBase:  "config",
			},
			{
				name:          "Windows-style ..\\sibling resolves to config dir",
				path:          "..\\sibling",
				cliConfigPath: configDir,
				expectedBase:  "config",
			},
			{
				name:          "Windows-style .\\subpath with empty config path resolves to CWD",
				path:          ".\\subpath",
				cliConfigPath: "",
				expectedBase:  "cwd",
			},
			{
				name:          "Windows-style ..\\sibling with empty config path resolves to CWD",
				path:          "..\\sibling",
				cliConfigPath: "",
				expectedBase:  "cwd",
			},
			{
				name:          "Windows-style .\\nested\\path resolves to config dir",
				path:          ".\\nested\\path",
				cliConfigPath: configDir,
				expectedBase:  "config",
			},
			{
				name:          "Windows-style ..\\..\\parent resolves to config dir",
				path:          "..\\..\\parent",
				cliConfigPath: configDir,
				expectedBase:  "config",
			},
		}
		tests = append(tests, windowsTests...)
	} else {
		// Unix: backslash is a literal character, so these paths resolve to CWD.
		// This tests that we don't incorrectly treat backslash as a path separator on Unix.
		unixBackslashTests := []struct {
			name          string
			path          string
			cliConfigPath string
			expectedBase  string
		}{
			{
				name:          "Unix treats .\\subpath as literal (not path separator), resolves to CWD",
				path:          ".\\subpath",
				cliConfigPath: configDir,
				expectedBase:  "cwd",
			},
			{
				name:          "Unix treats ..\\sibling as literal (not path separator), resolves to CWD",
				path:          "..\\sibling",
				cliConfigPath: configDir,
				expectedBase:  "cwd",
			},
		}
		tests = append(tests, unixBackslashTests...)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveAbsolutePath(tt.path, tt.cliConfigPath)
			require.NoError(t, err)

			if filepath.IsAbs(tt.path) {
				// Absolute paths should remain unchanged.
				assert.Equal(t, tt.path, result)
			} else {
				cwd, err := os.Getwd()
				require.NoError(t, err)

				switch tt.expectedBase {
				case "cwd":
					// Path should be resolved relative to CWD.
					expected := filepath.Join(cwd, tt.path)
					expectedAbs, err := filepath.Abs(expected)
					require.NoError(t, err)
					assert.Equal(t, expectedAbs, result,
						"Path %q should resolve relative to CWD", tt.path)
				case "config":
					// Path should be resolved relative to config dir.
					expected := filepath.Join(tt.cliConfigPath, tt.path)
					expectedAbs, err := filepath.Abs(expected)
					require.NoError(t, err)
					assert.Equal(t, expectedAbs, result,
						"Path %q should resolve relative to config dir", tt.path)
				}
			}
		})
	}
}

// TestCliConfigPathRegression tests the scenario from GitHub issue #1858
// where atmos.yaml is in a subdirectory but references directories at the repo root.
// This was broken in v1.201.0 (PR #1774) when path resolution was changed to
// resolve relative paths relative to the atmos.yaml location instead of CWD.
func TestCliConfigPathRegression(t *testing.T) {
	// Clear environment variables that might interfere.
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)
	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	t.Run("config in subdirectory with base_path pointing to parent", func(t *testing.T) {
		// Change to the fixture directory.
		changeWorkingDir(t, "../../tests/fixtures/scenarios/cli-config-path")

		// Load config using the config subdirectory.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "./config")

		cfg, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		require.NoError(t, err, "InitCliConfig should succeed")

		// The stacks directory should be found at the repo root, not inside config/.
		// The config has base_path: ".." which should resolve relative to atmos.yaml location,
		// and stacks.base_path: "stacks" should then be relative to that base_path.
		cwd, err := os.Getwd()
		require.NoError(t, err)

		expectedStacksPath := filepath.Join(cwd, "stacks")
		assert.Equal(t, expectedStacksPath, cfg.StacksBaseAbsolutePath,
			"Stacks path should be at repo root (CWD), not inside config/")

		// TerraformDirAbsolutePath should be the absolute path of components/terraform.
		expectedComponentsPath := filepath.Join(cwd, "components", "terraform")
		assert.Equal(t, expectedComponentsPath, cfg.TerraformDirAbsolutePath,
			"Terraform components path should be at repo root (CWD), not inside config/")
	})

	t.Run("base_path with .. should resolve relative to atmos.yaml location", func(t *testing.T) {
		// This test verifies the intended behavior of PR #1774:
		// When base_path is "..", it should resolve relative to atmos.yaml location.
		changeWorkingDir(t, "../../tests/fixtures/scenarios/cli-config-path")

		t.Setenv("ATMOS_CLI_CONFIG_PATH", "./config")

		cfg, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		require.NoError(t, err)

		// base_path: ".." in config/atmos.yaml should resolve to the parent of config/,
		// which is the repo root.
		cwd, err := os.Getwd()
		require.NoError(t, err)

		// BasePathAbsolute should be the repo root (parent of config/).
		assert.Equal(t, cwd, cfg.BasePathAbsolute,
			"Base path should resolve to repo root (parent of config/)")
	})
}

// TestPathBasedComponentResolution tests the scenario for path-based component resolution
// where users run atmos commands from within a component directory using ATMOS_BASE_PATH="."
// with ATMOS_CLI_CONFIG_PATH pointing back to the repo root.
func TestPathBasedComponentResolution(t *testing.T) {
	// Clear environment variables that might interfere.
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)
	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	t.Run("ATMOS_BASE_PATH=. with ATMOS_CLI_CONFIG_PATH should resolve to config dir", func(t *testing.T) {
		// This test verifies path-based component resolution:
		// When running from a component directory with ATMOS_BASE_PATH="." and
		// ATMOS_CLI_CONFIG_PATH pointing to the repo root, the base path should
		// resolve to the repo root (where atmos.yaml is), not to CWD.
		changeWorkingDir(t, "../../tests/fixtures/scenarios/complete/components/terraform/top-level-component1")

		// Point to the repo root where atmos.yaml is located.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "../../..")
		// Set base_path to "." - this should resolve relative to CLI config path.
		t.Setenv("ATMOS_BASE_PATH", ".")

		cfg, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		require.NoError(t, err, "InitCliConfig should succeed")

		// Get the repo root (where atmos.yaml is).
		cwd, err := os.Getwd()
		require.NoError(t, err)
		repoRoot := filepath.Clean(filepath.Join(cwd, "..", "..", ".."))

		// BasePathAbsolute should be the repo root (where atmos.yaml is), not CWD.
		assert.Equal(t, repoRoot, cfg.BasePathAbsolute,
			"Base path with '.' should resolve to config dir (repo root), not CWD")

		// Stacks should be found at repo_root/stacks.
		expectedStacksPath := filepath.Join(repoRoot, "stacks")
		assert.Equal(t, expectedStacksPath, cfg.StacksBaseAbsolutePath,
			"Stacks path should be at repo root, not inside component dir")

		// Components should be found at repo_root/components/terraform.
		expectedComponentsPath := filepath.Join(repoRoot, "components", "terraform")
		assert.Equal(t, expectedComponentsPath, cfg.TerraformDirAbsolutePath,
			"Terraform components path should be at repo root, not inside component dir")
	})

	t.Run("empty ATMOS_BASE_PATH should resolve to CWD for backward compatibility", func(t *testing.T) {
		// This test verifies issue #1858 backward compatibility:
		// When ATMOS_BASE_PATH is empty (or not set), paths should resolve relative to CWD.
		changeWorkingDir(t, "../../tests/fixtures/scenarios/complete")

		// Don't set ATMOS_CLI_CONFIG_PATH - use default discovery.
		// Don't set ATMOS_BASE_PATH - should default to CWD.

		cfg, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		require.NoError(t, err, "InitCliConfig should succeed")

		cwd, err := os.Getwd()
		require.NoError(t, err)

		// BasePathAbsolute should be CWD (since base_path: "." in atmos.yaml).
		// Note: The fixture's atmos.yaml has base_path: "." which resolves to config dir.
		// Since we're running from the same dir as atmos.yaml, config dir == CWD.
		assert.Equal(t, cwd, cfg.BasePathAbsolute,
			"Base path should resolve to CWD when running from repo root")
	})
}
