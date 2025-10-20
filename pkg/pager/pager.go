package pager

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

//go:generate go run go.uber.org/mock/mockgen@latest -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
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
	if !(p.enablePager) || !p.isTTYSupportForStdout() {
		fmt.Print(content)
		return nil
	}
	// Count visible lines (taking word wrapping into account)
	contentFits := p.contentFitsTerminal(content)
	// If content exceeds terminal height, use pager
	if !contentFits {
		if _, err := p.newTeaProgram(
			&model{
				title:    title,
				content:  content,
				ready:    false,
				viewport: viewport.New(0, 0),
			},
			tea.WithAltScreen(), // use the full size of the terminal in its "alternate screen buffer"
		).Run(); err != nil {
			return err
		}
	} else {
		fmt.Print(content)
	}
	return nil
}
