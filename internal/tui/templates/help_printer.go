package templates

import (
	"fmt"
	"io"
	"strings"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/pflag"
)

const (
	defaultOffset = 10
	minWidth      = 80
	flagIndent    = "    "
)

type HelpFlagPrinter struct {
	wrapLimit uint
	out       io.Writer
}

func NewHelpFlagPrinter(out io.Writer, wrapLimit uint) *HelpFlagPrinter {
	if wrapLimit < minWidth {
		wrapLimit = minWidth
	}
	return &HelpFlagPrinter{
		wrapLimit: wrapLimit,
		out:       term.NewResponsiveWriter(out),
	}
}

func (p *HelpFlagPrinter) PrintHelpFlag(flag *pflag.Flag) {
	var prefix strings.Builder
	prefix.WriteString(flagIndent)

	if len(flag.Shorthand) > 0 {
		prefix.WriteString(fmt.Sprintf("-%s, --%s", flag.Shorthand, flag.Name))
	} else {
		prefix.WriteString(fmt.Sprintf("    --%s", flag.Name))
	}

	if flag.Value.Type() != "bool" {
		prefix.WriteString(fmt.Sprintf(" %s", flag.Value.Type()))
	}

	prefixLen := len(prefix.String())
	descWidth := int(p.wrapLimit) - prefixLen - len(flagIndent) - 4

	description := flag.Usage
	if flag.DefValue != "" && flag.DefValue != "false" {
		description += fmt.Sprintf(" (default %q)", flag.DefValue)
	}

	wrapped := wordwrap.WrapString(description, uint(descWidth))
	lines := strings.Split(wrapped, "\n")

	fmt.Fprintf(p.out, "%s%s%s\n",
		prefix.String(),
		strings.Repeat(" ", descWidth-len(lines[0])),
		lines[0])

	if len(lines) > 1 {
		indent := strings.Repeat(" ", prefixLen+2)
		for _, line := range lines[1:] {
			fmt.Fprintf(p.out, "%s%s\n", indent, line)
		}
	}

	fmt.Fprintln(p.out)
}
