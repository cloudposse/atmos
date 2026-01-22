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

// TestGetAliasColumnsDefault tests getAliasColumns with no custom columns.
func TestGetAliasColumnsDefault(t *testing.T) {
	columns := getAliasColumns(nil)

	assert.Len(t, columns, 3)
	assert.Equal(t, "Alias", columns[0].Name)
	assert.Equal(t, "Command", columns[1].Name)
	assert.Equal(t, "Type", columns[2].Name)
}

// TestGetAliasColumnsCustom tests getAliasColumns with custom column selection.
func TestGetAliasColumnsCustom(t *testing.T) {
	columns := getAliasColumns([]string{"alias", "command"})

	assert.Len(t, columns, 2)
	assert.Equal(t, "Alias", columns[0].Name)
	assert.Equal(t, "Command", columns[1].Name)
}

// TestGetAliasColumnsInvalidFallback tests getAliasColumns falls back to default with invalid columns.
func TestGetAliasColumnsInvalidFallback(t *testing.T) {
	columns := getAliasColumns([]string{"invalid", "unknown"})

	// Should fall back to defaults when no valid columns found.
	assert.Len(t, columns, 3)
}

// TestBuildAliasSortersDefault tests buildAliasSorters with empty spec (default sort).
func TestBuildAliasSortersDefault(t *testing.T) {
	sorters, err := buildAliasSorters("")

	assert.NoError(t, err)
	assert.Len(t, sorters, 1)
	assert.Equal(t, "Alias", sorters[0].Column)
}

// TestBuildAliasSortersCustom tests buildAliasSorters with custom sort spec.
func TestBuildAliasSortersCustom(t *testing.T) {
	sorters, err := buildAliasSorters("type:desc,alias:asc")

	assert.NoError(t, err)
	assert.Len(t, sorters, 2)
}

// TestBuildAliasFooter tests the buildAliasFooter function.
func TestBuildAliasFooter(t *testing.T) {
	aliases := []AliasInfo{
		{Alias: "ls", Command: "list stacks", Type: aliasTypeConfigured},
		{Alias: "tf", Command: "terraform", Type: aliasTypeBuiltIn},
		{Alias: "tp", Command: "terraform plan", Type: aliasTypeConfigured},
	}

	footer := buildAliasFooter(aliases)

	assert.Contains(t, footer, "3 aliases")
	assert.Contains(t, footer, "1 built-in")
	assert.Contains(t, footer, "2 configured")
}

// TestBuildAliasFooterSingular tests buildAliasFooter with singular alias count.
func TestBuildAliasFooterSingular(t *testing.T) {
	aliases := []AliasInfo{
		{Alias: "tf", Command: "terraform", Type: aliasTypeBuiltIn},
	}

	footer := buildAliasFooter(aliases)

	assert.Contains(t, footer, "1 alias")
	assert.Contains(t, footer, "1 built-in")
	assert.Contains(t, footer, "0 configured")
}

// TestAliasesFormatFlag tests that the format flag is registered.
func TestAliasesFormatFlag(t *testing.T) {
	formatFlag := aliasesCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag, "Expected format flag to exist")
	assert.Equal(t, "", formatFlag.DefValue)
	assert.Equal(t, "f", formatFlag.Shorthand)
}

// TestAliasesColumnsFlag tests that the columns flag is registered.
func TestAliasesColumnsFlag(t *testing.T) {
	columnsFlag := aliasesCmd.Flags().Lookup("columns")
	assert.NotNil(t, columnsFlag, "Expected columns flag to exist")
}

// TestAliasesSortFlag tests that the sort flag is registered.
func TestAliasesSortFlag(t *testing.T) {
	sortFlag := aliasesCmd.Flags().Lookup("sort")
	assert.NotNil(t, sortFlag, "Expected sort flag to exist")
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
	assert.Contains(t, aliasesCmd.Example, "--columns")
	assert.Contains(t, aliasesCmd.Example, "--sort")
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
	// The nested command "plan" has alias "p" under "terraform".
	// Alias paths exclude root command name, so it's "terraform p" not "atmos terraform p".
	foundPlan := false
	for _, a := range aliases {
		if a.Alias == "terraform p" && a.Command == "terraform plan" {
			foundPlan = true
		}
	}
	assert.True(t, foundPlan, "Should find nested alias 'terraform p' for 'terraform plan'")
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

	// Aliases should be sorted alphabetically.
	for i := 1; i < len(allAliases); i++ {
		assert.LessOrEqual(t, allAliases[i-1].Alias, allAliases[i].Alias, "Aliases should be sorted alphabetically")
	}

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

// TestStripRootPrefix tests the stripRootPrefix function.
func TestStripRootPrefix(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		rootName string
		expected string
	}{
		{
			name:     "strips root prefix",
			path:     "atmos terraform plan",
			rootName: "atmos",
			expected: "terraform plan",
		},
		{
			name:     "no prefix to strip",
			path:     "terraform plan",
			rootName: "atmos",
			expected: "terraform plan",
		},
		{
			name:     "empty path",
			path:     "",
			rootName: "atmos",
			expected: "",
		},
		{
			name:     "path equals root name",
			path:     "atmos",
			rootName: "atmos",
			expected: "atmos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripRootPrefix(tt.path, tt.rootName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCollectCommandAliases tests the collectCommandAliases function.
func TestCollectCommandAliases(t *testing.T) {
	rootCmd := &cobra.Command{Use: "atmos"}
	tfCmd := &cobra.Command{Use: "terraform", Aliases: []string{"tf", "terra"}}
	rootCmd.AddCommand(tfCmd)

	// Test top-level command aliases.
	aliases := collectCommandAliases(tfCmd, "atmos", "atmos terraform", "atmos")

	// Should have 2 aliases: tf and terra.
	assert.Len(t, aliases, 2)

	// Verify alias paths (top-level commands just use the alias).
	foundTf := false
	foundTerra := false
	for _, a := range aliases {
		if a.Alias == "tf" && a.Command == "terraform" && a.Type == aliasTypeBuiltIn {
			foundTf = true
		}
		if a.Alias == "terra" && a.Command == "terraform" && a.Type == aliasTypeBuiltIn {
			foundTerra = true
		}
	}
	assert.True(t, foundTf, "Should find 'tf' alias")
	assert.True(t, foundTerra, "Should find 'terra' alias")
}

// TestCollectCommandAliasesNested tests collectCommandAliases with nested commands.
func TestCollectCommandAliasesNested(t *testing.T) {
	rootCmd := &cobra.Command{Use: "atmos"}
	tfCmd := &cobra.Command{Use: "terraform"}
	planCmd := &cobra.Command{Use: "plan", Aliases: []string{"p"}}

	rootCmd.AddCommand(tfCmd)
	tfCmd.AddCommand(planCmd)

	// Test nested command aliases.
	aliases := collectCommandAliases(planCmd, "atmos terraform", "atmos terraform plan", "atmos")

	// Should have 1 alias: terraform p -> terraform plan.
	assert.Len(t, aliases, 1)
	assert.Equal(t, "terraform p", aliases[0].Alias)
	assert.Equal(t, "terraform plan", aliases[0].Command)
}

// TestCollectAllAliasesEmptyConfigured tests collectAllAliases with no configured aliases.
func TestCollectAllAliasesEmptyConfigured(t *testing.T) {
	rootCmd := &cobra.Command{Use: "atmos"}
	tfCmd := &cobra.Command{Use: "terraform", Aliases: []string{"tf"}}
	rootCmd.AddCommand(tfCmd)

	emptyConfigured := schema.CommandAliases{}

	allAliases := collectAllAliases(rootCmd, emptyConfigured)

	// Should have only 1 built-in alias.
	assert.Len(t, allAliases, 1)
	assert.Equal(t, "tf", allAliases[0].Alias)
	assert.Equal(t, aliasTypeBuiltIn, allAliases[0].Type)
}

// TestCollectAllAliasesEmptyBuiltIn tests collectAllAliases with no built-in aliases.
func TestCollectAllAliasesEmptyBuiltIn(t *testing.T) {
	rootCmd := &cobra.Command{Use: "atmos"}
	tfCmd := &cobra.Command{Use: "terraform"} // No aliases.
	rootCmd.AddCommand(tfCmd)

	configuredAliases := schema.CommandAliases{
		"tp": "terraform plan",
	}

	allAliases := collectAllAliases(rootCmd, configuredAliases)

	// Should have only 1 configured alias.
	assert.Len(t, allAliases, 1)
	assert.Equal(t, "tp", allAliases[0].Alias)
	assert.Equal(t, aliasTypeConfigured, allAliases[0].Type)
}

// TestCollectBuiltInAliasesSkipsHelp tests that the help command is skipped.
func TestCollectBuiltInAliasesSkipsHelp(t *testing.T) {
	rootCmd := &cobra.Command{Use: "atmos"}
	helpCmd := &cobra.Command{Use: "help", Aliases: []string{"h"}}
	rootCmd.AddCommand(helpCmd)

	aliases := collectBuiltInAliases(rootCmd, "")

	// Should not find any aliases for the help command.
	for _, a := range aliases {
		assert.NotEqual(t, "h", a.Alias, "Should not find alias for help command")
	}
}

// TestGetAliasColumnsCaseInsensitive tests getAliasColumns with mixed case column names.
func TestGetAliasColumnsCaseInsensitive(t *testing.T) {
	columns := getAliasColumns([]string{"ALIAS", "Command", "TYPE"})

	// Should handle case-insensitive column names.
	assert.Len(t, columns, 3)
	assert.Equal(t, "Alias", columns[0].Name)
	assert.Equal(t, "Command", columns[1].Name)
	assert.Equal(t, "Type", columns[2].Name)
}

// TestGetAliasColumnsPartialValid tests getAliasColumns with some valid and some invalid columns.
func TestGetAliasColumnsPartialValid(t *testing.T) {
	columns := getAliasColumns([]string{"alias", "invalid", "type"})

	// Should return only valid columns.
	assert.Len(t, columns, 2)
	assert.Equal(t, "Alias", columns[0].Name)
	assert.Equal(t, "Type", columns[1].Name)
}

// TestBuildAliasSortersInvalidSpec tests buildAliasSorters with an invalid sort spec.
func TestBuildAliasSortersInvalidSpec(t *testing.T) {
	_, err := buildAliasSorters("invalid::spec")

	// Should return an error for invalid sort spec.
	assert.Error(t, err)
}

// TestBuildAliasFooterEmpty tests buildAliasFooter with empty aliases.
func TestBuildAliasFooterEmpty(t *testing.T) {
	aliases := []AliasInfo{}

	footer := buildAliasFooter(aliases)

	assert.Contains(t, footer, "0 aliases")
	assert.Contains(t, footer, "0 built-in")
	assert.Contains(t, footer, "0 configured")
}

// TestAliasesToDataEmpty tests aliasesToData with empty slice.
func TestAliasesToDataEmpty(t *testing.T) {
	aliases := []AliasInfo{}

	data := aliasesToData(aliases)

	assert.Len(t, data, 0)
	assert.NotNil(t, data)
}

// TestAliasesOptionsAllFields tests AliasesOptions with all fields populated.
func TestAliasesOptionsAllFields(t *testing.T) {
	opts := &AliasesOptions{
		Format:  "yaml",
		Columns: []string{"alias", "command"},
		Sort:    "alias:asc",
	}
	assert.Equal(t, "yaml", opts.Format)
	assert.Equal(t, []string{"alias", "command"}, opts.Columns)
	assert.Equal(t, "alias:asc", opts.Sort)
}
