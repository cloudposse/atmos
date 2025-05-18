package exec

import (
	"os"
	"path/filepath"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// --- Tests ---
func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
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
		expectedError bool
	}{
		{
			name: "Test pager with YAML format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "less",
				Format:    "yaml",
			},
		},
		{
			name: "Test pager with JSON format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "less",
				Format:    "json",
			},
		},
		{
			name: "Test no pager with YAML format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "more",
				Format:    "yaml",
			},
		},
		{
			name: "Test no pager with JSON format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "more",
				Format:    "json",
			},
		},
		{
			name: "Test invalid format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "less",
				Format:    "invalid-format",
			},
			expectedError: true,
		},
		{
			name: "Test pager with query",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "less",
				Format:    "json",
				Query:     ".component",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.params.Pager == "less" && !test.expectedError {
				ctrl := gomock.NewController(t)
				pagerMock := pager.NewMockPageCreator(ctrl)
				pagerMock.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockedExec.pageCreator = pagerMock
			} else {
				mockedExec.printOrWriteToFile = func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error {
					assert.Equal(t, test.params.Format, format)
					assert.Equal(t, "", file)
					assert.Equal(t, map[string]any{
						"component": "component-1",
						"stack":     "nonprod",
					}, data)
					return nil
				}
			}
			// stub out internal deps
			err := mockedExec.ExecuteDescribeComponentCmd(test.params)
			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
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
}
