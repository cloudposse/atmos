package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	log "github.com/cloudposse/atmos/pkg/logger"
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

func TestDescribeComponentWithProvenance(t *testing.T) {
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

	// Define the working directory - using quick-start-advanced as it has a good mix of configs
	workDir := "../../examples/quick-start-advanced"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	component := "vpc-flow-logs-bucket"
	stack := "plat-ue2-dev"

	// Execute describe component WITHOUT provenance first to get a baseline
	componentSection, err := ExecuteDescribeComponent(
		component,
		stack,
		true, // processTemplates
		true, // processYamlFunctions
		nil,  // skip
	)
	assert.NoError(t, err)
	assert.NotNil(t, componentSection)

	// Now execute with provenance by passing nil atmosConfig
	// This will initialize it properly, but we need to set provenance tracking
	// We'll use the DescribeComponentExec with the Provenance flag set
	exec := NewDescribeComponentExec()

	// Execute with provenance enabled
	err = exec.ExecuteDescribeComponentCmd(DescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Provenance:           true,
	})
	assert.NoError(t, err)

	// Get the last merge context that was stored
	mergeContext := GetLastMergeContext()
	assert.NotNil(t, mergeContext, "MergeContext should not be nil when provenance is enabled")
	assert.True(t, mergeContext.IsProvenanceEnabled(), "MergeContext should have provenance enabled")

	// Use the baseline component section for further checks
	result := &DescribeComponentResult{
		ComponentSection: componentSection,
		MergeContext:     mergeContext,
	}
	assert.NotNil(t, result)

	// Verify component section is populated
	assert.NotNil(t, result.ComponentSection)
	assert.NotEmpty(t, result.ComponentSection)

	// Verify MergeContext is populated
	assert.NotNil(t, result.MergeContext, "MergeContext should not be nil when provenance is enabled")
	assert.True(t, result.MergeContext.IsProvenanceEnabled(), "MergeContext should have provenance enabled")

	// Verify provenance data exists
	provenancePaths := result.MergeContext.GetProvenancePaths()
	assert.NotEmpty(t, provenancePaths, "Provenance paths should not be empty")

	// Verify we have provenance entries for vars fields.
	// Check for any vars-related paths rather than specific ones to avoid platform-specific issues.
	foundVarsPath := false
	varsPathsFound := []string{}
	for _, path := range provenancePaths {
		entries := result.MergeContext.GetProvenance(path)
		if len(entries) > 0 {
			// Check for any vars.* path.
			if strings.Contains(path, "vars.") {
				foundVarsPath = true
				varsPathsFound = append(varsPathsFound, path)
				// Verify the entry has file and line information
				assert.NotEmpty(t, entries[0].File, "Provenance entry for %s should have a file", path)
				assert.Greater(t, entries[0].Line, 0, "Provenance entry for %s should have a line number", path)
			}
		}
	}

	// At least one vars path should be found.
	if !foundVarsPath {
		t.Logf("No vars.* paths found in provenance. Available paths: %v", provenancePaths)
	}
	assert.True(t, foundVarsPath, "Should find provenance for at least one vars field. Found vars paths: %v", varsPathsFound)

	// Filter computed fields
	filtered := FilterComputedFields(result.ComponentSection)

	// Verify filtered section only has stack-defined fields
	allowedFields := []string{"vars", "settings", "env", "backend", "metadata", "overrides", "providers", "imports"}
	for k := range filtered {
		assert.Contains(t, allowedFields, k, "Filtered component section should only contain stack-defined fields")
	}

	// Verify computed fields are removed
	computedFields := []string{"atmos_component", "atmos_stack", "component_info", "cli_args", "sources", "deps", "workspace"}
	for _, field := range computedFields {
		assert.NotContains(t, filtered, field, "Filtered component section should not contain computed field: %s", field)
	}

	// Verify expected fields exist
	assert.Contains(t, filtered, "vars", "Should contain vars")
	assert.Contains(t, filtered, "settings", "Should contain settings")

	// Verify vars content
	vars, ok := filtered["vars"].(map[string]any)
	assert.True(t, ok, "vars should be a map")
	assert.NotEmpty(t, vars, "vars should not be empty")
	assert.Contains(t, vars, "enabled", "vars should contain 'enabled'")
	assert.Contains(t, vars, "name", "vars should contain 'name'")

	// Verify we can convert to YAML without errors
	yamlBytes, err := u.ConvertToYAML(filtered)
	assert.NoError(t, err)
	assert.NotEmpty(t, yamlBytes)

	// Verify YAML contains expected content
	yamlStr := yamlBytes
	assert.Contains(t, yamlStr, "vars:", "YAML should contain vars")
	assert.Contains(t, yamlStr, "enabled:", "YAML should contain enabled")

	// Verify YAML structure doesn't have unwanted top-level keys
	// (We already verified this in the filtered map checks above, but double-check in YAML)
	lines := strings.Split(yamlStr, "\n")
	topLevelKeys := make(map[string]bool)
	for _, line := range lines {
		// Check for non-indented lines (top-level keys)
		if len(line) > 0 && !strings.HasPrefix(line, " ") && strings.Contains(line, ":") {
			key := strings.Split(line, ":")[0]
			topLevelKeys[key] = true
		}
	}

	// Verify computed fields are not top-level keys
	assert.False(t, topLevelKeys["component_info"], "component_info should not be a top-level key")
	assert.False(t, topLevelKeys["atmos_cli_config"], "atmos_cli_config should not be a top-level key")
	assert.False(t, topLevelKeys["sources"], "sources should not be a top-level key")
	assert.False(t, topLevelKeys["deps"], "deps should not be a top-level key")
	assert.False(t, topLevelKeys["workspace"], "workspace should not be a top-level key")

	t.Logf("Successfully tested provenance tracking with %d provenance paths", len(provenancePaths))
}

func TestFilterComputedFields(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name: "Filters out all computed fields",
			input: map[string]any{
				"vars":             map[string]any{"key": "value"},
				"settings":         map[string]any{"setting": "value"},
				"atmos_component":  "test-component",
				"atmos_stack":      "test-stack",
				"component_info":   map[string]any{"path": "/some/path"},
				"cli_args":         []string{"arg1"},
				"sources":          []string{"file1.yaml"},
				"deps":             []string{"dep1"},
				"workspace":        "default",
				"atmos_cli_config": map[string]any{"base_path": "."},
				"spacelift_stack":  "stack-name",
				"atlantis_project": "project-name",
				"atmos_stack_file": "stack.yaml",
				"atmos_manifest":   "manifest.yaml",
			},
			expected: map[string]any{
				"vars":     map[string]any{"key": "value"},
				"settings": map[string]any{"setting": "value"},
			},
		},
		{
			name: "Keeps only allowed fields",
			input: map[string]any{
				"vars":      map[string]any{"enabled": true},
				"env":       map[string]any{"VAR": "value"},
				"backend":   map[string]any{"type": "s3"},
				"metadata":  map[string]any{"type": "real"},
				"overrides": map[string]any{"key": "val"},
				"providers": map[string]any{"aws": "config"},
				"settings":  map[string]any{"key": "val"},
			},
			expected: map[string]any{
				"vars":      map[string]any{"enabled": true},
				"env":       map[string]any{"VAR": "value"},
				"backend":   map[string]any{"type": "s3"},
				"metadata":  map[string]any{"type": "real"},
				"overrides": map[string]any{"key": "val"},
				"providers": map[string]any{"aws": "config"},
				"settings":  map[string]any{"key": "val"},
			},
		},
		{
			name:     "Handles empty input",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name:     "Handles nil input",
			input:    nil,
			expected: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterComputedFields(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
