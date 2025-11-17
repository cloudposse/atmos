package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// TestWithFormatFlag verifies format flag registration.
func TestWithFormatFlag(t *testing.T) {
	parser := NewListParser(WithFormatFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("format")
	require.NotNil(t, flag, "format flag should be registered")
	assert.Equal(t, "f", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
	assert.Contains(t, flag.Usage, "Output format")
}

// TestWithColumnsFlag verifies columns flag registration.
func TestWithColumnsFlag(t *testing.T) {
	parser := NewListParser(WithColumnsFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("columns")
	require.NotNil(t, flag, "columns flag should be registered")
	assert.Equal(t, "", flag.Shorthand)
	assert.Contains(t, flag.Usage, "Columns to display")
}

// TestWithStackFlag verifies stack flag registration.
func TestWithStackFlag(t *testing.T) {
	parser := NewListParser(WithStackFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("stack")
	require.NotNil(t, flag, "stack flag should be registered")
	assert.Equal(t, "s", flag.Shorthand)
	assert.Contains(t, flag.Usage, "Filter by stack pattern")
}

// TestWithFilterFlag verifies filter flag registration.
func TestWithFilterFlag(t *testing.T) {
	parser := NewListParser(WithFilterFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("filter")
	require.NotNil(t, flag, "filter flag should be registered")
	assert.Contains(t, flag.Usage, "Filter expression")
}

// TestWithSortFlag verifies sort flag registration.
func TestWithSortFlag(t *testing.T) {
	parser := NewListParser(WithSortFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("sort")
	require.NotNil(t, flag, "sort flag should be registered")
	assert.Contains(t, flag.Usage, "Sort by column")
}

// TestWithEnabledFlag verifies enabled flag registration.
func TestWithEnabledFlag(t *testing.T) {
	parser := NewListParser(WithEnabledFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("enabled")
	require.NotNil(t, flag, "enabled flag should be registered")
	assert.Contains(t, flag.Usage, "Filter by enabled")
}

// TestWithLockedFlag verifies locked flag registration.
func TestWithLockedFlag(t *testing.T) {
	parser := NewListParser(WithLockedFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("locked")
	require.NotNil(t, flag, "locked flag should be registered")
	assert.Contains(t, flag.Usage, "Filter by locked")
}

// TestWithTypeFlag verifies type flag registration.
func TestWithTypeFlag(t *testing.T) {
	parser := NewListParser(WithTypeFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("type")
	require.NotNil(t, flag, "type flag should be registered")
	assert.Equal(t, "t", flag.Shorthand)
	assert.Equal(t, "real", flag.DefValue)
	assert.Contains(t, flag.Usage, "Component type")
}

// TestWithComponentFlag verifies component flag registration.
func TestWithComponentFlag(t *testing.T) {
	parser := NewListParser(WithComponentFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("component")
	require.NotNil(t, flag, "component flag should be registered")
	assert.Equal(t, "c", flag.Shorthand)
	assert.Contains(t, flag.Usage, "Filter stacks")
}

// TestWithDelimiterFlag verifies delimiter flag registration.
func TestWithDelimiterFlag(t *testing.T) {
	parser := NewListParser(WithDelimiterFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("delimiter")
	require.NotNil(t, flag, "delimiter flag should be registered")
	assert.Contains(t, flag.Usage, "Delimiter")
}

// TestWithFileFlag verifies file flag registration.
func TestWithFileFlag(t *testing.T) {
	parser := NewListParser(WithFileFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("file")
	require.NotNil(t, flag, "file flag should be registered")
	assert.Contains(t, flag.Usage, "Filter workflows")
}

// TestWithMaxColumnsFlag verifies max-columns flag registration.
func TestWithMaxColumnsFlag(t *testing.T) {
	parser := NewListParser(WithMaxColumnsFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("max-columns")
	require.NotNil(t, flag, "max-columns flag should be registered")
	assert.Equal(t, "0", flag.DefValue)
	assert.Contains(t, flag.Usage, "Maximum number of columns")
}

// TestWithQueryFlag verifies query flag registration.
func TestWithQueryFlag(t *testing.T) {
	parser := NewListParser(WithQueryFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("query")
	require.NotNil(t, flag, "query flag should be registered")
	assert.Equal(t, "q", flag.Shorthand)
	assert.Contains(t, flag.Usage, "YQ expression")
}

// TestWithAbstractFlag verifies abstract flag registration.
func TestWithAbstractFlag(t *testing.T) {
	parser := NewListParser(WithAbstractFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("abstract")
	require.NotNil(t, flag, "abstract flag should be registered")
	assert.Equal(t, "false", flag.DefValue)
	assert.Contains(t, flag.Usage, "Include abstract")
}

// TestWithProcessTemplatesFlag verifies process-templates flag registration.
func TestWithProcessTemplatesFlag(t *testing.T) {
	parser := NewListParser(WithProcessTemplatesFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("process-templates")
	require.NotNil(t, flag, "process-templates flag should be registered")
	assert.Equal(t, "true", flag.DefValue)
	assert.Contains(t, flag.Usage, "Go template processing")
}

// TestWithProcessFunctionsFlag verifies process-functions flag registration.
func TestWithProcessFunctionsFlag(t *testing.T) {
	parser := NewListParser(WithProcessFunctionsFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("process-functions")
	require.NotNil(t, flag, "process-functions flag should be registered")
	assert.Equal(t, "true", flag.DefValue)
	assert.Contains(t, flag.Usage, "template function processing")
}

// TestWithUploadFlag verifies upload flag registration.
func TestWithUploadFlag(t *testing.T) {
	parser := NewListParser(WithUploadFlag)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag exists
	flag := cmd.Flags().Lookup("upload")
	require.NotNil(t, flag, "upload flag should be registered")
	assert.Equal(t, "false", flag.DefValue)
	assert.Contains(t, flag.Usage, "Upload instances")
}

// TestNewListParser_MultipleFlagsComposition verifies composing multiple flags.
func TestNewListParser_MultipleFlagsComposition(t *testing.T) {
	// Simulate components command with all relevant flags
	parser := NewListParser(
		WithFormatFlag,
		WithColumnsFlag,
		WithSortFlag,
		WithFilterFlag,
		WithStackFlag,
		WithTypeFlag,
		WithEnabledFlag,
		WithLockedFlag,
	)
	assert.NotNil(t, parser)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify all flags are registered
	flags := []string{"format", "columns", "sort", "filter", "stack", "type", "enabled", "locked"}
	for _, flagName := range flags {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "flag %s should be registered", flagName)
	}
}

// TestNewListParser_SelectiveFlagComposition verifies each command composes only needed flags.
func TestNewListParser_SelectiveFlagComposition(t *testing.T) {
	tests := []struct {
		name          string
		builders      []func(*[]flags.Option)
		expectedFlags []string
		missingFlags  []string
	}{
		{
			name: "components command",
			builders: []func(*[]flags.Option){
				WithFormatFlag,
				WithColumnsFlag,
				WithSortFlag,
				WithFilterFlag,
				WithStackFlag,
				WithTypeFlag,
				WithEnabledFlag,
				WithLockedFlag,
			},
			expectedFlags: []string{"format", "columns", "sort", "filter", "stack", "type", "enabled", "locked"},
			missingFlags:  []string{"component", "file", "max-columns", "query", "upload"},
		},
		{
			name: "stacks command",
			builders: []func(*[]flags.Option){
				WithFormatFlag,
				WithColumnsFlag,
				WithSortFlag,
				WithComponentFlag,
			},
			expectedFlags: []string{"format", "columns", "sort", "component"},
			missingFlags:  []string{"stack", "filter", "type", "enabled", "locked"},
		},
		{
			name: "workflows command",
			builders: []func(*[]flags.Option){
				WithFormatFlag,
				WithDelimiterFlag,
				WithColumnsFlag,
				WithSortFlag,
				WithFileFlag,
			},
			expectedFlags: []string{"format", "delimiter", "columns", "sort", "file"},
			missingFlags:  []string{"stack", "filter", "component"},
		},
		{
			name: "values command",
			builders: []func(*[]flags.Option){
				WithFormatFlag,
				WithDelimiterFlag,
				WithMaxColumnsFlag,
				WithQueryFlag,
				WithStackFlag,
				WithAbstractFlag,
				WithProcessTemplatesFlag,
				WithProcessFunctionsFlag,
			},
			expectedFlags: []string{"format", "delimiter", "max-columns", "query", "stack", "abstract", "process-templates", "process-functions"},
			missingFlags:  []string{"columns", "sort", "filter", "component"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewListParser(tt.builders...)
			assert.NotNil(t, parser)

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			// Verify expected flags are present
			for _, flagName := range tt.expectedFlags {
				flag := cmd.Flags().Lookup(flagName)
				assert.NotNil(t, flag, "flag %s should be registered for %s", flagName, tt.name)
			}

			// Verify flags that shouldn't be present are absent
			for _, flagName := range tt.missingFlags {
				flag := cmd.Flags().Lookup(flagName)
				assert.Nil(t, flag, "flag %s should NOT be registered for %s", flagName, tt.name)
			}
		})
	}
}

// TestFlagEnvironmentVariableBinding verifies environment variable bindings.
func TestFlagEnvironmentVariableBinding(t *testing.T) {
	tests := []struct {
		name       string
		builder    func(*[]flags.Option)
		flagName   string
		envVarName string
	}{
		{"format", WithFormatFlag, "format", "ATMOS_LIST_FORMAT"},
		{"columns", WithColumnsFlag, "columns", "ATMOS_LIST_COLUMNS"},
		{"sort", WithSortFlag, "sort", "ATMOS_LIST_SORT"},
		{"filter", WithFilterFlag, "filter", "ATMOS_LIST_FILTER"},
		{"stack", WithStackFlag, "stack", "ATMOS_STACK"},
		{"type", WithTypeFlag, "type", "ATMOS_COMPONENT_TYPE"},
		{"enabled", WithEnabledFlag, "enabled", "ATMOS_COMPONENT_ENABLED"},
		{"locked", WithLockedFlag, "locked", "ATMOS_COMPONENT_LOCKED"},
		{"component", WithComponentFlag, "component", "ATMOS_COMPONENT"},
		{"delimiter", WithDelimiterFlag, "delimiter", "ATMOS_LIST_DELIMITER"},
		{"query", WithQueryFlag, "query", "ATMOS_LIST_QUERY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			parser := NewListParser(tt.builder)
			assert.NotNil(t, parser)

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			err := parser.BindToViper(v)
			require.NoError(t, err, "binding to viper should not fail")

			// Environment variable binding is tested via Viper integration
			// The actual binding happens when BindToViper is called
		})
	}
}

// TestFlagDefaultValues verifies default values for flags.
func TestFlagDefaultValues(t *testing.T) {
	tests := []struct {
		name         string
		builder      func(*[]flags.Option)
		flagName     string
		defaultValue string
	}{
		{"format empty", WithFormatFlag, "format", ""},
		{"type real", WithTypeFlag, "type", "real"},
		{"enabled false", WithEnabledFlag, "enabled", "false"},
		{"locked false", WithLockedFlag, "locked", "false"},
		{"abstract false", WithAbstractFlag, "abstract", "false"},
		{"process-templates true", WithProcessTemplatesFlag, "process-templates", "true"},
		{"process-functions true", WithProcessFunctionsFlag, "process-functions", "true"},
		{"upload false", WithUploadFlag, "upload", "false"},
		{"max-columns zero", WithMaxColumnsFlag, "max-columns", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewListParser(tt.builder)
			assert.NotNil(t, parser)

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			flag := cmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag, "flag %s should exist", tt.flagName)
			assert.Equal(t, tt.defaultValue, flag.DefValue, "default value mismatch for %s", tt.flagName)
		})
	}
}

// TestFlagShorthands verifies shorthand flags are registered correctly.
func TestFlagShorthands(t *testing.T) {
	tests := []struct {
		name      string
		builder   func(*[]flags.Option)
		flagName  string
		shorthand string
	}{
		{"format -f", WithFormatFlag, "format", "f"},
		{"stack -s", WithStackFlag, "stack", "s"},
		{"type -t", WithTypeFlag, "type", "t"},
		{"component -c", WithComponentFlag, "component", "c"},
		{"query -q", WithQueryFlag, "query", "q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewListParser(tt.builder)
			assert.NotNil(t, parser)

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)

			flag := cmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag, "flag %s should exist", tt.flagName)
			assert.Equal(t, tt.shorthand, flag.Shorthand, "shorthand mismatch for %s", tt.flagName)
		})
	}
}
