package step

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	viewportDefaultWidth  = 80
	viewportDefaultHeight = 24
	viewportTailBytes     = 64 * 1024
	viewportRefresh       = 100 * time.Millisecond
)

// viewportTail shares only the recent display text with the renderer. Full stdout
// and stderr are captured separately for step outputs and failure reporting.
type viewportTail struct {
	mu   sync.Mutex
	text string
}

// Write appends output under a lock and bounds the text retained for the live viewport.
func (b *viewportTail) Write(p []byte) (int, error) {
	defer perf.Track(nil, "step.viewportTail.Write")()

	b.mu.Lock()
	defer b.mu.Unlock()
	b.text += string(p)
	if len(b.text) > viewportTailBytes {
		b.text = b.text[len(b.text)-viewportTailBytes:]
	}
	return len(p), nil
}

// snapshot returns a consistent copy of the recent output for rendering.
func (b *viewportTail) snapshot() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text
}

type (
	viewportRefreshMsg  struct{}
	viewportFinishedMsg struct{}
)

type outputViewportModel struct {
	tail                *viewportTail
	title               string
	width, height       int
	maxWidth, maxHeight int
	padding             int
	spinner             spinner.Model
	done                bool
}

// Init starts spinner animation and periodic output refreshes.
func (m *outputViewportModel) Init() tea.Cmd {
	defer perf.Track(nil, "step.outputViewportModel.Init")()

	return tea.Batch(m.spinner.Tick, m.refresh())
}

// refresh schedules the next viewport redraw while the subprocess runs.
func (m *outputViewportModel) refresh() tea.Cmd {
	return tea.Tick(viewportRefresh, func(time.Time) tea.Msg { return viewportRefreshMsg{} })
}

// Update handles terminal resizing, animation, and clearing the completed viewport.
func (m *outputViewportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer perf.Track(nil, "step.outputViewportModel.Update")()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
	case viewportFinishedMsg:
		m.done = true
		return m, tea.Quit
	case viewportRefreshMsg:
		return m, m.refresh()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the newest output rows within the configured size and padding.
func (m *outputViewportModel) View() string {
	defer perf.Track(nil, "step.outputViewportModel.View")()

	if m.done {
		return ""
	}
	width, height := m.width, m.height
	if m.maxWidth > 0 {
		width = min(width, m.maxWidth)
	}
	if m.maxHeight > 0 {
		height = min(height, m.maxHeight)
	}
	// Leave the last column free to avoid automatic terminal wrapping.
	width = max(1, width-1)
	padding := min(max(0, m.padding), (width-1)/2)
	inset := strings.Repeat(" ", padding)
	contentWidth := width - 2*padding
	rows := max(0, height-1)
	text := strings.ReplaceAll(ansi.Strip(m.tail.snapshot()), "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	var view strings.Builder
	for _, line := range lines {
		// Treat carriage-return progress messages as updates to the same row.
		if index := strings.LastIndex(line, "\r"); index >= 0 {
			line = line[index+1:]
		}
		line = strings.ReplaceAll(line, "\t", "    ")
		view.WriteString(terminal.EscResetLine + inset + ansi.Truncate(line, contentWidth, "…") + inset)
		view.WriteByte('\n')
	}
	view.WriteString(terminal.EscResetLine + inset + ansi.Truncate(m.spinner.View()+" "+m.title, contentWidth, "…") + inset)
	return view.String()
}

// executeViewportWithIO captures masked streams, shows their live tail, and reveals full logs on failure.
func (w *OutputModeWriter) executeViewportWithIO(runner func(stdout, stderr io.Writer) error) (string, string, error) {
	term := terminal.New()
	if !term.IsTTY(terminal.Stderr) {
		// CI and redirected output remain readable and stream as work happens.
		return w.executeRawWithIO(runner)
	}
	var stdout, stderr bytes.Buffer
	tail := &viewportTail{}
	width, height := term.Width(terminal.Stderr), term.Height(terminal.Stderr)
	if width <= 0 {
		width = viewportDefaultWidth
	}
	if height <= 0 {
		height = viewportDefaultHeight
	}
	title := w.stepName
	if title == "" {
		title = "Command"
	}
	model := &outputViewportModel{
		tail: tail, title: title, width: width, height: height,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.GetCurrentStyles().Spinner)),
	}
	if w.viewport != nil {
		model.maxWidth, model.maxHeight = w.viewport.Width, w.viewport.Height
		model.padding = w.viewport.Padding
	}
	program := tea.NewProgram(model, tea.WithOutput(iolib.GetContext().UI()), tea.WithInput(nil), tea.WithoutSignalHandler())
	done := make(chan struct{})
	var renderErr error
	go func() {
		_, renderErr = program.Run()
		close(done)
	}()
	unregister := signals.RegisterExitCleanup(func() {
		program.Kill()
		<-done
	})
	defer unregister()
	runErr := runner(iolib.MaskWriter(io.MultiWriter(&stdout, tail)), iolib.MaskWriter(io.MultiWriter(&stderr, tail)))
	program.Send(viewportFinishedMsg{})
	<-done
	if runErr != nil || renderErr != nil {
		// The live window is cleared first; publish each full stream once.
		return w.fallbackToLog(stdout.String(), stderr.String(), runErr)
	}
	_, _ = io.WriteString(iolib.GetContext().UI(), w.formatStepFooter(nil)+"\n")
	return stdout.String(), stderr.String(), nil
}
