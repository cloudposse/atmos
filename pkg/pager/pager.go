package pager

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type PageCreator interface {
	Run(title, content string) error
}

type pageCreator struct {
	enablePager           bool
	newTeaProgram         func(model tea.Model, opts ...tea.ProgramOption) *tea.Program
	contentFitsTerminal   func(content string) bool
	isTTYSupportForStdout func() bool
}

func NewWithAtmosConfig(enablePager bool) PageCreator {
	pager := New()
	pager.(*pageCreator).enablePager = enablePager
	return pager
}

func New() PageCreator {
	return &pageCreator{
		enablePager:           false,
		newTeaProgram:         tea.NewProgram,
		contentFitsTerminal:   ContentFitsTerminal,
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
	}
}

func (p *pageCreator) Run(title, content string) error {
	// Always print content directly if pager is disabled or no TTY support.
	if !(p.enablePager) || !p.isTTYSupportForStdout() {
		fmt.Print(content)
		return nil
	}

	// Check if /dev/tty is accessible before trying to use alternate screen.
	// The alternate screen mode requires opening /dev/tty for input, which may not
	// be available in CI/test environments even if stdout is a TTY.
	if tty, err := os.Open("/dev/tty"); err != nil {
		// /dev/tty not accessible, print directly without pager.
		fmt.Print(content)
		return nil
	} else {
		// Close the file descriptor immediately to avoid leaking.
		tty.Close()
	}

	// Count visible lines (taking word wrapping into account)
	contentFits := p.contentFitsTerminal(content)

	// If content fits in terminal, print it directly without pager.
	if contentFits {
		fmt.Print(content)
		return nil
	}

	// Content doesn't fit - use the pager with alternate screen.
	_, err := p.newTeaProgram(
		&model{
			title:    title,
			content:  content,
			ready:    false,
			viewport: viewport.New(0, 0),
		},
		tea.WithAltScreen(), // use the full size of the terminal in its "alternate screen buffer"
	).Run()
	if err != nil {
		// Pager failed, fall back to direct print.
		fmt.Print(content)
		return nil
	}

	return nil
}
