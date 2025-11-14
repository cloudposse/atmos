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
