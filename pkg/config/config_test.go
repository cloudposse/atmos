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

func TestMergeConfig_ImportOverrideBehavior(t *testing.T) {
	// Test that the main config file's settings override imported settings.
	tempDir := t.TempDir()

	// Create an import file with a command.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	importContent := `
commands:
  - name: "imported-command"
    description: "This is from import"
settings:
  imported: true
  shared: "from-import"
`
	createConfigFile(t, importDir, "commands.yaml", importContent)

	// Create main config that imports the above file and overrides the command.
	mainContent := `
base_path: ./
import:
  - "./imports/commands.yaml"
commands:
  - name: "main-command"
    description: "This is from main"
settings:
  main: true
  shared: "from-main"
`
	createConfigFile(t, tempDir, "atmos.yaml", mainContent)

	v := viper.New()
	v.SetConfigType("yaml")
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)

	// Verify that main config overrides imports.
	commands := v.Get("commands")
	assert.NotNil(t, commands)

	// Verify that commands were replaced, not appended.
	commandsList, ok := commands.([]interface{})
	assert.True(t, ok, "commands should be a slice")
	assert.Equal(t, 1, len(commandsList), "should have exactly one command (imported commands replaced)")

	// Verify the single command is from the main config.
	if len(commandsList) > 0 {
		cmd, ok := commandsList[0].(map[string]interface{})
		assert.True(t, ok, "command should be a map")
		assert.Equal(t, "main-command", cmd["name"], "command should be from main config")
		assert.Equal(t, "This is from main", cmd["description"])
	}

	// The main config's settings should override imported settings.
	assert.Equal(t, "from-main", v.GetString("settings.shared"))
	assert.True(t, v.GetBool("settings.main"))
	// Note: settings.imported is NOT present because the entire settings section
	// from the main config replaces the imported settings section.
}

func TestMergeConfig_ImportDeepMerge(t *testing.T) {
	// Test that imports are deep merged at the top level, but sections are replaced.
	tempDir := t.TempDir()

	// Create an import file with various settings.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	importContent := `
base_path: /imported
vendor:
  base_path: /imported/vendor
  setting1: imported
logs:
  level: Debug
  file: /imported.log
`
	createConfigFile(t, importDir, "base.yaml", importContent)

	// Create main config that imports and partially overrides.
	mainContent := `
base_path: ./
import:
  - "./imports/base.yaml"
vendor:
  base_path: /main/vendor
  setting2: main
logs:
  level: Info
`
	createConfigFile(t, tempDir, "atmos.yaml", mainContent)

	v := viper.New()
	v.SetConfigType("yaml")
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)

	// base_path from main config should override import.
	assert.Equal(t, "./", v.GetString("base_path"))

	// vendor section is completely replaced by main config.
	assert.Equal(t, "/main/vendor", v.GetString("vendor.base_path"))
	assert.Equal(t, "main", v.GetString("vendor.setting2"))
	assert.False(t, v.IsSet("vendor.setting1"), "vendor.setting1 should not exist (section replaced)")

	// logs section is completely replaced by main config.
	assert.Equal(t, "Info", v.GetString("logs.level"))
	assert.False(t, v.IsSet("logs.file"), "logs.file should not exist (section replaced)")
}

func TestMergeConfig_ProcessImportsWithInvalidYAML(t *testing.T) {
	// Test error handling when import file contains invalid YAML.
	tempDir := t.TempDir()

	// Create an import file with invalid YAML.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	// Write invalid YAML content directly.
	invalidYAMLPath := filepath.Join(importDir, "invalid.yaml")
	err = os.WriteFile(invalidYAMLPath, []byte("invalid: yaml: content:\n  - with bad indentation\n    and broken structure"), 0o644)
	require.NoError(t, err)

	// Create main config that tries to import the invalid file.
	mainContent := `
base_path: ./
import:
  - "./imports/invalid.yaml"
`
	createConfigFile(t, tempDir, "atmos.yaml", mainContent)

	v := viper.New()
	v.SetConfigType("yaml")
	// This should still succeed as invalid imports are logged but not fatal.
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)
}

func TestMergeConfig_EmptyConfig(t *testing.T) {
	// Test mergeConfig with an empty config file to ensure edge case coverage.
	tempDir := t.TempDir()

	// Create an empty config file.
	configPath := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(""), 0o644)
	require.NoError(t, err)

	v := viper.New()
	v.SetConfigType("yaml")

	// This should succeed even with an empty file.
	err = mergeConfig(v, tempDir, CliConfigFileName, false)
	assert.NoError(t, err)
}

func TestMergeConfig_ComplexImportHierarchy(t *testing.T) {
	// Test complex import hierarchy to improve coverage of import processing.
	tempDir := t.TempDir()

	// Create a chain of imports: A imports B, B imports C.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	// Create C (base config).
	configC := `
base_path: /from-c
settings:
  level: 3
  from_c: true
`
	createConfigFile(t, importDir, "c.yaml", configC)

	// Create B (imports C).
	configB := `
import:
  - "./c.yaml"
settings:
  level: 2
  from_b: true
`
	createConfigFile(t, importDir, "b.yaml", configB)

	// Create A (imports B).
	configA := `
base_path: ./
import:
  - "./imports/b.yaml"
settings:
  level: 1
  from_a: true
`
	createConfigFile(t, tempDir, "atmos.yaml", configA)

	v := viper.New()
	v.SetConfigType("yaml")
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)

	// Verify the hierarchy: A overrides B, B overrides C.
	assert.Equal(t, "./", v.GetString("base_path"))
	assert.Equal(t, 1, v.GetInt("settings.level"))
	assert.True(t, v.GetBool("settings.from_a"))
	// B and C's unique settings should not exist (sections are replaced).
	assert.False(t, v.IsSet("settings.from_b"))
	assert.False(t, v.IsSet("settings.from_c"))
}

func TestMergeConfig_WithoutImports(t *testing.T) {
	// Test mergeConfig with processImports=false to ensure that code path is covered.
	tempDir := t.TempDir()

	// Create a simple config file without imports.
	content := `
base_path: ./test
vendor:
  base_path: ./vendor
logs:
  level: Debug
`
	createConfigFile(t, tempDir, "atmos.yaml", content)

	v := viper.New()
	v.SetConfigType("yaml")

	// Call with processImports=false to cover that branch.
	err := mergeConfig(v, tempDir, CliConfigFileName, false)
	assert.NoError(t, err)

	// Verify the config was loaded correctly.
	assert.Equal(t, "./test", v.GetString("base_path"))
	assert.Equal(t, "./vendor", v.GetString("vendor.base_path"))
	assert.Equal(t, "Debug", v.GetString("logs.level"))
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
