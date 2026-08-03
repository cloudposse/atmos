package templates

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/terminal"
)

func runnableCommand(use, short string, annotations map[string]string) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Annotations: annotations,
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
}

func TestCommandAnnotationHelpers(t *testing.T) {
	assert.False(t, isConfigAlias(&cobra.Command{}))
	assert.True(t, isConfigAlias(&cobra.Command{
		Annotations: map[string]string{annotationConfigAlias: "terraform"},
	}))

	assert.False(t, isCustomCommand(&cobra.Command{
		Annotations: map[string]string{annotationCustomCommand: "false"},
	}))
	assert.True(t, isCustomCommand(&cobra.Command{
		Annotations: map[string]string{annotationCustomCommand: annotationValueTrue},
	}))

	assert.False(t, isNativeCommand(&cobra.Command{
		Annotations: map[string]string{annotationNativeCommand: "false"},
	}))
	assert.True(t, isNativeCommand(&cobra.Command{
		Annotations: map[string]string{annotationNativeCommand: annotationValueTrue},
	}))
}

func TestHasCommandsBySection(t *testing.T) {
	commands := []*cobra.Command{
		runnableCommand("builtin", "Built in", nil),
		runnableCommand("custom", "Custom", map[string]string{
			annotationCustomCommand: annotationValueTrue,
		}),
		runnableCommand("native", "Native", map[string]string{
			annotationNativeCommand: annotationValueTrue,
		}),
		runnableCommand("alias", "Alias", map[string]string{
			annotationConfigAlias: "terraform",
		}),
	}

	assert.True(t, hasCommands(commands, commandListTypeBuiltInCommands))
	assert.True(t, hasCommands(commands, commandListTypeCustomCommands))
	assert.True(t, hasCommands(commands, commandListTypeNative))
	assert.True(t, hasCommands(commands, ""))
	assert.False(t, hasCommands([]*cobra.Command{
		runnableCommand("alias", "Alias", map[string]string{annotationConfigAlias: "terraform"}),
	}, commandListTypeBuiltInCommands))
	assert.False(t, hasCommands([]*cobra.Command{
		runnableCommand("alias", "Alias", map[string]string{
			annotationConfigAlias:   "terraform",
			annotationCustomCommand: annotationValueTrue,
		}),
	}, commandListTypeCustomCommands))
}

func TestHasCommandsIncludesHelpCommand(t *testing.T) {
	commands := []*cobra.Command{
		{Use: helpCommandName, Short: "Cobra help command"},
	}

	assert.True(t, hasCommands(commands, commandListTypeBuiltInCommands))
}

func TestFilterCommandsReturnsAliasesOnlyWhenRequested(t *testing.T) {
	commands := []*cobra.Command{
		runnableCommand("terraform", "Terraform", nil),
		runnableCommand("plan", "Plan", nil),
	}
	commands[0].Aliases = []string{"tf", "plan"}

	filtered := filterCommands(commands, false)
	assert.Equal(t, commands, filtered)

	aliases := filterCommands(commands, true)
	assert.Len(t, aliases, 1)
	assert.Equal(t, "tf", aliases[0].Use)
	assert.Equal(t, `Alias of "terraform" command`, aliases[0].Short)
}

func TestCommandAvailabilityHelpers(t *testing.T) {
	commands := []*cobra.Command{
		runnableCommand("terraform", "Terraform", map[string]string{
			annotationNativeCommand: annotationValueTrue,
		}),
		runnableCommand("deploy", "Deploy", nil),
	}
	commands[1].Aliases = []string{"dep"}

	assert.True(t, isNativeCommandsAvailable(commands))
	assert.True(t, isAliasesPresent(commands))

	assert.False(t, isNativeCommandsAvailable([]*cobra.Command{
		runnableCommand("deploy", "Deploy", nil),
	}))
	assert.False(t, isAliasesPresent([]*cobra.Command{
		runnableCommand("deploy", "Deploy", nil),
	}))
}

func TestFormatCommandsSeparatesBuiltInCustomAndNativeCommands(t *testing.T) {
	commands := []*cobra.Command{
		runnableCommand("builtin", "Built in command", nil),
		runnableCommand("custom", "Custom command", map[string]string{
			annotationCustomCommand: annotationValueTrue,
		}),
		runnableCommand("native", "Native command", map[string]string{
			annotationNativeCommand: annotationValueTrue,
		}),
		runnableCommand("alias", "Config alias", map[string]string{
			annotationConfigAlias: "terraform",
		}),
	}

	builtIns := formatCommands(commands, commandListTypeBuiltInCommands)
	assert.Contains(t, builtIns, "builtin")
	assert.NotContains(t, builtIns, "custom")
	assert.NotContains(t, builtIns, "native")
	assert.NotContains(t, builtIns, "alias")

	custom := formatCommands(commands, commandListTypeCustomCommands)
	assert.Contains(t, custom, "custom")
	assert.NotContains(t, custom, "builtin")
	assert.NotContains(t, custom, "native")
	assert.NotContains(t, custom, "alias")

	native := formatCommands(commands, commandListTypeNative)
	assert.Contains(t, native, "native")
	assert.NotContains(t, native, "builtin")
	assert.NotContains(t, native, "custom")
	assert.NotContains(t, native, "alias")
}

func TestFormatCommandsSubcommandAliasesUsesAliasHelp(t *testing.T) {
	command := runnableCommand("terraform", "Terraform", nil)
	command.Aliases = []string{"tf"}

	output := formatCommands([]*cobra.Command{command}, commandListTypeSubcommandAliases)

	assert.Contains(t, output, "tf")
	assert.Contains(t, output, `Alias of "terraform" command`)
	assert.NotContains(t, output, "Terraform")
}

func TestSetCustomUsageFunc(t *testing.T) {
	assert.ErrorIs(t, SetCustomUsageFunc(nil), errUtils.ErrCommandNil)

	cmd := &cobra.Command{Use: "atmos", Short: "Atmos"}
	err := SetCustomUsageFunc(cmd)

	assert.NoError(t, err)
	assert.Contains(t, cmd.UsageTemplate(), "CUSTOM COMMANDS")
	assert.Contains(t, cmd.UsageTemplate(), `{{formatCommands .Commands "customCommands"}}`)
}

func TestGetTerminalWidthHonorsColumnsWithoutTTY(t *testing.T) {
	if terminal.New().IsTTY(terminal.Stdout) {
		t.Skip("stdout is a real TTY; non-TTY fallback does not apply")
	}

	t.Setenv("COLUMNS", "120")
	assert.Equal(t, 118, GetTerminalWidth())

	t.Setenv("COLUMNS", "")
	assert.Equal(t, 80, GetTerminalWidth())
}

func TestGetTerminalWidthUsesDetectedWidthAndConfiguredLimit(t *testing.T) {
	if terminal.New().IsTTY(terminal.Stdout) {
		t.Skip("stdout is a real TTY; non-TTY fallback does not apply")
	}

	originalLimit := terminalWidthLimit.Load()
	t.Cleanup(func() { terminalWidthLimit.Store(originalLimit) })

	t.Setenv("COLUMNS", "182")
	SetTerminalWidthLimit(0)
	assert.Equal(t, 180, GetTerminalWidth(), "detected width is unlimited by default")

	SetTerminalWidthLimit(120)
	assert.Equal(t, 120, GetTerminalWidth(), "configured max_width is a ceiling")

	t.Setenv("COLUMNS", "")
	SetTerminalWidthLimit(72)
	assert.Equal(t, 72, GetTerminalWidth(), "the limit also applies to the fallback width")
}

func TestWrappedFlagUsages_DoubleDashAtEnd(t *testing.T) {
	// Create a new FlagSet with various flags
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	// Add regular flags
	fs.StringP("input", "i", "default.txt", "input file path")
	fs.BoolP("quiet", "q", false, "suppress output")
	// Add double dash flag
	fs.StringP("", "", "", "separates flags from arguments")

	// Execute the function
	output := WrappedFlagUsages(fs)

	// Split the output into individual flag entries (assuming double newline separation)
	entries := strings.Split(strings.TrimSpace(output), "\n\n")
	assert.Greater(t, len(entries), 1, "should have multiple flag entries")

	// Find the last non-empty entry
	var lastEntry string
	for i := len(entries) - 1; i >= 0; i-- {
		if strings.TrimSpace(entries[i]) != "" {
			lastEntry = entries[i]
			break
		}
	}
	assert.NotEmpty(t, lastEntry, "should have a non-empty last entry")

	// Verify the double dash is in the last entry
	assert.Contains(t, lastEntry, "-- ", "last entry should contain double dash")
	assert.Contains(t, lastEntry, "separates flags from arguments",
		"last entry should contain double dash usage")

	// Verify other flags appear before the double dash
	inputIndex := -1
	quietIndex := -1
	doubleDashIndex := -1

	for i, entry := range entries {
		if strings.Contains(entry, "--input") {
			inputIndex = i
		}
		if strings.Contains(entry, "--quiet") {
			quietIndex = i
		}
		if strings.Contains(entry, "-- ") {
			doubleDashIndex = i
		}
	}

	assert.Greater(t, doubleDashIndex, inputIndex,
		"double dash should appear after input flag")
	assert.Greater(t, doubleDashIndex, quietIndex,
		"double dash should appear after quiet flag")
	assert.Equal(t, len(entries)-1, doubleDashIndex,
		"double dash should be in the last entry")
}
