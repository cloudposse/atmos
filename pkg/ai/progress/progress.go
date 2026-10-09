// Package progress shows what an AI request is doing.
//
// It wraps the shared spinner so that provider tool events ("Running atmos list
// stacks…") update the spinner text, and so the spinner steps aside while an
// approval prompt owns the terminal.
package progress

import (
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/ai/approval"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// Indicator is the subset of the spinner used by Progress. It is satisfied by *spinner.Spinner.
type Indicator interface {
	Start()
	Stop()
	Update(message string)
	SetInterruptHandler(handler func())
}

// Progress drives a spinner for the lifetime of one AI request.
// All methods are safe for concurrent use.
type Progress struct {
	mu          sync.Mutex
	indicator   Indicator
	message     string // Base message shown while the AI is thinking.
	current     string // Message to show whenever the spinner (re)starts.
	interactive bool   // True when the spinner can animate (stdout is a terminal).
	wanted      bool   // Between Start and Stop: the spinner should be visible unless paused.
	running     bool   // The indicator is currently started.
}

// New creates Progress that shows message while the AI is thinking.
func New(message string) *Progress {
	defer perf.Track(nil, "progress.New")()

	return newProgress(spinner.New(message), message, term.IsTTYSupportForStdout())
}

func newProgress(indicator Indicator, message string, interactive bool) *Progress {
	return &Progress{
		indicator:   indicator,
		message:     message,
		current:     message,
		interactive: interactive,
	}
}

// OnInterrupt registers a function that runs when the user presses ctrl+c while the spinner
// is showing. Call it before Start. Pausing for a prompt is not an interrupt.
func (p *Progress) OnInterrupt(handler func()) {
	defer perf.Track(nil, "progress.Progress.OnInterrupt")()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.indicator.SetInterruptHandler(handler)
}

// Start shows the spinner. In output that cannot animate it emits the message once as a status line.
func (p *Progress) Start() {
	defer perf.Track(nil, "progress.Progress.Start")()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.wanted {
		return
	}
	p.wanted = true
	p.startLocked()
	if !p.interactive {
		p.indicator.Update(p.message)
	}
}

// Stop removes the spinner. It is idempotent.
func (p *Progress) Stop() {
	defer perf.Track(nil, "progress.Progress.Stop")()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.wanted = false
	p.stopLocked()
}

// Pause hides the spinner so a prompt can use the terminal. It does nothing when the spinner is not showing.
func (p *Progress) Pause() {
	defer perf.Track(nil, "progress.Progress.Pause")()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopLocked()
}

// Resume shows the spinner again after Pause. It does nothing after Stop.
func (p *Progress) Resume() {
	defer perf.Track(nil, "progress.Progress.Resume")()

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.wanted || p.running {
		return
	}
	p.startLocked()
	if p.interactive && p.current != p.message {
		p.indicator.Update(p.current)
	}
}

// PromptHooks returns functions to call immediately before and after a prompt is shown.
func (p *Progress) PromptHooks() (before, after func()) {
	defer perf.Track(nil, "progress.Progress.PromptHooks")()

	return p.Pause, p.Resume
}

// Handle reflects a provider progress event in the spinner text.
func (p *Progress) Handle(event approval.Event) {
	defer perf.Track(nil, "progress.Progress.Handle")()

	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.Kind {
	case approval.ToolStart:
		label := event.Summary
		if label == "" {
			label = event.Tool
		}
		p.current = fmt.Sprintf("Running %s…", label)
	case approval.ToolDone:
		p.current = p.message
		// Outside a terminal every update is a new status line; only the start of a tool is worth one.
		if !p.interactive {
			return
		}
	default:
		return
	}

	if p.running {
		p.indicator.Update(p.current)
	}
}

// Attach subscribes Progress to a provider's events when the provider reports them.
// It reports whether the provider does.
func (p *Progress) Attach(target any) bool {
	defer perf.Track(nil, "progress.Progress.Attach")()

	reporter, ok := target.(approval.ProgressReporter)
	if !ok {
		return false
	}
	reporter.SetProgressHandler(p.Handle)
	return true
}

func (p *Progress) startLocked() {
	if p.running {
		return
	}
	p.indicator.Start()
	p.running = true
}

func (p *Progress) stopLocked() {
	if !p.running {
		return
	}
	p.indicator.Stop()
	p.running = false
}
