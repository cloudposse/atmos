package templates

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

// Templater handles the generation and management of command usage templates.
type Templater struct {
	UsageTemplate string
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

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

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
