package pager

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type PageCreator interface {
	Run(title, content string) error
}

type pageCreator struct {
	enablePager           bool
	pagerFlagExplicit     bool // whether --pager flag was explicitly set.
	newTeaProgram         func(model tea.Model, opts ...tea.ProgramOption) *tea.Program
	contentFitsTerminal   func(content string) bool
	isTTYSupportForStdout func() bool
}

func NewWithAtmosConfig(enablePager bool) PageCreator {
	return NewWithAtmosConfigAndFlag(enablePager, false)
}

// NewWithAtmosConfigAndFlag creates a page creator with pager settings and tracks if the flag was explicit.
func NewWithAtmosConfigAndFlag(enablePager bool, pagerFlagExplicit bool) PageCreator {
	pager := New()
	pager.(*pageCreator).enablePager = enablePager
	pager.(*pageCreator).pagerFlagExplicit = pagerFlagExplicit
	return pager
}

func New() PageCreator {
	return &pageCreator{
		enablePager:           false,
		pagerFlagExplicit:     false,
		newTeaProgram:         tea.NewProgram,
		contentFitsTerminal:   ContentFitsTerminal,
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
	}
}

func (p *pageCreator) Run(title, content string) error {
	// Check if pager is enabled.
	if !p.enablePager {
		fmt.Print(content)
		return nil
	}

	// Check if we have TTY support.
	if !p.isTTYSupportForStdout() {
		// Log debug message only if --pager flag was explicitly set.
		if p.pagerFlagExplicit {
			log.Debug("Pager disabled: no TTY detected. Output will not be paginated.")
		}
		fmt.Print(content)
		return nil
	}

	// Count visible lines (taking word wrapping into account).
	contentFits := p.contentFitsTerminal(content)
	// If content exceeds terminal height, use pager.
	if !contentFits {
		if _, err := p.newTeaProgram(
			&model{
				title:    title,
				content:  content,
				ready:    false,
				viewport: viewport.New(0, 0),
			},
			tea.WithAltScreen(), // use the full size of the terminal in its "alternate screen buffer".
		).Run(); err != nil {
			return err
		}
	} else {
		fmt.Print(content)
	}
	return nil
}
