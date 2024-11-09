package templates

import (
	"fmt"
	"io"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/pflag"
)

const (
	defaultOffset = 10
	minWidth      = 80
	flagIndent    = "    "
)

type HelpFlagPrinter struct {
	wrapLimit  uint
	out        io.Writer
	maxFlagLen int
}

func NewHelpFlagPrinter(out io.Writer, wrapLimit uint, flags *pflag.FlagSet) *HelpFlagPrinter {
	return &HelpFlagPrinter{
		wrapLimit:  wrapLimit,
		out:        out,
		maxFlagLen: calculateMaxFlagLength(flags),
	}
}

func calculateMaxFlagLength(flags *pflag.FlagSet) int {
	maxLen := 0
	flags.VisitAll(func(flag *pflag.Flag) {
		length := len(flagIndent)

		if len(flag.Shorthand) > 0 {
			length += len(fmt.Sprintf("-%s, --%s", flag.Shorthand, flag.Name))
		} else {
			length += len(fmt.Sprintf("    --%s", flag.Name))
		}

		if flag.Value.Type() != "bool" {
			length += len(fmt.Sprintf(" %s", flag.Value.Type()))
		}

		if length > maxLen {
			maxLen = length
		}
	})
	return maxLen
}

func (p *HelpFlagPrinter) PrintHelpFlag(flag *pflag.Flag) {
	nameIndent := 4

	// Build the complete flag section (name + type) together
	var flagSection string
	if flag.Shorthand != "" {
		flagSection = fmt.Sprintf("%s-%s, --%s", strings.Repeat(" ", nameIndent), flag.Shorthand, flag.Name)
	} else {
		flagSection = fmt.Sprintf("%s    --%s", strings.Repeat(" ", nameIndent), flag.Name)
	}

	if flag.Value.Type() != "bool" {
		flagSection += fmt.Sprintf(" %s", flag.Value.Type())
	}

	padding := p.maxFlagLen - len(flagSection)
	if padding > 0 {
		flagSection += strings.Repeat(" ", padding)
	}

	descIndent := p.maxFlagLen + 2
	descWidth := int(p.wrapLimit) - descIndent

	description := flag.Usage
	if flag.DefValue != "" {
		description = fmt.Sprintf("%s (default %q)", description, flag.DefValue)
	}

	wrapped := wordwrap.WrapString(description, uint(descWidth))
	lines := strings.Split(wrapped, "\n")

	fmt.Fprintf(p.out, "%s  %s\n", flagSection, lines[0])

	// Print remaining lines with proper indentation
	for _, line := range lines[1:] {
		fmt.Fprintf(p.out, "%s%s\n", strings.Repeat(" ", descIndent), line)
	}

	fmt.Fprintln(p.out)
}
