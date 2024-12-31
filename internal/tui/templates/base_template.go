package templates

import "fmt"

type HelpTemplateSections int

const (
	LongDescription HelpTemplateSections = iota
	Usage
	Aliases
	Examples
	AvailableCommands
	Flags
	GlobalFlags
	AdditionalHelpTopics
	NativeCommands
	DoubleDashHelp
	Footer
)

func GenerateFromBaseTemplate(commandName string, parts []HelpTemplateSections) string {
	template := ""
	for _, value := range parts {
		template += getSection(commandName, value)
	}
	return template
}

func getSection(commandName string, section HelpTemplateSections) string {
	switch section {
	case LongDescription:
		return `{{ .Long }}
`
	case AdditionalHelpTopics:
		return `{{if .HasHelpSubCommands}}

Additional help topics:
{{formatCommands .Commands "additionalHelpTopics"}}{{end}}`
	case Aliases:
		return `{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}`
	case AvailableCommands:
		return `{{if .HasAvailableSubCommands}}

Available Commands:
{{formatCommands .Commands "availableCommands"}}{{end}}`
	case Examples:
		return `{{if .HasExample}}

Examples:
{{.Example}}{{end}}`
	case Flags:
		return `{{if .HasAvailableLocalFlags}}

Flags:
{{wrappedFlagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}`
	case GlobalFlags:
		return `{{if .HasAvailableInheritedFlags}}

Global Flags:
{{wrappedFlagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}`
	case NativeCommands:
		return fmt.Sprintf(`
{{HeadingStyle "Native %s Commands:"}}

{{formatCommands .Commands "native"}}
`, commandName)
	case Usage:
		return `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}`
	case DoubleDashHelp:
		return fmt.Sprintf(`

The '--' (double-dash) can be used to signify the end of Atmos-specific options 
and the beginning of additional native arguments and flags for the specific command being run.

Example:
  atmos %s <subcommand> <component> -s <stack> -- <native-flags>`, commandName)
	case Footer:
		return `{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}`
	default:
		return ""
	}
}
