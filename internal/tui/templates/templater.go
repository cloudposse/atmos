package templates

import (
	"fmt"
	"strings"

	"github.com/elewis787/boa"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Templater handles the generation and management of command usage templates.
type Templater struct {
	UsageTemplate string
}

// SetCustomUsageFunc configures a custom usage template for the provided cobra command.
// It returns an error if the command is nil.
func SetCustomUsageFunc(cmd *cobra.Command, b *boa.Boa) error {
	if cmd == nil {
		return fmt.Errorf("command cannot be nil")
	}
	t := &Templater{
		UsageTemplate: MainUsageTemplate(),
	}

	cmd.SetUsageTemplate(t.UsageTemplate)
	return nil
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

// Default terminal width if actual width cannot be determined
const maxWidth = 80

// WrappedFlagUsages formats the flag usage string to fit within the terminal width
func WrappedFlagUsages(f *pflag.FlagSet) string {
	var builder strings.Builder

	printer := NewHelpFlagPrinter(&builder, maxWidth)

	f.VisitAll(func(flag *pflag.Flag) {
		printer.PrintHelpFlag(flag)
	})

	return builder.String()
}
