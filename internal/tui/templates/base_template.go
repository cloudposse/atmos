package templates

type HelpTemplateSections int

const (
	LongDescription HelpTemplateSections = iota
	Usage
	Aliases
	SubCommandAliases
	Examples
	AvailableCommands
	CustomCommands
	Flags
	GlobalFlags
	AdditionalHelpTopics
	NativeCommands
	Footer
)

const formatCommandsTemplateEnd = `"}}{{end}}`

func GenerateFromBaseTemplate(parts []HelpTemplateSections) string {
	template := ""
	for _, value := range parts {
		template += getSection(value)
	}
	return template + "\n"
}

func formatCommandsTemplate(listType string) string {
	return `{{formatCommands .Commands "` + listType + formatCommandsTemplateEnd
}

func getSection(section HelpTemplateSections) string {
	switch section {
	case LongDescription:
		return `{{renderMarkdown .Long }}
`
	case AdditionalHelpTopics:
		return `{{if .HasHelpSubCommands}}


{{HeadingStyle "Additional help topics:"}}

` + formatCommandsTemplate(commandListTypeAdditionalHelpTopics)
	case Aliases:
		return `{{if gt (len .Aliases) 0}}

{{HeadingStyle "Aliases:"}}

  {{.NameAndAliases}}{{end}}`
	case SubCommandAliases:
		return `{{if (isAliasesPresent .Commands)}}

{{HeadingStyle "Subcommand Aliases:"}}

` + formatCommandsTemplate(commandListTypeSubcommandAliases)
	case AvailableCommands:
		return `{{if (hasCommands .Commands "` + commandListTypeBuiltInCommands + `")}}


{{HeadingStyle "BUILT-IN COMMANDS"}}

` + formatCommandsTemplate(commandListTypeBuiltInCommands)
	case CustomCommands:
		return `{{if (hasCommands .Commands "` + commandListTypeCustomCommands + `")}}


{{HeadingStyle "CUSTOM COMMANDS"}}

` + formatCommandsTemplate(commandListTypeCustomCommands)
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

` + formatCommandsTemplate(commandListTypeNative)
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
