package pager

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

// maskContent applies the native io package masking to content.
// This is used as a fallback when the data package is not available.
// It leverages the centralized masking infrastructure from pkg/io.
// Returns empty string if masking is unavailable to prevent data leaks.
func maskContent(content string) string {
	// Try to get the global I/O context for its masker.
	ioCtx := iolib.GetContext()
	if ioCtx == nil {
		// If context is not initialized, refuse to return unmasked content.
		// This prevents accidental data leaks when masking is unavailable.
		log.Error("io context not initialized, refusing to return unmasked content")
		return ""
	}

	// Use the native masker.
	return ioCtx.Masker().Mask(content)
}

// Writer is an interface for writing content to the data stream.
// This allows for dependency injection and testing without relying on the global data package state.
type Writer interface {
	Write(content string) error
}

// dataWriter is the default implementation that uses the data package.
// Falls back to fmt.Print if data package isn't initialized.
type dataWriter struct{}

// Write writes content using the data package.
// If data package isn't initialized (panics), falls back to fmt.Print with masking.
func (d *dataWriter) Write(content string) (err error) {
	// Use recover to catch panic from data.Write() when not initialized.
	defer func() {
		if r := recover(); r != nil {
			// Data package not initialized, use fallback with native masking.
			log.Debug("data package not initialized, using fmt.Print fallback")
			maskedContent := maskContent(content)
			fmt.Print(maskedContent)
			err = nil
		}
	}()

	// Try to use data.Write() - will panic if not initialized.
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
		writer:                &dataWriter{},
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
		return p.writeContent(content)
	}

	// Check if /dev/tty is accessible before trying to use alternate screen.
	// The alternate screen mode requires opening /dev/tty for input, which may not
	// be available in CI/test environments even if stdout is a TTY.
	if !p.isTTYAccessible() {
		// /dev/tty not accessible, print directly without pager.
		// This is an expected condition in non-interactive environments, not an error.
		log.Trace("Pager disabled: /dev/tty not accessible")
		return p.writeContent(content)
	}

	// Count visible lines (taking word wrapping into account)
	contentFits := p.contentFitsTerminal(content)

	// If content fits in terminal, print it directly without pager.
	if contentFits {
		return p.writeContent(content)
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
		return p.writeContent(content)
	}

	return nil
}

// writeContent writes content to the configured writer.
// If the writer is nil, it falls back to fmt.Print with masking and logs a warning.
// Returns any error from the writer.
func (p *pageCreator) writeContent(content string) error {
	if p.writer == nil {
		// Fallback for nil writer (shouldn't happen in production, but safe for tests).
		log.Warn("pager writer is nil, falling back to fmt.Print")
		maskedContent := maskContent(content)
		fmt.Print(maskedContent)
		return nil
	}

	// Write content and return any error.
	if err := p.writer.Write(content); err != nil {
		return fmt.Errorf("failed to write content: %w", err)
	}

	return nil
}
