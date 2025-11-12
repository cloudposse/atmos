package pager

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/data"
	log "github.com/cloudposse/atmos/pkg/logger"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

// Writer is an interface for writing content to the data stream.
// This allows for dependency injection and testing without relying on the global data package state.
type Writer interface {
	Write(content string) error
}

// dataWriter is the default implementation that uses the data package.
type dataWriter struct{}

// Write writes content using the data package.
func (d *dataWriter) Write(content string) error {
	return data.Write(content)
}

type PageCreator interface {
	Run(title, content string) error
}

type pageCreator struct {
	enablePager           bool
	writer                Writer
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
		writer:                &dataWriter{},
		newTeaProgram:         tea.NewProgram,
		contentFitsTerminal:   ContentFitsTerminal,
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
	}
}

func (p *pageCreator) Run(title, content string) error {
	if !(p.enablePager) || !p.isTTYSupportForStdout() {
		return p.writeContent(content)
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
			tea.WithAltScreen(), // use the full size of the terminal in its "alternate screen buffer"
		).Run(); err != nil {
			return err
		}
	} else {
		return p.writeContent(content)
	}
	return nil
}

// writeContent writes content to the configured writer.
// If the writer is nil, it falls back to fmt.Print and logs a warning.
// Returns any error from the writer.
func (p *pageCreator) writeContent(content string) error {
	if p.writer == nil {
		// Fallback for nil writer (shouldn't happen in production, but safe for tests).
		log.Warn("pager writer is nil, falling back to fmt.Print")
		fmt.Print(content)
		return nil
	}

	// Write content and return any error.
	if err := p.writer.Write(content); err != nil {
		return fmt.Errorf("failed to write content: %w", err)
	}

	return nil
}
