package templates

import "fmt"

//

// MainUsageTemplate returns the usage template for the root command and wrap cobra flag usages to the terminal width
// func MainUsageTemplate() string {
// 	return `
// {{ .Long }}
// Usage:{{if .Runnable}}
//   {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
//   {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

// Aliases:
//   {{.NameAndAliases}}{{end}}{{if .HasExample}}

// Examples:
// {{.Example}}{{end}}{{if .HasAvailableSubCommands}}

// Available Commands:
// {{formatCommands .Commands false}}{{end}}{{if .HasAvailableLocalFlags}}

// Flags:
// {{wrappedFlagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

// Global Flags:
// {{wrappedFlagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

// Additional help topics:
// {{formatCommands .Commands true}}{{end}}{{if .HasAvailableSubCommands}}

// Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
// `
// }

// MainUsageTemplate returns the usage template for the root command and wrap cobra flag usages to the terminal width
func MainUsageTemplate(isLongRequired bool) string {
	template := `
Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}
  
 
{{if gt (len .Aliases) 0}}
Aliases:
  {{.NameAndAliases}}
{{end}}

{{if .HasExample}}
Examples:
{{.Example}}{{end}}

{{if .HasAvailableSubCommands}}
Available Commands:
{{formatCommands .Commands false}}{{end}}

{{if .HasAvailableLocalFlags}}
Flags:
{{wrappedFlagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}


{{if .HasAvailableInheritedFlags}}
Global Flags:
{{wrappedFlagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}

{{if .HasHelpSubCommands}}
Additional help topics:
{{formatCommands .Commands true}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
	if isLongRequired {
		template = `
{{ .Long }}
` + template
	}
	return template
}

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
	case Footer:
		return `{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}`
	default:
		return ""
	}
}
