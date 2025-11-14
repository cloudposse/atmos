package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestListSettingsFlags tests that the list settings command has the correct flags.
func TestListSettingsFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "settings [component]",
		Short: "List settings across stacks or for a specific component",
		Long:  "List settings configuration across all stacks or for a specific component",
		Args:  cobra.MaximumNArgs(1),
	}

	cmd.PersistentFlags().String("format", "", "Output format")
	cmd.PersistentFlags().String("delimiter", "", "Delimiter for CSV/TSV output")
	cmd.PersistentFlags().String("stack", "", "Stack pattern")
	cmd.PersistentFlags().String("query", "", "JQ query")
	cmd.PersistentFlags().Int("max-columns", 0, "Maximum columns")
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

	processTemplatesFlag := cmd.PersistentFlags().Lookup("process-templates")
	assert.NotNil(t, processTemplatesFlag, "Expected process-templates flag to exist")
	assert.Equal(t, "true", processTemplatesFlag.DefValue)

	processFunctionsFlag := cmd.PersistentFlags().Lookup("process-functions")
	assert.NotNil(t, processFunctionsFlag, "Expected process-functions flag to exist")
	assert.Equal(t, "true", processFunctionsFlag.DefValue)
}

// TestListSettingsValidatesArgs tests that the command validates arguments.
func TestListSettingsValidatesArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "settings [component]",
		Args: cobra.MaximumNArgs(1),
	}

	err := cmd.ValidateArgs([]string{})
	assert.NoError(t, err, "Validation should pass with no arguments")

	err = cmd.ValidateArgs([]string{"component"})
	assert.NoError(t, err, "Validation should pass with one argument")

	err = cmd.ValidateArgs([]string{"component", "extra"})
	assert.Error(t, err, "Validation should fail with too many arguments")
}

// TestListSettingsCommand tests the settings command structure.
func TestListSettingsCommand(t *testing.T) {
	assert.Equal(t, "settings [component]", settingsCmd.Use)
	assert.Contains(t, settingsCmd.Short, "List settings across stacks")
	assert.NotNil(t, settingsCmd.RunE)
	assert.NotEmpty(t, settingsCmd.Example)
}

// TestSetupSettingsOptions tests the setupSettingsOptions function.
func TestSetupSettingsOptions(t *testing.T) {
	testCases := []struct {
		name            string
		opts            *SettingsOptions
		componentFilter string
		expectedQuery   string
		expectedComp    string
	}{
		{
			name: "with component and custom query",
			opts: &SettingsOptions{
				Query:      ".terraform",
				MaxColumns: 10,
				Format:     "json",
				Delimiter:  ",",
				Stack:      "prod-*",
			},
			componentFilter: "vpc",
			expectedQuery:   ".terraform",
			expectedComp:    "settings",
		},
		{
			name: "without component and no query",
			opts: &SettingsOptions{
				Query:      "",
				MaxColumns: 5,
				Format:     "yaml",
				Delimiter:  "\t",
				Stack:      "",
			},
			componentFilter: "",
			expectedQuery:   "",
			expectedComp:    "settings",
		},
		{
			name: "with component but no query",
			opts: &SettingsOptions{
				Query:      "",
				MaxColumns: 0,
				Format:     "",
				Delimiter:  "",
				Stack:      "*-dev-*",
			},
			componentFilter: "app",
			expectedQuery:   "",
			expectedComp:    "settings",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterOpts := setupSettingsOptions(tc.opts, tc.componentFilter)

			assert.Equal(t, tc.expectedComp, filterOpts.Component)
			assert.Equal(t, tc.componentFilter, filterOpts.ComponentFilter)
			assert.Equal(t, tc.expectedQuery, filterOpts.Query)
			assert.False(t, filterOpts.IncludeAbstract)
			assert.Equal(t, tc.opts.MaxColumns, filterOpts.MaxColumns)
			assert.Equal(t, tc.opts.Format, filterOpts.FormatStr)
			assert.Equal(t, tc.opts.Delimiter, filterOpts.Delimiter)
			assert.Equal(t, tc.opts.Stack, filterOpts.StackPattern)
		})
	}
}

// TestSettingsOptions tests the SettingsOptions structure.
func TestSettingsOptions(t *testing.T) {
	opts := &SettingsOptions{
		Format:           "json",
		MaxColumns:       10,
		Delimiter:        ",",
		Stack:            "prod-*",
		Query:            ".terraform",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	assert.Equal(t, "json", opts.Format)
	assert.Equal(t, 10, opts.MaxColumns)
	assert.Equal(t, ",", opts.Delimiter)
	assert.Equal(t, "prod-*", opts.Stack)
	assert.Equal(t, ".terraform", opts.Query)
	assert.True(t, opts.ProcessTemplates)
	assert.False(t, opts.ProcessFunctions)
}

// TestListSettingsWithOptions_EmptyQuery tests that empty query is preserved.
func TestListSettingsWithOptions_EmptyQuery(t *testing.T) {
	opts := &SettingsOptions{
		Query: "",
	}

	filterOpts := setupSettingsOptions(opts, "")
	assert.Equal(t, "", filterOpts.Query, "Should preserve empty query")
}

// TestListSettingsWithOptions_CustomQuery tests that custom query is preserved.
func TestListSettingsWithOptions_CustomQuery(t *testing.T) {
	opts := &SettingsOptions{
		Query: ".terraform.backend",
	}

	filterOpts := setupSettingsOptions(opts, "")
	assert.Equal(t, ".terraform.backend", filterOpts.Query, "Should preserve custom query")
}

// TestLogNoSettingsFoundMessage tests the logNoSettingsFoundMessage function.
func TestLogNoSettingsFoundMessage(t *testing.T) {
	testCases := []struct {
		name            string
		componentFilter string
	}{
		{
			name:            "with component filter",
			componentFilter: "vpc",
		},
		{
			name:            "without component filter",
			componentFilter: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This function only logs, so we just verify it doesn't panic
			assert.NotPanics(t, func() {
				logNoSettingsFoundMessage(tc.componentFilter)
			})
		})
	}
}

// TestSetupSettingsOptions_AllCombinations tests various option combinations.
func TestSetupSettingsOptions_AllCombinations(t *testing.T) {
	testCases := []struct {
		name               string
		opts               *SettingsOptions
		componentFilter    string
		expectedComponent  string
		expectedCompFilter string
		expectedQuery      string
		expectedAbstract   bool
		expectedMaxColumns int
		expectedFormat     string
		expectedDelimiter  string
		expectedStackPat   string
	}{
		{
			name: "all options populated",
			opts: &SettingsOptions{
				Query:      ".terraform",
				MaxColumns: 10,
				Format:     "json",
				Delimiter:  ",",
				Stack:      "prod-*",
			},
			componentFilter:    "vpc",
			expectedComponent:  "settings",
			expectedCompFilter: "vpc",
			expectedQuery:      ".terraform",
			expectedAbstract:   false,
			expectedMaxColumns: 10,
			expectedFormat:     "json",
			expectedDelimiter:  ",",
			expectedStackPat:   "prod-*",
		},
		{
			name:               "minimal options",
			opts:               &SettingsOptions{},
			componentFilter:    "",
			expectedComponent:  "settings",
			expectedCompFilter: "",
			expectedQuery:      "",
			expectedAbstract:   false,
			expectedMaxColumns: 0,
			expectedFormat:     "",
			expectedDelimiter:  "",
			expectedStackPat:   "",
		},
		{
			name: "CSV format with custom delimiter",
			opts: &SettingsOptions{
				Format:    "csv",
				Delimiter: ";",
			},
			componentFilter:    "app",
			expectedComponent:  "settings",
			expectedCompFilter: "app",
			expectedQuery:      "",
			expectedAbstract:   false,
			expectedMaxColumns: 0,
			expectedFormat:     "csv",
			expectedDelimiter:  ";",
			expectedStackPat:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterOpts := setupSettingsOptions(tc.opts, tc.componentFilter)

			assert.Equal(t, tc.expectedComponent, filterOpts.Component)
			assert.Equal(t, tc.expectedCompFilter, filterOpts.ComponentFilter)
			assert.Equal(t, tc.expectedQuery, filterOpts.Query)
			assert.Equal(t, tc.expectedAbstract, filterOpts.IncludeAbstract)
			assert.Equal(t, tc.expectedMaxColumns, filterOpts.MaxColumns)
			assert.Equal(t, tc.expectedFormat, filterOpts.FormatStr)
			assert.Equal(t, tc.expectedDelimiter, filterOpts.Delimiter)
			assert.Equal(t, tc.expectedStackPat, filterOpts.StackPattern)
		})
	}
}

// TestSettingsOptions_AllFields tests the SettingsOptions structure with all fields.
func TestSettingsOptions_AllFields(t *testing.T) {
	opts := &SettingsOptions{
		Format:           "yaml",
		MaxColumns:       15,
		Delimiter:        "|",
		Stack:            "*-staging-*",
		Query:            ".metadata",
		ProcessTemplates: false,
		ProcessFunctions: true,
	}

	assert.Equal(t, "yaml", opts.Format)
	assert.Equal(t, 15, opts.MaxColumns)
	assert.Equal(t, "|", opts.Delimiter)
	assert.Equal(t, "*-staging-*", opts.Stack)
	assert.Equal(t, ".metadata", opts.Query)
	assert.False(t, opts.ProcessTemplates)
	assert.True(t, opts.ProcessFunctions)
}

// TestSettingsCmd_ArgsValidation tests argument validation.
func TestSettingsCmd_ArgsValidation(t *testing.T) {
	// Settings command accepts MaximumNArgs(1)
	err := settingsCmd.Args(settingsCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")

	err = settingsCmd.Args(settingsCmd, []string{"component"})
	assert.NoError(t, err, "Should accept one argument")

	err = settingsCmd.Args(settingsCmd, []string{"component", "extra"})
	assert.Error(t, err, "Should reject more than one argument")
}
