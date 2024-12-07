package templates

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

// Templater handles the generation and management of command usage templates.
type Templater struct {
	UsageTemplate string
}

// commandStyle defines the styles for command formatting
var (
	commandNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("2")). // Green color for command name
				Bold(true)

	commandDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("7")) // White color for description
)

// formatCommand returns a styled string for a command and its description
func formatCommand(name string, desc string, padding int) string {
	paddedName := fmt.Sprintf("%-*s", padding, name)
	styledName := commandNameStyle.Render(paddedName)
	styledDesc := commandDescStyle.Render(desc)
	return fmt.Sprintf("  %s    %s", styledName, styledDesc)
}

// formatCommands formats a slice of cobra commands with proper styling
func formatCommands(cmds []*cobra.Command) string {
	var maxLen int
	availableCmds := make([]*cobra.Command, 0)

	// First pass: collect available commands and find max length
	for _, cmd := range cmds {
		if cmd.IsAvailableCommand() || cmd.Name() == "help" {
			availableCmds = append(availableCmds, cmd)
			if len(cmd.Name()) > maxLen {
				maxLen = len(cmd.Name())
			}
		}
	}

	var lines []string
	for _, cmd := range availableCmds {
		lines = append(lines, formatCommand(cmd.Name(), cmd.Short, maxLen))
	}

	return strings.Join(lines, "\n")
}

// SetCustomUsageFunc configures a custom usage template for the provided cobra command.
// It returns an error if the command is nil.
func SetCustomUsageFunc(cmd *cobra.Command) error {
	if cmd == nil {
		return fmt.Errorf("command cannot be nil")
	}
	t := &Templater{
		UsageTemplate: MainUsageTemplate(),
	}

	cmd.SetUsageTemplate(t.UsageTemplate)
	cobra.AddTemplateFunc("formatCommands", formatCommands)
	return nil
}

// getTerminalWidth returns the width of the terminal, defaulting to 80 if it cannot be determined
func getTerminalWidth() int {
	defaultWidth := 80
	screenWidth := defaultWidth

	// Detect terminal width and use it by default if available
	if term.IsTerminal(int(os.Stdout.Fd())) {
		termWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil && termWidth > 0 {
			screenWidth = termWidth - 2
		}
	}

	return screenWidth
}

// MainUsageTemplate returns the usage template for the root command and wrap cobra flag usages to the terminal width
func MainUsageTemplate() string {
	return `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:
{{formatCommands .Commands}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{wrappedFlagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{wrappedFlagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
}

// WrappedFlagUsages formats the flag usage string to fit within the terminal width
func WrappedFlagUsages(f *pflag.FlagSet) string {
	var builder strings.Builder
	width := getTerminalWidth()
	printer := NewHelpFlagPrinter(&builder, uint(width), f)

	printer.maxFlagLen = calculateMaxFlagLength(f)

	f.VisitAll(func(flag *pflag.Flag) {
		printer.PrintHelpFlag(flag)
	})

	return builder.String()
}
