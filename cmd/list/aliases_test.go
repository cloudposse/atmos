package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestListAliasesCommand tests the aliases command structure.
func TestListAliasesCommand(t *testing.T) {
	assert.Equal(t, "aliases", aliasesCmd.Use)
	assert.Contains(t, aliasesCmd.Short, "List configured command aliases")
	assert.NotNil(t, aliasesCmd.RunE)
	assert.NotEmpty(t, aliasesCmd.Example)

	// Check that NoArgs validator is set.
	err := aliasesCmd.Args(aliasesCmd, []string{"unexpected"})
	assert.Error(t, err, "Should reject extra arguments")

	err = aliasesCmd.Args(aliasesCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")
}

// TestListAliasesValidatesArgs tests that the command validates arguments.
func TestListAliasesValidatesArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "aliases",
		Args: cobra.NoArgs,
	}

	err := cmd.ValidateArgs([]string{})
	assert.NoError(t, err, "Validation should pass with no arguments")

	err = cmd.ValidateArgs([]string{"extra"})
	assert.Error(t, err, "Validation should fail with arguments")
}

// TestAliasesOptions tests the AliasesOptions structure.
func TestAliasesOptions(t *testing.T) {
	opts := &AliasesOptions{}
	assert.NotNil(t, opts)
}

// TestFormatAliasesTable tests the formatAliasesTable function.
func TestFormatAliasesTable(t *testing.T) {
	aliases := schema.CommandAliases{
		"tf": "terraform",
		"tp": "terraform plan",
		"ls": "list stacks",
	}

	output := formatAliasesTable(aliases)

	assert.Contains(t, output, "tf")
	assert.Contains(t, output, "terraform")
	assert.Contains(t, output, "tp")
	assert.Contains(t, output, "terraform plan")
	assert.Contains(t, output, "ls")
	assert.Contains(t, output, "list stacks")
	assert.Contains(t, output, "3 aliases configured")
}

// TestFormatSimpleAliasesOutput tests the formatSimpleAliasesOutput function.
func TestFormatSimpleAliasesOutput(t *testing.T) {
	aliases := schema.CommandAliases{
		"tf": "terraform",
		"tp": "terraform plan",
	}

	output := formatSimpleAliasesOutput(aliases)

	assert.Contains(t, output, "Alias")
	assert.Contains(t, output, "Command")
	assert.Contains(t, output, "tf")
	assert.Contains(t, output, "terraform")
	assert.Contains(t, output, "tp")
	assert.Contains(t, output, "terraform plan")
	assert.Contains(t, output, "2 aliases configured")
}

// TestFormatSimpleAliasesOutputSingular tests singular alias count.
func TestFormatSimpleAliasesOutputSingular(t *testing.T) {
	aliases := schema.CommandAliases{
		"tf": "terraform",
	}

	output := formatSimpleAliasesOutput(aliases)

	assert.Contains(t, output, "1 alias configured")
	assert.NotContains(t, output, "aliases configured")
}

// TestFormatAliasesTableSingular tests singular alias count in table format.
func TestFormatAliasesTableSingular(t *testing.T) {
	aliases := schema.CommandAliases{
		"tf": "terraform",
	}

	output := formatAliasesTable(aliases)

	assert.Contains(t, output, "1 alias configured")
	assert.NotContains(t, output, "aliases configured")
}

// TestFormatAliasesTableSortOrder tests that aliases are sorted alphabetically.
func TestFormatAliasesTableSortOrder(t *testing.T) {
	aliases := schema.CommandAliases{
		"zulu":  "last",
		"alpha": "first",
		"mike":  "middle",
	}

	output := formatSimpleAliasesOutput(aliases)

	// Check that alpha appears before mike and mike appears before zulu.
	alphaIdx := indexOf(output, "alpha")
	mikeIdx := indexOf(output, "mike")
	zuluIdx := indexOf(output, "zulu")

	assert.Less(t, alphaIdx, mikeIdx, "alpha should appear before mike")
	assert.Less(t, mikeIdx, zuluIdx, "mike should appear before zulu")
}

// indexOf returns the index of the first occurrence of substr in s.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
