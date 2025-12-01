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
	Footer
)

func GenerateFromBaseTemplate(parts []HelpTemplateSections) string {
	template := ""
	for _, value := range parts {
		template += getSection(value)
	}
	return template + "\n"
}

func getSection(section HelpTemplateSections) string {
	switch section {
	case LongDescription:
		return `{{renderMarkdown .Long }}
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

{{HeadingStyle "Subcommand Aliases:"}}

{{formatCommands .Commands "subcommandAliases"}}{{end}}`
	case AvailableCommands:
		return `{{if .HasAvailableSubCommands}}


{{HeadingStyle "Available Commands:"}}

{{formatCommands .Commands "availableCommands"}}{{end}}`
	case Examples:
		return `{{if .HasExample}}


{{HeadingStyle "Examples:"}}
{{renderMarkdown .Example}}{{end}}`
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
	case Footer:
		return `

{{renderHelpMarkdown .}}`
	default:
		return ""
	}
}
