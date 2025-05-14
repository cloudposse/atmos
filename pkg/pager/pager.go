package pager

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type PageCreator interface {
	Run(title, content string) error
}

type pageCreator struct {
	newTeaProgram func(model tea.Model, opts ...tea.ProgramOption) *tea.Program
}

func New() PageCreator {
	return &pageCreator{
		newTeaProgram: tea.NewProgram,
	}
}

func (p *pageCreator) Run(title, content string) error {
	if _, err := p.newTeaProgram(
		&model{
			title:    title,
			content:  content,
			ready:    false,
			viewport: viewport.New(0, 0),
		},
		tea.WithAltScreen(),       // use the full size of the terminal in its "alternate screen buffer"
		tea.WithMouseCellMotion(), // turn on mouse support so we can track the mouse wheel
	).Run(); err != nil {
		return err
	}
	return nil
}
