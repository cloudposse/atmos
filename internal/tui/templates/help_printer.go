package templates

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/pflag"
)

const (
	defaultOffset   = 10
	minWidth        = 80
	flagIndent      = "    "
	nameIndentWidth = 4
	minDescWidth    = 20
)

type HelpFlagPrinter struct {
	wrapLimit  uint
	out        io.Writer
	maxFlagLen int
}

func NewHelpFlagPrinter(out io.Writer, wrapLimit uint, flags *pflag.FlagSet) (*HelpFlagPrinter, error) {
	if out == nil {
		return nil, fmt.Errorf("invalid argument: output writer cannot be nil")
	}
	if flags == nil {
		return nil, fmt.Errorf("invalid argument: flag set cannot be nil")
	}

	if wrapLimit < minWidth {
		wrapLimit = minWidth
	}

	return &HelpFlagPrinter{
		wrapLimit:  wrapLimit,
		out:        out,
		maxFlagLen: calculateMaxFlagLength(flags),
	}, nil
}

func calculateMaxFlagLength(flags *pflag.FlagSet) int {
	maxLen := 0
	flags.VisitAll(func(flag *pflag.Flag) {
		length := len(flagIndent)

		if len(flag.Shorthand) > 0 {
			if flag.Value.Type() != "bool" {
				length += len(fmt.Sprintf("-%s, --%s %s", flag.Shorthand, flag.Name, flag.Value.Type()))
			} else {
				length += len(fmt.Sprintf("-%s, --%s", flag.Shorthand, flag.Name))
			}
		} else {
			if flag.Value.Type() != "bool" {
				length += len(fmt.Sprintf("    --%s %s", flag.Name, flag.Value.Type()))
			} else {
				length += len(fmt.Sprintf("    --%s", flag.Name))
			}
		}

		if length > maxLen {
			maxLen = length
		}
	})
	return maxLen
}

func (p *HelpFlagPrinter) PrintHelpFlag(flag *pflag.Flag) {
	nameIndent := nameIndentWidth
	render, err := markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	if err != nil {
		return
	}

	// Get theme styles
	styles := theme.GetCurrentStyles()

	// Build flag parts separately for styling
	indent := strings.Repeat(" ", nameIndent)
	flagPart := ""
	typePart := ""

	if flag.Shorthand != "" {
		if flag.Value.Type() != "bool" {
			flagPart = fmt.Sprintf("-%s, --%s", flag.Shorthand, flag.Name)
			typePart = flag.Value.Type()
		} else {
			flagPart = fmt.Sprintf("-%s, --%s", flag.Shorthand, flag.Name)
		}
	} else {
		if flag.Value.Type() != "bool" {
			flagPart = fmt.Sprintf("    --%s", flag.Name)
			typePart = flag.Value.Type()
		} else {
			flagPart = fmt.Sprintf("    --%s", flag.Name)
		}
	}

	// Build the styled flag components
	var styledFlagPart, styledTypePart string
	if styles != nil {
		styledFlagPart = styles.Help.FlagName.Render(flagPart)
		if typePart != "" {
			styledTypePart = styles.Help.FlagDataType.Render(typePart)
		}
	} else {
		styledFlagPart = flagPart
		styledTypePart = typePart
	}

	// Calculate visual width (ignoring ANSI codes)
	flagWidth := len(indent) + lipgloss.Width(styledFlagPart)
	if typePart != "" {
		flagWidth += 1 + lipgloss.Width(styledTypePart) // +1 for space between flag and type
	}

	// Calculate padding needed to reach maxFlagLen
	padding := p.maxFlagLen - flagWidth
	if padding < 0 {
		padding = 0
	}

	// Build the complete flag section with proper spacing
	var flagSection string
	if typePart != "" {
		flagSection = fmt.Sprintf("%s%s %s%s", indent, styledFlagPart, styledTypePart, strings.Repeat(" ", padding))
	} else {
		flagSection = fmt.Sprintf("%s%s%s", indent, styledFlagPart, strings.Repeat(" ", padding))
	}

	// Handle case where flag is too long for single line
	availWidth := int(p.wrapLimit) - p.maxFlagLen - 4
	if availWidth < minDescWidth {
		if _, err := fmt.Fprintf(p.out, "%s\n", flagSection); err != nil {
			return
		}
		flagSection = strings.Repeat(" ", p.maxFlagLen)
		availWidth = int(p.wrapLimit) - 4
	}

	descIndent := p.maxFlagLen + 4

	description := flag.Usage
	// if Name is empty it is our double dash.
	if flag.DefValue != "" && flag.Name != "" && flag.Name != "help" {
		description = fmt.Sprintf("%s (default `%s`)", description, flag.DefValue)
	}

	wrapped := wordwrap.WrapString(description, uint(availWidth))
	wrapped, err = render.RenderWithoutWordWrap(wrapped)
	if err != nil {
		return
	}
	wrapped = strings.TrimSuffix(wrapped, "\n\n")
	lines := strings.Split(wrapped, "\n")

	// Skip empty first line if present (from markdown rendering)
	if len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}

	// Print first line with flag
	if len(lines) > 0 {
		if _, err := fmt.Fprintf(p.out, "%s    %s\n", flagSection, lines[0]); err != nil {
			return
		}
	} else {
		// No description, just print the flag
		if _, err := fmt.Fprintf(p.out, "%s\n", flagSection); err != nil {
			return
		}
		return
	}

	// Print remaining lines with proper indentation
	for _, line := range lines[1:] {
		if _, err := fmt.Fprintf(p.out, "%s%s\n", strings.Repeat(" ", descIndent), line); err != nil {
			return
		}
	}

	if _, err := fmt.Fprintln(p.out); err != nil {
		return
	}
}
