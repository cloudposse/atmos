package pager

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	log "github.com/cloudposse/atmos/pkg/logger"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type PageCreator interface {
	Run(title, content string) error
}

type pageCreator struct {
	enablePager           bool
	newTeaProgram         func(model tea.Model, opts ...tea.ProgramOption) *tea.Program
	contentFitsTerminal   func(content string) bool
	isTTYSupportForStdout func() bool
	isTTYAccessible       func() bool
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
		isTTYAccessible:       isTTYAccessible,
	}
}

// isTTYAccessible checks if /dev/tty can be opened.
func isTTYAccessible() bool {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	tty.Close()
	return true
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
	if !p.isTTYAccessible() {
		// /dev/tty not accessible, print directly without pager.
		// This is an expected condition in non-interactive environments, not an error.
		log.Trace("Pager disabled: /dev/tty not accessible")
		fmt.Print(content)
		return nil
	}

	// Count visible lines (taking word wrapping into account)
	contentFits := p.contentFitsTerminal(content)

	// If content fits in terminal, print it directly without pager.
	if contentFits {
		fmt.Print(content)
		return nil
	}

	// Content doesn't fit - use the pager with alternate screen.
	_, pagerErr := p.newTeaProgram(
		&model{
			title:    title,
			content:  content,
			ready:    false,
			viewport: viewport.New(0, 0),
		},
		tea.WithAltScreen(), // use the full size of the terminal in its "alternate screen buffer".
	).Run()
	if pagerErr != nil {
		// Pager failed, fall back to direct print.
		// This is a graceful fallback, not a critical error.
		log.Trace("Pager failed, falling back to direct print", "error", pagerErr)
		fmt.Print(content)
		return nil
	}

	return nil
}
