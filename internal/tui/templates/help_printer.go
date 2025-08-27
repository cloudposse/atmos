package templates

import (
	"fmt"
	"io"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
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
	render, err := markdown.NewTerminalMarkdownRenderer(&schema.AtmosConfiguration{})
	if err != nil {
		return
	}

	flagName := ""

	if flag.Shorthand != "" {
		if flag.Value.Type() != "bool" {
			flagName = fmt.Sprintf("%s-%s, --%s %s", strings.Repeat(" ", nameIndent),
				flag.Shorthand, flag.Name, flag.Value.Type())
		} else {
			flagName = fmt.Sprintf("%s-%s, --%s", strings.Repeat(" ", nameIndent),
				flag.Shorthand, flag.Name)
		}
	} else {
		if flag.Value.Type() != "bool" {
			flagName = fmt.Sprintf("%s    --%s %s", strings.Repeat(" ", nameIndent),
				flag.Name, flag.Value.Type())
		} else {
			flagName = fmt.Sprintf("%s    --%s", strings.Repeat(" ", nameIndent),
				flag.Name)
		}
	}

	availWidth := int(p.wrapLimit) - p.maxFlagLen - 4
	if availWidth < minDescWidth {
		if _, err := fmt.Fprintf(p.out, "%s\n", flagName); err != nil {
			return
		}
		flagName = strings.Repeat(" ", p.maxFlagLen)
		availWidth = int(p.wrapLimit) - 4
	}

	flagSection := fmt.Sprintf("%-*s", p.maxFlagLen, flagName)
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
	if len(lines) > 0 {
		lines = lines[1:]
	}

	if _, err := fmt.Fprintf(p.out, "%-*s%s\n", descIndent, flagSection, lines[0]); err != nil {
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
