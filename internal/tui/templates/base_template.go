package templates

type HelpTemplateSections int

const (
	LongDescription HelpTemplateSections = iota
	Usage
	Aliases
	SubCommandAliases
	Examples
	AvailableCommands
	Flags
	GlobalFlags
	AdditionalHelpTopics
	NativeCommands
	DoubleDashHelp
	Footer
)

func GenerateFromBaseTemplate(parts []HelpTemplateSections) string {
	template := ""
	for _, value := range parts {
		template += getSection(value)
	}
	return template
}

func getSection(section HelpTemplateSections) string {
	switch section {
	case LongDescription:
		return `{{ .Long }}
`
	case AdditionalHelpTopics:
		return `{{if .HasHelpSubCommands}}


{{HeadingStyle "Additional help topics:"}}

{{formatCommands .Commands "additionalHelpTopics"}}{{end}}`
	case Aliases:
		return `{{if gt (len .Aliases) 0}}

{{HeadingStyle "Aliases:"}}

  {{.NameAndAliases}}{{end}}`
	case SubCommandAliases:
		return `{{if (isAliasesPresent .Commands)}}

{{HeadingStyle "SubCommand Aliases:"}}

{{formatCommands .Commands "subcommandAliases"}}{{end}}`
	case AvailableCommands:
		return `{{if .HasAvailableSubCommands}}


{{HeadingStyle "Available Commands:"}}

{{formatCommands .Commands "availableCommands"}}{{end}}`
	case Examples:
		return `{{if .HasExample}}


{{HeadingStyle "Examples:"}}

{{.Example}}{{end}}`
	case Flags:
		return `{{if .HasAvailableLocalFlags}}


{{HeadingStyle "Flags:"}}

{{wrappedFlagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}`
	case GlobalFlags:
		return `{{if .HasAvailableInheritedFlags}}


{{HeadingStyle "Global Flags:"}}

{{wrappedFlagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}`
	case NativeCommands:
		return `{{if (isNativeCommandsAvailable .Commands)}}

{{HeadingStyle "Native "}}{{HeadingStyle .Use}}{{HeadingStyle " Commands:"}}

{{formatCommands .Commands "native"}}{{end}}`
	case Usage:
		return `
{{HeadingStyle "Usage:"}}
{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [sub-command] [flags]{{end}}`
	case DoubleDashHelp:
		return `

The '--' (double-dash) can be used to signify the end of Atmos-specific options 
and the beginning of additional native arguments and flags for the specific command being run.

Example:
  atmos {{.CommandPath}} {{if gt (len .Commands) 0}}[subcommand]{{end}} <component> -s <stack> -- <native-flags>`
	case Footer:
		return `{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} {{if gt (len .Commands) 0}}[subcommand]{{end}} --help" for more information about a command.{{end}}`
	default:
		return ""
	}
}
