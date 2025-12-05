package templates

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	termUtils "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// Templater handles the generation and management of command usage templates.
type Templater struct {
	UsageTemplate string
}

// commandStyle defines the styles for command formatting.
var (
	commandNameStyle = theme.Styles.CommandName
	commandDescStyle = theme.Styles.Description

	commandUnsupportedNameStyle = theme.Styles.CommandName.
					Foreground(lipgloss.Color(theme.ColorGray)).
					Bold(true)
	commandUnsupportedDescStyle = theme.Styles.Description.
					Foreground(lipgloss.Color(theme.ColorGray))
)

// formatCommand returns a styled string for a command and its description.
func formatCommand(name string, desc string, padding int, IsNotSupported bool) string {
	paddedName := fmt.Sprintf("%-*s", padding, name)
	if IsNotSupported {
		styledName := commandUnsupportedNameStyle.Render(paddedName)
		styledDesc := commandUnsupportedDescStyle.Render(desc + " [unsupported]")
		return fmt.Sprintf("  %-30s %s", styledName, styledDesc)
	}
	styledName := commandNameStyle.Render(paddedName)
	styledDesc := commandDescStyle.Render(desc)
	return fmt.Sprintf("  %-30s %s", styledName, styledDesc)
}

var customHelpShortMessage = map[string]string{
	"help": "Display help information for Atmos commands",
}

// filterCommands returns only commands or aliases based on returnOnlyAliases boolean.
func filterCommands(commands []*cobra.Command, returnOnlyAliases bool) []*cobra.Command {
	if !returnOnlyAliases {
		return commands
	}
	filtered := []*cobra.Command{}
	cmdMap := make(map[string]struct{})
	for _, cmd := range commands {
		cmdMap[cmd.Use] = struct{}{}
	}
	for _, cmd := range commands {
		for _, alias := range cmd.Aliases {
			if _, ok := cmdMap[alias]; ok {
				continue
			}
			copyCmd := *cmd
			copyCmd.Use = alias
			copyCmd.Short = fmt.Sprintf("Alias of %q command", cmd.CommandPath())
			filtered = append(filtered, &copyCmd)
		}
	}
	return filtered
}

func renderHelpMarkdown(cmd *cobra.Command) string {
	render, err := markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	if err != nil {
		return ""
	}
	commandPath := cmd.CommandPath()
	if cmd.HasSubCommands() {
		commandPath += " [subcommand]"
	}
	help := fmt.Sprintf("Use `%s --help` for more information about a command.", commandPath)
	var data string
	if termUtils.IsTTYSupportForStdout() {
		data, err = render.Render(help)
	} else {
		data, err = render.RenderAscii(help)
	}
	if err == nil {
		return data
	}
	return ""
}

func isNativeCommandsAvailable(cmds []*cobra.Command) bool {
	for _, cmd := range cmds {
		if cmd.Annotations["nativeCommand"] == "true" {
			return true
		}
	}
	return false
}

func isAliasesPresent(cmds []*cobra.Command) bool {
	return len(filterCommands(cmds, true)) > 0
}

func headingStyle(s string) string {
	if theme.Styles.Help.Headings != nil {
		ch := theme.Styles.Help.Headings
		return ch.Sprint(s)
	}
	return s
}

func renderMarkdown(example string) string {
	render, err := markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	if err != nil {
		return ""
	}

	data, err := render.Render(example)
	if err == nil {
		return data
	}
	return ""
}

// formatCommands formats a slice of cobra commands with proper styling.
func formatCommands(cmds []*cobra.Command, listType string) string {
	var maxLen int
	availableCmds := make([]*cobra.Command, 0)

	// First pass: collect available commands and find max length
	cmds = filterCommands(cmds, listType == "subcommandAliases")
	for _, cmd := range cmds {
		if v, ok := customHelpShortMessage[cmd.Name()]; ok {
			cmd.Short = v
		}
		switch listType {
		case "additionalHelpTopics":
			if cmd.IsAdditionalHelpTopicCommand() {
				availableCmds = append(availableCmds, cmd)
				if len(cmd.Name()) > maxLen {
					maxLen = len(cmd.Name())
				}
				continue
			}
		case "native":
			if cmd.Annotations["nativeCommand"] == "true" {
				availableCmds = append(availableCmds, cmd)
				if len(cmd.Name()) > maxLen {
					maxLen = len(cmd.Name())
				}
				continue
			}
		case "subcommandAliases":
			// if cmd.Annotations["nativeCommand"] == "true" {
			// 	continue
			// }

			if cmd.IsAvailableCommand() || cmd.Name() == "help" {
				availableCmds = append(availableCmds, cmd)
				if len(cmd.Name()) > maxLen {
					maxLen = len(cmd.Name())
				}
			}
		default:
			if cmd.Annotations["nativeCommand"] == "true" {
				continue
			}
			if cmd.IsAvailableCommand() || cmd.Name() == "help" {
				availableCmds = append(availableCmds, cmd)
				if len(cmd.Name()) > maxLen {
					maxLen = len(cmd.Name())
				}
			}
		}
	}

	var lines []string
	// Sorting by whether "IsNotSupported" is present in the Annotations map
	sort.Slice(availableCmds, func(i, j int) bool {
		// Check if "IsNotSupported" is present for commands[i] and commands[j]
		iHasKey := availableCmds[i].Annotations["IsNotSupported"] != "true"
		jHasKey := availableCmds[j].Annotations["IsNotSupported"] != "true"

		// Place commands with "IsNotSupported" at the top
		if iHasKey && !jHasKey {
			return true
		}
		if !iHasKey && jHasKey {
			return false
		}
		// If both or neither have the key, maintain original order
		return i < j
	})
	for _, cmd := range availableCmds {
		lines = append(lines, formatCommand(cmd.Name(), cmd.Short, maxLen, cmd.Annotations["IsNotSupported"] == "true"))
	}

	return strings.Join(lines, "\n")
}

// SetCustomUsageFunc configures a custom usage template for the provided cobra command.
// It returns an error if the command is nil.
func SetCustomUsageFunc(cmd *cobra.Command) error {
	if cmd == nil {
		return errUtils.ErrCommandNil
	}
	t := &Templater{
		UsageTemplate: GenerateFromBaseTemplate([]HelpTemplateSections{
			LongDescription,
			Usage,
			Aliases,
			AvailableCommands,
			NativeCommands,
			SubCommandAliases,
			Flags,
			GlobalFlags,
			AdditionalHelpTopics,
			Examples,
			Footer,
		}),
	}

	cmd.SetUsageTemplate(t.UsageTemplate)
	cobra.AddTemplateFunc("isAliasesPresent", isAliasesPresent)
	cobra.AddTemplateFunc("isNativeCommandsAvailable", isNativeCommandsAvailable)
	cobra.AddTemplateFunc("formatCommands", formatCommands)
	cobra.AddTemplateFunc("renderMarkdown", renderMarkdown)
	cobra.AddTemplateFunc("renderHelpMarkdown", renderHelpMarkdown)
	cobra.AddTemplateFunc("HeadingStyle", headingStyle)

	return nil
}

// GetTerminalWidth returns the width of the terminal, defaulting to 80 if it cannot be determined.
func GetTerminalWidth() int {
	defaultWidth := 80
	screenWidth := defaultWidth

	// Detect terminal width and use it by default if available
	if termUtils.IsTTYSupportForStdout() {
		termWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil && termWidth > 0 {
			screenWidth = termWidth - 2
		}
	}

	return screenWidth
}

// WrappedFlagUsages formats the flag usage string to fit within the terminal width.
func WrappedFlagUsages(f *pflag.FlagSet) string {
	var builder strings.Builder
	width := GetTerminalWidth()
	printer, err := NewHelpFlagPrinter(&builder, uint(width), f)
	if err != nil {
		// If we can't create the printer, return empty string
		// This is unlikely to happen since we're using a strings.Builder
		return ""
	}

	printer.maxFlagLen = calculateMaxFlagLength(f)

	var doubleDashFlag *pflag.Flag
	f.VisitAll(func(flag *pflag.Flag) {
		// We want double dash hint at the last
		if flag.Name == "" {
			doubleDashFlag = flag
			return
		}
		printer.PrintHelpFlag(flag)
	})
	if doubleDashFlag != nil {
		printer.PrintHelpFlag(doubleDashFlag)
	}

	return builder.String()
}
