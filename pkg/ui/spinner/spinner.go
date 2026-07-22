package spinner

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/reflow/truncate"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner/fps"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	newline = "\n"

	// FallbackWidth is used when the real terminal width can't be detected (terminal.Width
	// reports 0, e.g. a PTY that hasn't been given a window size yet). Matches
	// internal/exec/vendor_model.go's fallbackModelWidth.
	fallbackWidth = 120

	// LiveLineMargin reserves this many trailing columns so the live progress line's
	// rendered content never reaches the terminal's true last column, matching
	// internal/exec/vendor_model.go's identically named const.
	liveLineMargin = 1

	// Ellipsis marks a progress message truncated to fit the live progress line.
	ellipsis = "…"
)

// initialWidth returns the real terminal width to use for the live progress line,
// so the first frame can truncate against it before bubbletea's own asynchronous
// tea.WindowSizeMsg (sent by a background goroutine) has had a chance to arrive.
// See internal/exec/vendor_model.go's initialModelWidth for the full rationale:
// a PTY that hasn't been given a window size yet reports 0 to both this direct
// query and bubbletea's own, which would otherwise disable truncation entirely
// and let long lines overflow the real terminal, wrap, and corrupt the
// single-line spinner redraw (each frame appending a new scrollback line
// instead of updating in place).
func initialWidth() int {
	width := terminal.New().Width(terminal.Stderr)
	if width <= 0 {
		return fallbackWidth
	}
	return width
}

// clipToWidth truncates a rendered (possibly ANSI-styled) line to fit within width,
// appending an ellipsis when content is cut, so it never auto-wraps onto a second
// physical row. Bubbletea's inline renderer moves the cursor up by counting '\n' in
// the previous frame; a line that auto-wraps (no embedded '\n') desyncs that count,
// leaving stale wrapped remainders behind as scrollback instead of being overwritten.
// Lipgloss's Style.MaxWidth is deliberately avoided: it can word-wrap by inserting a
// literal newline instead of truncating, which would defeat the single-line guarantee
// this exists to provide (see internal/exec/vendor_model.go's View).
func clipToWidth(line string, width int) string {
	if width <= 0 {
		return line
	}
	effectiveWidth := width - liveLineMargin
	if effectiveWidth <= 0 {
		return line
	}
	return truncate.StringWithTail(line, uint(effectiveWidth), ellipsis)
}

// newDotSpinner builds the shared Dot spinner used by every spinner variant here.
// Apply honors the ATMOS_SPINNER_FPS override (for VHS demo recordings).
func newDotSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	fps.Apply(&s)
	return s
}

// ExecWithSpinner runs an operation with a spinner UI.
// ProgressMsg is shown while operation is running (e.g., "Starting container").
// CompletedMsg is shown when operation completes successfully (e.g., "Started container").
func ExecWithSpinner(progressMsg, completedMsg string, operation func() error) error {
	defer perf.Track(nil, "spinner.ExecWithSpinner")()

	// Check if TTY is available.
	isTTY := term.IsTTYSupportForStdout()

	if !isTTY {
		// No TTY - just run the operation and show result.
		err := operation()
		if err != nil {
			return err
		}
		ui.Success(completedMsg)
		return nil
	}

	// TTY available - use spinner.
	model := newSpinnerModel(progressMsg, completedMsg)

	// Use inline mode - output to stderr, no alternate screen.
	opts := []tea.ProgramOption{
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	}
	if !terminal.HasRealTTYInput() {
		// TTY mode is forced (screenshots, cast recordings): keep the renderer,
		// but don't let bubbletea open /dev/tty for input — there isn't one.
		opts = append(opts, tea.WithInput(nil))
	}
	p := tea.NewProgram(model, opts...)

	// goroutineDone is closed once the operation finishes, guaranteeing that any
	// side effects of the operation have completed before ExecWithSpinner returns.
	goroutineDone := make(chan struct{})
	go func() {
		defer close(goroutineDone)
		err := operation()
		p.Send(opCompleteMsg{err: err})
	}()

	finalModel, spinnerErr := p.Run()

	// Always wait for the operation goroutine to finish before returning.
	<-goroutineDone

	return evaluateSpinnerResult(finalModel, spinnerErr)
}

// evaluateSpinnerResult interprets the outcome of a finished ExecWithSpinner run.
// It returns a spinner-level error, the operation's own error, or
// ErrSpinnerOperationInterrupted if the spinner exited before the operation
// reported completion (e.g. the user pressed ctrl+c).
func evaluateSpinnerResult(finalModel tea.Model, spinnerErr error) error {
	if spinnerErr != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrTUIRun, spinnerErr)
	}
	if finalModel == nil {
		return errUtils.ErrSpinnerReturnedNilModel
	}

	m, ok := finalModel.(spinnerModel)
	if !ok {
		return fmt.Errorf("%w: got %T", errUtils.ErrSpinnerUnexpectedModelType, finalModel)
	}
	if m.err != nil {
		return m.err
	}
	if !m.done {
		return errUtils.ErrSpinnerOperationInterrupted
	}

	return nil
}

// spinnerModel is a simple spinner model for long-running operations.
type spinnerModel struct {
	spinner      spinner.Model
	progressMsg  string // Message shown during operation (e.g., "Starting container").
	completedMsg string // Message shown when done (e.g., "Started container").
	done         bool
	err          error
	width        int // Terminal width; seeded synchronously by initialWidth(), refined by tea.WindowSizeMsg.
}

type opCompleteMsg struct {
	err error
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		// A zero/negative width means the terminal genuinely doesn't have a size yet (e.g. a
		// freshly opened PTY that hasn't been resized): keep the initialWidth() fallback
		// already picked rather than stomping it down to 0, which would disable truncation
		// for the run's whole lifetime. See initialWidth's doc comment.
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case opCompleteMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) View() string {
	if m.done {
		if m.err != nil {
			// Show error indicator with reset line to clear spinner.
			// Use FormatError for proper markdown rendering.
			return terminal.EscResetLine + ui.FormatError(m.progressMsg) + newline
		}
		// Show completed message with checkmark.
		// Use FormatSuccess for proper markdown rendering.
		return terminal.EscResetLine + ui.FormatSuccess(m.completedMsg) + newline
	}
	// Show progress message with spinner.
	// Use FormatInline for proper markdown rendering (e.g., backtick code styling).
	line := fmt.Sprintf("%s %s", m.spinner.View(), ui.FormatInline(m.progressMsg))
	return terminal.EscResetLine + clipToWidth(line, m.width)
}

func newSpinnerModel(progressMsg, completedMsg string) spinnerModel {
	s := newDotSpinner()
	return spinnerModel{
		spinner:      s,
		progressMsg:  progressMsg,
		completedMsg: completedMsg,
		width:        initialWidth(),
	}
}

// ExecWithSpinnerDynamic runs an operation with a spinner UI where the completion message
// is determined dynamically by the operation. The operation returns both the completion
// message and any error. This is useful when the completion message depends on the result
// of the operation (e.g., displaying refs that were compared).
func ExecWithSpinnerDynamic(progressMsg string, operation func() (string, error)) error {
	defer perf.Track(nil, "spinner.ExecWithSpinnerDynamic")()

	// Check if TTY is available.
	isTTY := term.IsTTYSupportForStdout()

	if !isTTY {
		// No TTY - just run the operation and show result.
		completedMsg, err := operation()
		if err != nil {
			return err
		}
		// Show the dynamic completion message if provided, otherwise use progress message.
		if completedMsg != "" {
			ui.Success(completedMsg)
		} else {
			ui.Success(progressMsg)
		}
		return nil
	}

	// TTY available - use spinner with dynamic completion message.
	model := newDynamicSpinnerModel(progressMsg)

	// Use inline mode - output to stderr, no alternate screen.
	opts := []tea.ProgramOption{
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	}
	if !terminal.HasRealTTYInput() {
		// TTY mode is forced (screenshots, cast recordings): keep the renderer,
		// but don't let bubbletea open /dev/tty for input — there isn't one.
		opts = append(opts, tea.WithInput(nil))
	}
	p := tea.NewProgram(model, opts...)

	// goroutineDone is closed once the operation finishes, guaranteeing that any
	// values the operation wrote (e.g. via a closure) are visible to the caller
	// after ExecWithSpinnerDynamic returns.
	goroutineDone := make(chan struct{})
	go func() {
		defer close(goroutineDone)
		completedMsg, err := operation()
		p.Send(opCompleteDynamicMsg{completedMsg: completedMsg, err: err})
	}()

	finalModel, spinnerErr := p.Run()

	// Always wait for the operation goroutine to finish before returning.
	// This prevents callers from reading uninitialized closure variables when
	// the bubbletea program exits before the operation completes (e.g. ctrl+c).
	<-goroutineDone

	return evaluateDynamicSpinnerResult(finalModel, spinnerErr)
}

// evaluateDynamicSpinnerResult interprets the outcome of a finished
// ExecWithSpinnerDynamic run. It returns a spinner-level error, the
// operation's own error, or ErrSpinnerOperationInterrupted if the spinner
// exited before the operation reported completion (e.g. the user pressed
// ctrl+c).
func evaluateDynamicSpinnerResult(finalModel tea.Model, spinnerErr error) error {
	if spinnerErr != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrTUIRun, spinnerErr)
	}
	if finalModel == nil {
		return errUtils.ErrSpinnerReturnedNilModel
	}

	m, ok := finalModel.(dynamicSpinnerModel)
	if !ok {
		return fmt.Errorf("%w: got %T", errUtils.ErrSpinnerUnexpectedModelType, finalModel)
	}
	if m.err != nil {
		return m.err
	}
	if !m.done {
		return errUtils.ErrSpinnerOperationInterrupted
	}

	return nil
}

// dynamicSpinnerModel is a spinner model where completion message is set dynamically.
type dynamicSpinnerModel struct {
	spinner      spinner.Model
	progressMsg  string // Message shown during operation.
	completedMsg string // Message shown when done (set dynamically).
	done         bool
	err          error
	width        int // Terminal width, from tea.WindowSizeMsg; 0 until the first resize event arrives.
}

type opCompleteDynamicMsg struct {
	completedMsg string
	err          error
}

//nolint:gocritic // bubbletea models must be passed by value
func (m dynamicSpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m dynamicSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		// A zero/negative width means the terminal genuinely doesn't have a size yet (e.g. a
		// freshly opened PTY that hasn't been resized): keep the initialWidth() fallback
		// already picked rather than stomping it down to 0, which would disable truncation
		// for the run's whole lifetime. See initialWidth's doc comment.
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case opCompleteDynamicMsg:
		m.done = true
		m.completedMsg = msg.completedMsg
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

//nolint:gocritic // bubbletea models must be passed by value
func (m dynamicSpinnerModel) View() string {
	if m.done {
		if m.err != nil {
			// Show error indicator with reset line to clear spinner.
			// Use FormatError for proper markdown rendering.
			return terminal.EscResetLine + ui.FormatError(m.progressMsg) + newline
		}
		// Show completed message with checkmark.
		// Use FormatSuccess for proper markdown rendering.
		displayMsg := m.completedMsg
		if displayMsg == "" {
			displayMsg = m.progressMsg
		}
		return terminal.EscResetLine + ui.FormatSuccess(displayMsg) + newline
	}
	// Show progress message with spinner.
	// Use FormatInline for proper markdown rendering (e.g., backtick code styling).
	line := fmt.Sprintf("%s %s", m.spinner.View(), ui.FormatInline(m.progressMsg))
	return terminal.EscResetLine + clipToWidth(line, m.width)
}

func newDynamicSpinnerModel(progressMsg string) dynamicSpinnerModel {
	s := newDotSpinner()
	return dynamicSpinnerModel{
		spinner:     s,
		progressMsg: progressMsg,
		width:       initialWidth(),
	}
}

// Spinner provides a start/stop API for long-running operations where multiple
// sequential steps need to run while the spinner is displayed. For single operations,
// prefer ExecWithSpinner or ExecWithSpinnerDynamic instead.
//
// Example usage:
//
//	s := spinner.New("Processing...")
//	s.Start()
//	defer s.Stop()
//
//	// Do multiple operations while spinner runs...
//	step1()
//	step2()
//
//	s.Success("Processing complete")
type Spinner struct {
	progressMsg string
	program     *tea.Program
	done        chan struct{}
	isTTY       bool
}

// New creates a new Spinner with the given progress message.
func New(progressMsg string) *Spinner {
	defer perf.Track(nil, "spinner.New")()

	return &Spinner{
		progressMsg: progressMsg,
		isTTY:       term.IsTTYSupportForStdout(),
	}
}

// Start begins displaying the spinner. Call Stop() when done.
func (s *Spinner) Start() {
	if !s.isTTY {
		// No TTY - no spinner to show.
		return
	}

	model := newManualSpinnerModel(s.progressMsg)
	opts := []tea.ProgramOption{
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	}
	if !terminal.HasRealTTYInput() {
		// TTY mode is forced (screenshots, cast recordings): keep the renderer,
		// but don't let bubbletea open /dev/tty for input — there isn't one.
		opts = append(opts, tea.WithInput(nil))
	}
	s.program = tea.NewProgram(model, opts...)
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		_, _ = s.program.Run()
	}()
}

// Update replaces the in-progress message. In non-interactive output it emits
// the message as an informational line so long-running work remains visible.
func (s *Spinner) Update(message string) {
	if message == "" {
		return
	}

	if !s.isTTY {
		ui.Info(message)
		return
	}
	if s.program == nil {
		return
	}

	s.program.Send(manualUpdateMsg{message: message})
}

// Stop stops the spinner without displaying a completion message.
// Use Success() or Error() instead to show a completion status.
// Stop is idempotent and safe to call multiple times.
func (s *Spinner) Stop() {
	if s.program == nil {
		return
	}
	s.program.Send(manualStopMsg{})
	<-s.done
	s.program = nil
}

// Success stops the spinner and displays a success message with checkmark.
// Success is idempotent and safe to call multiple times.
func (s *Spinner) Success(message string) {
	if !s.isTTY {
		ui.Success(message)
		return
	}
	if s.program == nil {
		ui.Success(message)
		return
	}
	s.program.Send(manualStopMsg{message: message, success: true})
	<-s.done
	s.program = nil
}

// Error stops the spinner and displays an error message with xmark.
// Error is idempotent and safe to call multiple times.
func (s *Spinner) Error(message string) {
	if !s.isTTY {
		ui.Error(message)
		return
	}
	if s.program == nil {
		ui.Error(message)
		return
	}
	s.program.Send(manualStopMsg{message: message, success: false})
	<-s.done
	s.program = nil
}

// manualSpinnerModel is a spinner that runs until explicitly stopped.
type manualSpinnerModel struct {
	spinner     spinner.Model
	progressMsg string
	finalMsg    string
	success     bool
	done        bool
	width       int // Terminal width, from tea.WindowSizeMsg; 0 until the first resize event arrives.
}

type manualStopMsg struct {
	message string
	success bool
}

type manualUpdateMsg struct {
	message string
}

func newManualSpinnerModel(progressMsg string) manualSpinnerModel {
	s := newDotSpinner()
	return manualSpinnerModel{
		spinner:     s,
		progressMsg: progressMsg,
		width:       initialWidth(),
	}
}

//nolint:gocritic // bubbletea models must be passed by value
func (m manualSpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m manualSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		// A zero/negative width means the terminal genuinely doesn't have a size yet (e.g. a
		// freshly opened PTY that hasn't been resized): keep the initialWidth() fallback
		// already picked rather than stomping it down to 0, which would disable truncation
		// for the run's whole lifetime. See initialWidth's doc comment.
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case manualStopMsg:
		m.done = true
		m.finalMsg = msg.message
		m.success = msg.success
		return m, tea.Quit
	case manualUpdateMsg:
		m.progressMsg = msg.message
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

//nolint:gocritic // bubbletea models must be passed by value
func (m manualSpinnerModel) View() string {
	if m.done {
		if m.finalMsg == "" {
			// Just clear the line if no message.
			return terminal.EscResetLine
		}
		// Use FormatSuccess/FormatError for proper markdown rendering.
		if m.success {
			return terminal.EscResetLine + ui.FormatSuccess(m.finalMsg) + newline
		}
		return terminal.EscResetLine + ui.FormatError(m.finalMsg) + newline
	}
	// Use FormatInline for proper markdown rendering (e.g., backtick code styling).
	line := fmt.Sprintf("%s %s", m.spinner.View(), ui.FormatInline(m.progressMsg))
	return terminal.EscResetLine + clipToWidth(line, m.width)
}
