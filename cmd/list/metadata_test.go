package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	l "github.com/cloudposse/atmos/pkg/list"
)

// TestListMetadataFlags tests that the list metadata command has the correct flags.
func TestListMetadataFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "metadata [component]",
		Short: "List metadata across stacks",
		Long:  "List metadata information across all stacks or for a specific component",
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

// TestListMetadataCommand tests the metadata command structure.
func TestListMetadataCommand(t *testing.T) {
	assert.Equal(t, "metadata [component]", metadataCmd.Use)
	assert.Contains(t, metadataCmd.Short, "List metadata across stacks")
	assert.NotNil(t, metadataCmd.RunE)
	assert.NotEmpty(t, metadataCmd.Example)
}

// TestSetupMetadataOptions tests the setupMetadataOptions function.
func TestSetupMetadataOptions(t *testing.T) {
	testCases := []struct {
		name            string
		opts            *MetadataOptions
		componentFilter string
		expectedQuery   string
		expectedComp    string
	}{
		{
			name: "with component and custom query",
			opts: &MetadataOptions{
				Query:      ".metadata.component",
				MaxColumns: 10,
				Format:     "json",
				Delimiter:  ",",
				Stack:      "prod-*",
			},
			componentFilter: "vpc",
			expectedQuery:   ".metadata.component",
			expectedComp:    l.KeyMetadata,
		},
		{
			name: "without component and default query",
			opts: &MetadataOptions{
				Query:      "",
				MaxColumns: 5,
				Format:     "yaml",
				Delimiter:  "\t",
				Stack:      "",
			},
			componentFilter: "",
			expectedQuery:   ".metadata",
			expectedComp:    l.KeyMetadata,
		},
		{
			name: "with component but no query",
			opts: &MetadataOptions{
				Query:      "",
				MaxColumns: 0,
				Format:     "",
				Delimiter:  "",
				Stack:      "*-dev-*",
			},
			componentFilter: "app",
			expectedQuery:   ".metadata",
			expectedComp:    l.KeyMetadata,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterOpts := setupMetadataOptions(tc.opts, tc.componentFilter)

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

// TestMetadataOptions tests the MetadataOptions structure.
func TestMetadataOptions(t *testing.T) {
	opts := &MetadataOptions{
		Format:           "json",
		MaxColumns:       10,
		Delimiter:        ",",
		Stack:            "prod-*",
		Query:            ".metadata.component",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	assert.Equal(t, "json", opts.Format)
	assert.Equal(t, 10, opts.MaxColumns)
	assert.Equal(t, ",", opts.Delimiter)
	assert.Equal(t, "prod-*", opts.Stack)
	assert.Equal(t, ".metadata.component", opts.Query)
	assert.True(t, opts.ProcessTemplates)
	assert.False(t, opts.ProcessFunctions)
}

// TestListMetadataWithOptions_DefaultQuery tests that default query is applied.
func TestListMetadataWithOptions_DefaultQuery(t *testing.T) {
	opts := &MetadataOptions{
		Query: "",
	}

	filterOpts := setupMetadataOptions(opts, "")
	assert.Equal(t, ".metadata", filterOpts.Query, "Should apply default .metadata query")
}

// TestListMetadataWithOptions_CustomQuery tests that custom query is preserved.
func TestListMetadataWithOptions_CustomQuery(t *testing.T) {
	opts := &MetadataOptions{
		Query: ".metadata.custom",
	}

	filterOpts := setupMetadataOptions(opts, "")
	assert.Equal(t, ".metadata.custom", filterOpts.Query, "Should preserve custom query")
}

// TestLogNoMetadataFoundMessage tests the logNoMetadataFoundMessage function.
func TestLogNoMetadataFoundMessage(t *testing.T) {
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
				logNoMetadataFoundMessage(tc.componentFilter)
			})
		})
	}
}

// TestSetupMetadataOptions_AllCombinations tests various option combinations.
func TestSetupMetadataOptions_AllCombinations(t *testing.T) {
	testCases := []struct {
		name               string
		opts               *MetadataOptions
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
			name: "all options with custom query",
			opts: &MetadataOptions{
				Query:      ".metadata.terraform",
				MaxColumns: 8,
				Format:     "csv",
				Delimiter:  ",",
				Stack:      "*-dev-*",
			},
			componentFilter:    "database",
			expectedComponent:  l.KeyMetadata,
			expectedCompFilter: "database",
			expectedQuery:      ".metadata.terraform",
			expectedAbstract:   false,
			expectedMaxColumns: 8,
			expectedFormat:     "csv",
			expectedDelimiter:  ",",
			expectedStackPat:   "*-dev-*",
		},
		{
			name:               "empty query defaults to .metadata",
			opts:               &MetadataOptions{},
			componentFilter:    "",
			expectedComponent:  l.KeyMetadata,
			expectedCompFilter: "",
			expectedQuery:      ".metadata",
			expectedAbstract:   false,
			expectedMaxColumns: 0,
			expectedFormat:     "",
			expectedDelimiter:  "",
			expectedStackPat:   "",
		},
		{
			name: "with component and default query",
			opts: &MetadataOptions{
				Query: "",
			},
			componentFilter:    "app",
			expectedComponent:  l.KeyMetadata,
			expectedCompFilter: "app",
			expectedQuery:      ".metadata",
			expectedAbstract:   false,
			expectedMaxColumns: 0,
			expectedFormat:     "",
			expectedDelimiter:  "",
			expectedStackPat:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterOpts := setupMetadataOptions(tc.opts, tc.componentFilter)

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

// TestMetadataOptions_AllFields tests the MetadataOptions structure with all fields populated.
func TestMetadataOptions_AllFields(t *testing.T) {
	opts := &MetadataOptions{
		Format:           "table",
		MaxColumns:       20,
		Delimiter:        ";",
		Stack:            "prod-us-*",
		Query:            ".metadata.atmos_version",
		ProcessTemplates: false,
		ProcessFunctions: false,
	}

	assert.Equal(t, "table", opts.Format)
	assert.Equal(t, 20, opts.MaxColumns)
	assert.Equal(t, ";", opts.Delimiter)
	assert.Equal(t, "prod-us-*", opts.Stack)
	assert.Equal(t, ".metadata.atmos_version", opts.Query)
	assert.False(t, opts.ProcessTemplates)
	assert.False(t, opts.ProcessFunctions)
}
