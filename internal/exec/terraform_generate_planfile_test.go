package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestExecuteTerraformGeneratePlanfileCmd(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	componentPath := filepath.Join(stacksPath, "..", "..", "components", "terraform", "mock")
	component := "component-1"
	stack := "nonprod"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)

		// Delete the generated files and folders after the test
		err = os.RemoveAll(filepath.Join(componentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.terraform.tfvars.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component))
		assert.NoError(t, err)
	}()

	cmd := &cobra.Command{
		Use:                "terraform generate planfile",
		Short:              "Generate a planfile for a Terraform component",
		Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		Run: func(cmd *cobra.Command, args []string) {
			err := ExecuteTerraformGeneratePlanfileCmd(cmd, args)
			errUtils.CheckErrorPrintAndExit(err, "", "")
		},
	}

	cmd.PersistentFlags().StringP("stack", "s", "", "Atmos stack")
	cmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	cmd.PersistentFlags().StringP("dir", "d", "", "Directory where the planfile will be generated using the default naming convention ({stack}-{component}.planfile.{format})")
	cmd.PersistentFlags().String("format", "json", "Output format (json or yaml)")
	cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function when processing Atmos stack manifests")
	cmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	cmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration files (comma-separated or repeated flag)")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Paths to configuration directories (comma-separated or repeated flag)")

	// Execute the command
	cmd.SetArgs([]string{component, "-s", stack, "--format", "json"})
	err = cmd.Execute()
	assert.NoError(t, err, "'atmos terraform generate planfile' command should execute without error")

	// Check that the planfile was generated
	filePath := fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	t.Run("both file and dir flags return an error", func(t *testing.T) {
		conflictingCmd := &cobra.Command{
			Use:                "terraform generate planfile",
			Short:              "Generate a planfile for a Terraform component",
			Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
			FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
			RunE: func(cmd *cobra.Command, args []string) error {
				return ExecuteTerraformGeneratePlanfileCmd(cmd, args)
			},
		}

		conflictingCmd.PersistentFlags().StringP("stack", "s", "", "Atmos stack")
		conflictingCmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
		conflictingCmd.PersistentFlags().StringP("dir", "d", "", "Directory where the planfile will be generated using the default naming convention ({stack}-{component}.planfile.{format})")
		conflictingCmd.PersistentFlags().String("format", "json", "Output format (json or yaml)")
		conflictingCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
		conflictingCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
		conflictingCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function when processing Atmos stack manifests")
		conflictingCmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
		conflictingCmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
		conflictingCmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
		conflictingCmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration files (comma-separated or repeated flag)")
		conflictingCmd.PersistentFlags().StringSlice("config-path", []string{}, "Paths to configuration directories (comma-separated or repeated flag)")

		conflictingCmd.SetArgs([]string{
			component,
			"-s", stack,
			"--file", "custom.planfile.json",
			"--dir", "custom-dir",
		})

		err := conflictingCmd.Execute()
		assert.ErrorIs(t, err, ErrMutuallyExclusiveFlags)
	})
}

func TestExecuteTerraformGeneratePlanfile(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	componentPath := filepath.Join(stacksPath, "..", "..", "components", "terraform", "mock")
	component := "component-1"
	stack := "nonprod"
	info := schema.ConfigAndStacksInfo{}

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)

		// Delete the generated files and folders after the test
		err = os.RemoveAll(filepath.Join(componentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.terraform.tfvars.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/new-planfile.json", componentPath))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/planfiles/new-planfile.yaml", componentPath))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "custom-planfiles"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "planfiles", "nested"))
		assert.NoError(t, err)
	}()

	options := PlanfileOptions{
		Component:            component,
		Stack:                stack,
		Format:               "json",
		File:                 "",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
	}

	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath := fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	options.Format = "yaml"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	options.Format = "json"
	options.File = "new-planfile.json"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/new-planfile.json", componentPath)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	options.Format = "yaml"
	options.File = "planfiles/new-planfile.yaml"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/planfiles/new-planfile.yaml", componentPath)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	options.File = ""
	options.Format = "json"
	options.Dir = "custom-planfiles"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = filepath.Join(componentPath, "custom-planfiles", fmt.Sprintf("%s-%s.planfile.json", stack, component))
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	absDir := t.TempDir()
	options.Format = "yaml"
	options.Dir = absDir
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = filepath.Join(absDir, fmt.Sprintf("%s-%s.planfile.yaml", stack, component))
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	options.Format = "json"
	options.Dir = "planfiles/nested/deep"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = filepath.Join(componentPath, "planfiles", "nested", "deep", fmt.Sprintf("%s-%s.planfile.json", stack, component))
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}
}

func TestExecuteTerraformGeneratePlanfileErrors(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	component := "component-1"
	stack := "nonprod"
	info := schema.ConfigAndStacksInfo{}

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)
	}()

	options := PlanfileOptions{
		Component:            component,
		Stack:                stack,
		Format:               "",
		File:                 "",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
	}

	options.Format = "invalid-format"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFormat)

	options.Format = "json"
	options.Component = "invalid-component"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)

	options.Component = component
	options.Stack = "invalid-stack"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)

	options.Format = "json"
	options.Stack = stack
	options.Component = ""
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoComponent)

	options.Component = component
	options.File = "custom-file.json"
	options.Dir = "custom-dir"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMutuallyExclusiveFlags)
}

// TestValidatePlanfileFormat tests the validatePlanfileFormat function.
func TestValidatePlanfileFormat(t *testing.T) {
	tests := []struct {
		name           string
		format         string
		expectedFormat string
		expectError    bool
	}{
		{
			name:           "Empty string defaults to json",
			format:         "",
			expectedFormat: "json",
			expectError:    false,
		},
		{
			name:           "Valid json format",
			format:         "json",
			expectedFormat: "json",
			expectError:    false,
		},
		{
			name:           "Valid yaml format",
			format:         "yaml",
			expectedFormat: "yaml",
			expectError:    false,
		},
		{
			name:        "Invalid format xml",
			format:      "xml",
			expectError: true,
		},
		{
			name:        "Invalid format toml",
			format:      "toml",
			expectError: true,
		},
		{
			name:        "Invalid format random",
			format:      "random",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := tt.format
			err := validatePlanfileFormat(&format)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFormat)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedFormat, format)
			}
		})
	}
}

// TestPlanfileValidateComponent tests the validateComponent function in terraform_generate_planfile.go.
func TestPlanfileValidateComponent(t *testing.T) {
	tests := []struct {
		name        string
		component   string
		expectError bool
	}{
		{
			name:        "Valid component name",
			component:   "vpc",
			expectError: false,
		},
		{
			name:        "Valid component with hyphen",
			component:   "my-component",
			expectError: false,
		},
		{
			name:        "Valid component with underscore",
			component:   "my_component",
			expectError: false,
		},
		{
			name:        "Empty component name",
			component:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateComponent(tt.component)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrNoComponent)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResolvePlanfilePath(t *testing.T) {
	tempDir := t.TempDir()
	componentsDir := filepath.Join(tempDir, "components")
	atmosConfig := schema.AtmosConfiguration{
		TerraformDirAbsolutePath: componentsDir,
	}

	info := schema.ConfigAndStacksInfo{
		Component:                     "component-1",
		FinalComponent:                "component-1",
		ComponentFolderPrefix:         "",
		ComponentFolderPrefixReplaced: "",
		ContextPrefix:                 "nonprod",
	}

	componentPath := filepath.Join(componentsDir, info.FinalComponent)
	defaultPlanfileName := constructTerraformComponentPlanfileName(&info)
	defaultJSONPath := fmt.Sprintf("%s.%s", constructTerraformComponentPlanfilePath(&atmosConfig, &info), "json")
	defaultYAMLPath := fmt.Sprintf("%s.%s", constructTerraformComponentPlanfilePath(&atmosConfig, &info), "yaml")
	relativeDir := filepath.Join("plans", "nested")
	absDir := filepath.Join(tempDir, "absolute-plans")

	testCases := []struct {
		name     string
		options  PlanfileOptions
		expected string
	}{
		{
			name: "default json",
			options: PlanfileOptions{
				Format: "json",
			},
			expected: defaultJSONPath,
		},
		{
			name: "default yaml",
			options: PlanfileOptions{
				Format: "yaml",
			},
			expected: defaultYAMLPath,
		},
		{
			name: "custom file relative",
			options: PlanfileOptions{
				Format: "json",
				File:   "custom-planfile.json",
			},
			expected: filepath.Join(componentPath, "custom-planfile.json"),
		},
		{
			name: "dir relative",
			options: PlanfileOptions{
				Format: "json",
				Dir:    relativeDir,
			},
			expected: filepath.Join(componentPath, relativeDir, fmt.Sprintf("%s.%s", defaultPlanfileName, "json")),
		},
		{
			name: "dir absolute",
			options: PlanfileOptions{
				Format: "yaml",
				Dir:    absDir,
			},
			expected: filepath.Join(absDir, fmt.Sprintf("%s.%s", defaultPlanfileName, "yaml")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path, err := resolvePlanfilePath(componentPath, &tc.options, &info, &atmosConfig)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, path)
		})
	}
}
