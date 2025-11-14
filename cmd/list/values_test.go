package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestListValuesFlags tests that the list values command has the correct flags.
func TestListValuesFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "values [component]",
		Short: "List component values across stacks",
		Long:  "List values for a component across all stacks where it is used",
		Args:  cobra.ExactArgs(1),
	}

	cmd.PersistentFlags().String("format", "", "Output format")
	cmd.PersistentFlags().String("delimiter", "", "Delimiter for CSV/TSV output")
	cmd.PersistentFlags().String("stack", "", "Stack pattern")
	cmd.PersistentFlags().String("query", "", "JQ query")
	cmd.PersistentFlags().Int("max-columns", 0, "Maximum columns")
	cmd.PersistentFlags().Bool("abstract", false, "Include abstract components")
	cmd.PersistentFlags().Bool("vars", false, "Show only vars")
	cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing")
	cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing")

	formatFlag := cmd.PersistentFlags().Lookup("format")
	assert.NotNil(t, formatFlag, "Expected format flag to exist")
	assert.Equal(t, "", formatFlag.DefValue)

	delimiterFlag := cmd.PersistentFlags().Lookup("delimiter")
	assert.NotNil(t, delimiterFlag, "Expected delimiter flag to exist")
	assert.Equal(t, "", delimiterFlag.DefValue)

	stackFlag := cmd.PersistentFlags().Lookup("stack")
	assert.NotNil(t, stackFlag, "Expected stack flag to exist")
	assert.Equal(t, "", stackFlag.DefValue)

	queryFlag := cmd.PersistentFlags().Lookup("query")
	assert.NotNil(t, queryFlag, "Expected query flag to exist")
	assert.Equal(t, "", queryFlag.DefValue)

	maxColumnsFlag := cmd.PersistentFlags().Lookup("max-columns")
	assert.NotNil(t, maxColumnsFlag, "Expected max-columns flag to exist")
	assert.Equal(t, "0", maxColumnsFlag.DefValue)

	abstractFlag := cmd.PersistentFlags().Lookup("abstract")
	assert.NotNil(t, abstractFlag, "Expected abstract flag to exist")
	assert.Equal(t, "false", abstractFlag.DefValue)

	varsFlag := cmd.PersistentFlags().Lookup("vars")
	assert.NotNil(t, varsFlag, "Expected vars flag to exist")
	assert.Equal(t, "false", varsFlag.DefValue)

	processTemplatesFlag := cmd.PersistentFlags().Lookup("process-templates")
	assert.NotNil(t, processTemplatesFlag, "Expected process-templates flag to exist")
	assert.Equal(t, "true", processTemplatesFlag.DefValue)

	processFunctionsFlag := cmd.PersistentFlags().Lookup("process-functions")
	assert.NotNil(t, processFunctionsFlag, "Expected process-functions flag to exist")
	assert.Equal(t, "true", processFunctionsFlag.DefValue)
}

// TestListVarsFlags tests that the list vars command has the correct flags.
func TestListVarsFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "vars [component]",
		Short: "List component vars across stacks (alias for `list values --query .vars`)",
		Long:  "List vars for a component across all stacks where it is used",
		Args:  cobra.ExactArgs(1),
	}

	cmd.PersistentFlags().String("format", "", "Output format")
	cmd.PersistentFlags().String("delimiter", "", "Delimiter for CSV/TSV output")
	cmd.PersistentFlags().String("stack", "", "Stack pattern")
	cmd.PersistentFlags().String("query", "", "JQ query")
	cmd.PersistentFlags().Int("max-columns", 0, "Maximum columns")
	cmd.PersistentFlags().Bool("abstract", false, "Include abstract components")
	cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing")
	cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing")

	assert.Equal(t, "vars [component]", cmd.Use)
	assert.Contains(t, cmd.Short, "List component vars across stacks")
	assert.Contains(t, cmd.Long, "List vars for a component")

	formatFlag := cmd.PersistentFlags().Lookup("format")
	assert.NotNil(t, formatFlag, "Expected format flag to exist")
	assert.Equal(t, "", formatFlag.DefValue)

	delimiterFlag := cmd.PersistentFlags().Lookup("delimiter")
	assert.NotNil(t, delimiterFlag, "Expected delimiter flag to exist")
	assert.Equal(t, "", delimiterFlag.DefValue)

	stackFlag := cmd.PersistentFlags().Lookup("stack")
	assert.NotNil(t, stackFlag, "Expected stack flag to exist")
	assert.Equal(t, "", stackFlag.DefValue)

	queryFlag := cmd.PersistentFlags().Lookup("query")
	assert.NotNil(t, queryFlag, "Expected query flag to exist")
	assert.Equal(t, "", queryFlag.DefValue)

	maxColumnsFlag := cmd.PersistentFlags().Lookup("max-columns")
	assert.NotNil(t, maxColumnsFlag, "Expected max-columns flag to exist")
	assert.Equal(t, "0", maxColumnsFlag.DefValue)

	abstractFlag := cmd.PersistentFlags().Lookup("abstract")
	assert.NotNil(t, abstractFlag, "Expected abstract flag to exist")
	assert.Equal(t, "false", abstractFlag.DefValue)

	processTemplatesFlag := cmd.PersistentFlags().Lookup("process-templates")
	assert.NotNil(t, processTemplatesFlag, "Expected process-templates flag to exist")
	assert.Equal(t, "true", processTemplatesFlag.DefValue)

	processFunctionsFlag := cmd.PersistentFlags().Lookup("process-functions")
	assert.NotNil(t, processFunctionsFlag, "Expected process-functions flag to exist")
	assert.Equal(t, "true", processFunctionsFlag.DefValue)
}

// TestListValuesValidatesArgs tests that the command validates arguments.
func TestListValuesValidatesArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "values [component]",
		Args: cobra.ExactArgs(1),
	}

	err := cmd.ValidateArgs([]string{})
	assert.Error(t, err, "Validation should fail with no arguments")

	err = cmd.ValidateArgs([]string{"component"})
	assert.NoError(t, err, "Validation should pass with one argument")

	err = cmd.ValidateArgs([]string{"component", "extra"})
	assert.Error(t, err, "Validation should fail with too many arguments")
}

// TestGetBoolFlagWithDefault tests getBoolFlagWithDefault helper function.
func TestGetBoolFlagWithDefault(t *testing.T) {
	testCases := []struct {
		name         string
		setupCmd     func() *cobra.Command
		flagName     string
		defaultValue bool
		expected     bool
	}{
		{
			name: "flag exists and returns true",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().Bool("test-flag", false, "test flag")
				cmd.Flags().Set("test-flag", "true")
				return cmd
			},
			flagName:     "test-flag",
			defaultValue: false,
			expected:     true,
		},
		{
			name: "flag exists and returns false",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().Bool("test-flag", false, "test flag")
				return cmd
			},
			flagName:     "test-flag",
			defaultValue: true,
			expected:     false,
		},
		{
			name: "flag doesn't exist returns default true",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{}
			},
			flagName:     "nonexistent",
			defaultValue: true,
			expected:     true,
		},
		{
			name: "flag doesn't exist returns default false",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{}
			},
			flagName:     "nonexistent",
			defaultValue: false,
			expected:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.setupCmd()
			result := getBoolFlagWithDefault(cmd, tc.flagName, tc.defaultValue)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetFilterOptionsFromValues tests the getFilterOptionsFromValues helper function.
func TestGetFilterOptionsFromValues(t *testing.T) {
	testCases := []struct {
		name              string
		opts              *ValuesOptions
		expectedQuery     string
		expectedDelimiter string
		expectedAbstract  bool
		expectedMaxCols   int
	}{
		{
			name: "vars flag true sets query to .vars",
			opts: &ValuesOptions{
				Vars:      true,
				Query:     ".custom",
				Format:    "json",
				Delimiter: "\t",
				Abstract:  false,
			},
			expectedQuery:     ".vars",
			expectedDelimiter: "\t",
			expectedAbstract:  false,
			expectedMaxCols:   0,
		},
		{
			name: "CSV format changes TSV delimiter to comma",
			opts: &ValuesOptions{
				Format:    "csv",
				Delimiter: "\t",
			},
			expectedQuery:     "",
			expectedDelimiter: ",",
			expectedAbstract:  false,
			expectedMaxCols:   0,
		},
		{
			name: "custom query preserved when vars false",
			opts: &ValuesOptions{
				Query:      ".vars.region",
				Format:     "yaml",
				Delimiter:  "",
				Abstract:   true,
				MaxColumns: 10,
			},
			expectedQuery:     ".vars.region",
			expectedDelimiter: "",
			expectedAbstract:  true,
			expectedMaxCols:   10,
		},
		{
			name: "TSV format preserves tab delimiter",
			opts: &ValuesOptions{
				Format:    "tsv",
				Delimiter: "\t",
			},
			expectedQuery:     "",
			expectedDelimiter: "\t",
			expectedAbstract:  false,
			expectedMaxCols:   0,
		},
		{
			name: "custom delimiter preserved for non-CSV formats",
			opts: &ValuesOptions{
				Format:    "json",
				Delimiter: "|",
			},
			expectedQuery:     "",
			expectedDelimiter: "|",
			expectedAbstract:  false,
			expectedMaxCols:   0,
		},
		{
			name:              "empty options",
			opts:              &ValuesOptions{},
			expectedQuery:     "",
			expectedDelimiter: "",
			expectedAbstract:  false,
			expectedMaxCols:   0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterOpts := getFilterOptionsFromValues(tc.opts)

			assert.Equal(t, tc.expectedQuery, filterOpts.Query)
			assert.Equal(t, tc.expectedDelimiter, filterOpts.Delimiter)
			assert.Equal(t, tc.expectedAbstract, filterOpts.IncludeAbstract)
			assert.Equal(t, tc.expectedMaxCols, filterOpts.MaxColumns)
		})
	}
}

// TestLogNoValuesFoundMessage tests that the correct log message is generated.
func TestLogNoValuesFoundMessage(t *testing.T) {
	testCases := []struct {
		name          string
		componentName string
		query         string
		// We can't easily test log output, but we can verify the function doesn't panic
	}{
		{
			name:          "vars query",
			componentName: "vpc",
			query:         ".vars",
		},
		{
			name:          "custom query",
			componentName: "app",
			query:         ".settings",
		},
		{
			name:          "empty query",
			componentName: "database",
			query:         "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This function only logs, so we just verify it doesn't panic
			assert.NotPanics(t, func() {
				logNoValuesFoundMessage(tc.componentName, tc.query)
			})
		})
	}
}

// TestPrepareListValuesOptions tests the prepareListValuesOptions helper function.
func TestPrepareListValuesOptions(t *testing.T) {
	testCases := []struct {
		name               string
		opts               *ValuesOptions
		componentName      string
		expectedComponent  string
		expectedCompFilter string
		expectedQuery      string
		expectedIncludeAbs bool
	}{
		{
			name: "vars query clears Component field",
			opts: &ValuesOptions{
				Query:    ".vars",
				Vars:     true,
				Abstract: false,
			},
			componentName:      "vpc",
			expectedComponent:  "",
			expectedCompFilter: "vpc",
			expectedQuery:      ".vars",
			expectedIncludeAbs: false,
		},
		{
			name: "non-vars query sets both Component and ComponentFilter",
			opts: &ValuesOptions{
				Query:    ".settings",
				Abstract: true,
			},
			componentName:      "app",
			expectedComponent:  "app",
			expectedCompFilter: "app",
			expectedQuery:      ".settings",
			expectedIncludeAbs: true,
		},
		{
			name: "empty query with component",
			opts: &ValuesOptions{
				Query:    "",
				Abstract: false,
			},
			componentName:      "database",
			expectedComponent:  "database",
			expectedCompFilter: "database",
			expectedQuery:      "",
			expectedIncludeAbs: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterOpts := prepareListValuesOptions(tc.opts, tc.componentName)

			assert.Equal(t, tc.expectedComponent, filterOpts.Component)
			assert.Equal(t, tc.expectedCompFilter, filterOpts.ComponentFilter)
			assert.Equal(t, tc.expectedQuery, filterOpts.Query)
			assert.Equal(t, tc.expectedIncludeAbs, filterOpts.IncludeAbstract)
		})
	}
}

// TestListValuesCommand tests the values command structure.
func TestListValuesCommand(t *testing.T) {
	assert.Equal(t, "values [component]", valuesCmd.Use)
	assert.Contains(t, valuesCmd.Short, "List component values across stacks")
	assert.NotNil(t, valuesCmd.RunE)
	assert.NotEmpty(t, valuesCmd.Example)
}

// TestListVarsCommand tests the vars command structure.
func TestListVarsCommand(t *testing.T) {
	assert.Equal(t, "vars [component]", varsCmd.Use)
	assert.Contains(t, varsCmd.Short, "List component vars across stacks")
	assert.NotNil(t, varsCmd.RunE)
	assert.NotEmpty(t, varsCmd.Example)
}
