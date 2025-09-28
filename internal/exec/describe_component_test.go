package exec

import (
	"os"
	"path/filepath"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	// Set up gomock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockedExec := &DescribeComponentExec{
		printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error {
			return nil
		},
		IsTTYSupportForStdout: func() bool {
			return true
		},
		executeDescribeComponent: func(component, stack string, processTemplates, processYamlFunctions bool, skip []string) (map[string]any, error) {
			return map[string]any{
				"component": component,
				"stack":     stack,
			}, nil
		},
		initCliConfig: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		evaluateYqExpression: func(atmosConfig *schema.AtmosConfiguration, data any, yq string) (any, error) {
			return data, nil
		},
	}

	tests := []struct {
		name          string
		params        DescribeComponentParams
		pagerSetting  string
		expectPager   bool
		expectedError bool
	}{
		{
			name: "Test pager with YAML format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Format:    "yaml",
			},
			pagerSetting: "less",
			expectPager:  true,
		},
		{
			name: "Test pager with JSON format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Format:    "json",
			},
			pagerSetting: "more",
			expectPager:  true,
		},
		{
			name: "Test no pager with YAML format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Format:    "yaml",
			},
			pagerSetting: "false",
			expectPager:  false,
		},
		{
			name: "Test no pager with JSON format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Format:    "json",
			},
			pagerSetting: "off",
			expectPager:  false,
		},
		{
			name: "Test invalid format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Format:    "invalid-format",
			},
			pagerSetting:  "less",
			expectPager:   true,
			expectedError: true,
		},
		{
			name: "Test pager with query",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Format:    "json",
				Query:     ".component",
			},
			pagerSetting: "less",
			expectPager:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			printOrWriteToFileCalled := false

			// Set up mock pager based on test expectation
			if test.expectPager {
				mockPager := pager.NewMockPageCreator(ctrl)
				if test.expectedError && test.params.Format == "invalid-format" {
					// The pager won't be called because viewConfig will return error before reaching pageCreator
				} else {
					mockPager.EXPECT().Run("component-1", gomock.Any()).Return(nil).Times(1)
				}
				mockedExec.pageCreator = mockPager
			} else {
				// For non-pager tests, we don't need to mock the pager as it won't be called
				mockedExec.pageCreator = nil
			}

			// Mock the initCliConfig to return a config with the test's pager setting
			mockedExec.initCliConfig = func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Pager: test.pagerSetting,
						},
					},
				}, nil
			}

			// Mock printOrWriteToFile - this should only be called when pager is disabled or fails
			mockedExec.printOrWriteToFile = func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error {
				printOrWriteToFileCalled = true
				if test.expectedError && test.params.Format == "invalid-format" {
					return DescribeConfigFormatError{format: "invalid-format"}
				}
				assert.Equal(t, test.params.Format, format)
				assert.Equal(t, "", file)
				assert.Equal(t, map[string]any{
					"component": "component-1",
					"stack":     "nonprod",
				}, data)
				return nil
			}

			// Execute the command
			err := mockedExec.ExecuteDescribeComponentCmd(test.params)

			// Assert expectations
			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// When pager is enabled and successful, printOrWriteToFile should NOT be called (pager path returns early)
			// When pager is disabled, printOrWriteToFile SHOULD be called
			// When pager is enabled but fails with DescribeConfigFormatError, printOrWriteToFile should NOT be called (error path returns early)
			expectedPrintCall := !test.expectPager && !test.expectedError
			assert.Equal(t, expectedPrintCall, printOrWriteToFileCalled,
				"printOrWriteToFile call expectation mismatch for pager setting: %s", test.pagerSetting)
		})
	}
}

func TestDescribeComponentWithOverridesSection(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)

		// Change back to the original working directory after the test
		if err = os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/atmos-overrides-section"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	component := "c1"

	// `dev`
	res, err := ExecuteDescribeComponent(
		component,
		"dev",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err := u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "a: a-dev")
	assert.Contains(t, y, "b: b-team2")
	assert.Contains(t, y, "c: c-team1")
	assert.Contains(t, y, "d: d")

	// `staging`
	res, err = ExecuteDescribeComponent(
		component,
		"staging",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "a: a-staging")
	assert.Contains(t, y, "b: b-team2")
	assert.Contains(t, y, "c: c-team1")
	assert.Contains(t, y, "d: d")

	// `prod`
	res, err = ExecuteDescribeComponent(
		component,
		"prod",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "a: a-prod")
	assert.Contains(t, y, "b: b-prod")
	assert.Contains(t, y, "c: c-prod")
	assert.Contains(t, y, "d: d")

	// `sandbox`
	res, err = ExecuteDescribeComponent(
		component,
		"sandbox",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "a: a-team2")
	assert.Contains(t, y, "b: b-team2")
	assert.Contains(t, y, "c: c-team1")
	assert.Contains(t, y, "d: d")

	// `test`
	res, err = ExecuteDescribeComponent(
		component,
		"test",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "a: a-test-2")
	assert.Contains(t, y, "b: b-test")
	assert.Contains(t, y, "c: c-team1")
	assert.Contains(t, y, "d: d")

	// `test2`
	res, err = ExecuteDescribeComponent(
		component,
		"test2",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "a: a")
	assert.Contains(t, y, "b: b")
	assert.Contains(t, y, "c: c")
	assert.Contains(t, y, "d: d")

	// `test3`
	res, err = ExecuteDescribeComponent(
		component,
		"test3",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "a: a-overridden")
	assert.Contains(t, y, "b: b-overridden")
	assert.Contains(t, y, "c: c")
	assert.Contains(t, y, "d: d")
}

func TestDescribeComponent_Packer(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err = os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/packer"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	component := "aws/bastion"

	res, err := ExecuteDescribeComponent(
		component,
		"prod",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	val, err := u.EvaluateYqExpression(&atmosConfig, res, ".vars.ami_tags.SourceAMI")
	assert.Nil(t, err)
	assert.Equal(t, "ami-0013ceeff668b979b", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, res, ".stack")
	assert.Nil(t, err)
	assert.Equal(t, "prod", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, res, ".vars.assume_role_arn")
	assert.Nil(t, err)
	assert.Equal(t, "arn:aws:iam::PROD_ACCOUNT_ID:role/ROLE_NAME", val)
}
