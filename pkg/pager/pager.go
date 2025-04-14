package pager

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type pageCreator struct {
	title   string
	content string
	ready   bool
}

func New(title, content string) *pageCreator {
	return &pageCreator{
		title:   title,
		content: content,
		ready:   false,
	}
}

func (p *pageCreator) Run() error {
	if _, err := tea.NewProgram(
		&model{
			title:    p.title,
			content:  p.content,
			ready:    p.ready,
			viewport: viewport.New(0, 0),
		},
		tea.WithAltScreen(),       // use the full size of the terminal in its "alternate screen buffer"
		tea.WithMouseCellMotion(), // turn on mouse support so we can track the mouse wheel
	).Run(); err != nil {
		return err
	}
	return nil
}
