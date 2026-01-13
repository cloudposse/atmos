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
	assert.Contains(t, aliasesCmd.Short, "command aliases")
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

// TestAliasInfo tests the AliasInfo structure.
func TestAliasInfo(t *testing.T) {
	alias := AliasInfo{
		Alias:   "tf",
		Command: "terraform",
		Type:    aliasTypeBuiltIn,
	}
	assert.Equal(t, "tf", alias.Alias)
	assert.Equal(t, "terraform", alias.Command)
	assert.Equal(t, "built-in", alias.Type)
}

// TestFormatAliasesTable tests the formatAliasesTable function.
func TestFormatAliasesTable(t *testing.T) {
	aliases := []AliasInfo{
		{Alias: "ls", Command: "list stacks", Type: aliasTypeConfigured},
		{Alias: "tf", Command: "terraform", Type: aliasTypeBuiltIn},
		{Alias: "tp", Command: "terraform plan", Type: aliasTypeConfigured},
	}

	output := formatAliasesTable(aliases)

	assert.Contains(t, output, "tf")
	assert.Contains(t, output, "terraform")
	assert.Contains(t, output, "tp")
	assert.Contains(t, output, "terraform plan")
	assert.Contains(t, output, "ls")
	assert.Contains(t, output, "list stacks")
	assert.Contains(t, output, "3 aliases")
	assert.Contains(t, output, "1 built-in")
	assert.Contains(t, output, "2 configured")
}

// TestFormatSimpleAliasesOutput tests the formatSimpleAliasesOutput function.
func TestFormatSimpleAliasesOutput(t *testing.T) {
	aliases := []AliasInfo{
		{Alias: "tf", Command: "terraform", Type: aliasTypeBuiltIn},
		{Alias: "tp", Command: "terraform plan", Type: aliasTypeConfigured},
	}

	output := formatSimpleAliasesOutput(aliases)

	assert.Contains(t, output, "Alias")
	assert.Contains(t, output, "Command")
	assert.Contains(t, output, "Type")
	assert.Contains(t, output, "tf")
	assert.Contains(t, output, "terraform")
	assert.Contains(t, output, "tp")
	assert.Contains(t, output, "terraform plan")
	assert.Contains(t, output, "2 aliases")
	assert.Contains(t, output, "1 built-in")
	assert.Contains(t, output, "1 configured")
}

// TestFormatSimpleAliasesOutputSingular tests singular alias count.
func TestFormatSimpleAliasesOutputSingular(t *testing.T) {
	aliases := []AliasInfo{
		{Alias: "tf", Command: "terraform", Type: aliasTypeBuiltIn},
	}

	output := formatSimpleAliasesOutput(aliases)

	assert.Contains(t, output, "1 alias")
	assert.Contains(t, output, "1 built-in")
	assert.Contains(t, output, "0 configured")
}

// TestFormatAliasesTableSingular tests singular alias count in table format.
func TestFormatAliasesTableSingular(t *testing.T) {
	aliases := []AliasInfo{
		{Alias: "tf", Command: "terraform", Type: aliasTypeConfigured},
	}

	output := formatAliasesTable(aliases)

	assert.Contains(t, output, "1 alias")
	assert.Contains(t, output, "0 built-in")
	assert.Contains(t, output, "1 configured")
}

// TestAliasesFormatFlag tests that the format flag is registered.
func TestAliasesFormatFlag(t *testing.T) {
	formatFlag := aliasesCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag, "Expected format flag to exist")
	assert.Equal(t, "", formatFlag.DefValue)
	assert.Equal(t, "f", formatFlag.Shorthand)
}

// TestAliasesToData tests the aliasesToData conversion function.
func TestAliasesToData(t *testing.T) {
	aliases := []AliasInfo{
		{Alias: "alpha", Command: "first command", Type: aliasTypeConfigured},
		{Alias: "mike", Command: "middle command", Type: aliasTypeBuiltIn},
		{Alias: "zulu", Command: "last command", Type: aliasTypeConfigured},
	}

	data := aliasesToData(aliases)

	// Should have 3 entries.
	assert.Len(t, data, 3)

	// Should preserve order (already sorted by caller).
	assert.Equal(t, "alpha", data[0]["alias"])
	assert.Equal(t, "first command", data[0]["command"])
	assert.Equal(t, "configured", data[0]["type"])
	assert.Equal(t, "mike", data[1]["alias"])
	assert.Equal(t, "middle command", data[1]["command"])
	assert.Equal(t, "built-in", data[1]["type"])
	assert.Equal(t, "zulu", data[2]["alias"])
	assert.Equal(t, "last command", data[2]["command"])
	assert.Equal(t, "configured", data[2]["type"])
}

// TestAliasesOptionsFormat tests that AliasesOptions includes Format field.
func TestAliasesOptionsFormat(t *testing.T) {
	opts := &AliasesOptions{
		Format: "json",
	}
	assert.Equal(t, "json", opts.Format)
}

// TestAliasesExamples tests that the command examples include format flag usage.
func TestAliasesExamples(t *testing.T) {
	assert.Contains(t, aliasesCmd.Example, "atmos list aliases")
	assert.Contains(t, aliasesCmd.Example, "--format json")
	assert.Contains(t, aliasesCmd.Example, "--format yaml")
}

// TestCollectBuiltInAliases tests the collectBuiltInAliases function.
func TestCollectBuiltInAliases(t *testing.T) {
	// Create a mock command tree.
	rootCmd := &cobra.Command{Use: "atmos"}
	tfCmd := &cobra.Command{Use: "terraform", Aliases: []string{"tf"}}
	hfCmd := &cobra.Command{Use: "helmfile", Aliases: []string{"hf"}}
	planCmd := &cobra.Command{Use: "plan", Aliases: []string{"p"}}

	rootCmd.AddCommand(tfCmd)
	rootCmd.AddCommand(hfCmd)
	tfCmd.AddCommand(planCmd)

	aliases := collectBuiltInAliases(rootCmd, "")

	// Should find tf, hf, and a nested plan alias.
	assert.GreaterOrEqual(t, len(aliases), 3)

	// Verify tf alias exists (maps to "terraform", not "atmos terraform").
	foundTf := false
	for _, a := range aliases {
		if a.Alias == "tf" && a.Command == "terraform" && a.Type == aliasTypeBuiltIn {
			foundTf = true
		}
	}
	assert.True(t, foundTf, "Should find 'tf' alias for 'terraform'")

	// Verify hf alias exists.
	foundHf := false
	for _, a := range aliases {
		if a.Alias == "hf" && a.Command == "helmfile" && a.Type == aliasTypeBuiltIn {
			foundHf = true
		}
	}
	assert.True(t, foundHf, "Should find 'hf' alias for 'helmfile'")

	// Verify nested alias exists.
	// The nested command "plan" has alias "p" under "atmos terraform".
	// Since parent path is "atmos terraform" (not just root name), alias becomes "atmos terraform p".
	foundPlan := false
	for _, a := range aliases {
		// Look for any alias containing "p" that maps to something with "plan".
		if a.Alias == "atmos terraform p" && a.Command == "terraform plan" {
			foundPlan = true
		}
	}
	assert.True(t, foundPlan, "Should find nested alias 'atmos terraform p' for 'terraform plan'")
}

// TestCollectAllAliases tests the collectAllAliases function.
func TestCollectAllAliases(t *testing.T) {
	// Create a mock command tree.
	rootCmd := &cobra.Command{Use: "atmos"}
	tfCmd := &cobra.Command{Use: "terraform", Aliases: []string{"tf"}}
	rootCmd.AddCommand(tfCmd)

	configuredAliases := schema.CommandAliases{
		"ls": "list stacks",
		"tp": "terraform plan",
	}

	allAliases := collectAllAliases(rootCmd, configuredAliases)

	// Should have at least 3 aliases (1 built-in + 2 configured).
	assert.GreaterOrEqual(t, len(allAliases), 3)

	// Built-in should come first.
	assert.Equal(t, aliasTypeBuiltIn, allAliases[0].Type)

	// Verify configured aliases are present.
	foundLs := false
	foundTp := false
	for _, a := range allAliases {
		if a.Alias == "ls" && a.Command == "list stacks" && a.Type == aliasTypeConfigured {
			foundLs = true
		}
		if a.Alias == "tp" && a.Command == "terraform plan" && a.Type == aliasTypeConfigured {
			foundTp = true
		}
	}
	assert.True(t, foundLs, "Should find configured alias 'ls'")
	assert.True(t, foundTp, "Should find configured alias 'tp'")
}

// TestCollectBuiltInAliasesSkipsHidden tests that hidden commands are skipped.
func TestCollectBuiltInAliasesSkipsHidden(t *testing.T) {
	rootCmd := &cobra.Command{Use: "atmos"}
	hiddenCmd := &cobra.Command{Use: "hidden", Aliases: []string{"h"}, Hidden: true}
	visibleCmd := &cobra.Command{Use: "visible", Aliases: []string{"v"}}

	rootCmd.AddCommand(hiddenCmd)
	rootCmd.AddCommand(visibleCmd)

	aliases := collectBuiltInAliases(rootCmd, "")

	// Should find 'v' (mapping to "visible") but not 'h'.
	foundHidden := false
	foundVisible := false
	for _, a := range aliases {
		if a.Alias == "h" {
			foundHidden = true
		}
		if a.Alias == "v" && a.Command == "visible" {
			foundVisible = true
		}
	}
	assert.False(t, foundHidden, "Should not find hidden command alias")
	assert.True(t, foundVisible, "Should find visible command alias 'v' → 'visible'")
}
