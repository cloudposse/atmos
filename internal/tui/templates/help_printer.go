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
	return maxLen + 4
}

func (p *HelpFlagPrinter) PrintHelpFlag(flag *pflag.Flag) {
	nameIndent := 4
	typeIndent := 4

	flagName := ""
	if flag.Shorthand != "" {
		flagName = fmt.Sprintf("%s-%s, --%s", strings.Repeat(" ", nameIndent), flag.Shorthand, flag.Name)
	} else {
		flagName = fmt.Sprintf("%s    --%s", strings.Repeat(" ", nameIndent), flag.Name)
	}

	typeStr := ""
	if flag.Value.Type() != "bool" {
		typeStr = fmt.Sprintf(" %s", flag.Value.Type())
	}

	flagSection := fmt.Sprintf("%-*s%-*s", p.maxFlagLen, flagName, typeIndent, typeStr)

	descIndent := p.maxFlagLen + len(typeStr) + typeIndent + 4

	description := flag.Usage
	if flag.DefValue != "" {
		description = fmt.Sprintf("%s (default %q)", description, flag.DefValue)
	}

	descWidth := int(p.wrapLimit) - descIndent

	wrapped := wordwrap.WrapString(description, uint(descWidth))
	lines := strings.Split(wrapped, "\n")

	fmt.Fprintf(p.out, "%-*s%s\n", descIndent, flagSection, lines[0])

	// Print remaining lines with proper indentation
	for _, line := range lines[1:] {
		fmt.Fprintf(p.out, "%s%s\n", strings.Repeat(" ", descIndent), line)
	}

	fmt.Fprintln(p.out)
}
