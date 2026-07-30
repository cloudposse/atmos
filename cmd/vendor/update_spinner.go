package vendor

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/reflow/truncate"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/spinner/fps"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// progressBarWidth matches internal/exec/vendor_model.go's identically named const, which
// renders the equivalent progress bar for the sibling `atmos vendor pull` command. It's the floor
// the bar renders at on narrow terminals; see progressBarWidthFor.
const progressBarWidth = 30

// progressBarMaxWidth matches internal/exec/vendor_model.go's identically named const: the ceiling
// the progress bar grows to on wide terminals, so it doesn't stay pinned at progressBarWidth (and
// look artificially small) on a terminal with plenty of room to spare.
const progressBarMaxWidth = 60

// liveLineMargin reserves this many trailing columns so the live status line's rendered content
// never reaches the terminal's true last column. Writing content that exactly fills a terminal's
// width puts some terminal emulators into a "pending autowrap" state whose cursor-column rendering
// is inconsistent across implementations, independent of whether the cursor is hidden via DECTCEM
// (\x1b[?25l); reserving a small margin sidesteps that ambiguity entirely. Matches
// internal/exec/vendor_model.go's identically named const.
const liveLineMargin = 1

// ellipsis marks a component name that was truncated to fit the live status line.
const ellipsis = "…"

// progressBarWidthFor returns how wide the progress bar should render for a live status line of
// the given total terminal width: progressBarWidth on narrow terminals, growing toward
// progressBarMaxWidth as more room becomes available. Matches
// internal/exec/vendor_model.go's identically named helper.
func progressBarWidthFor(width int) int {
	w := width / 4
	switch {
	case w < progressBarWidth:
		return progressBarWidth
	case w > progressBarMaxWidth:
		return progressBarMaxWidth
	default:
		return w
	}
}

// vendorProgressFunc reports that the source named component is about to be checked against its
// remote, at 1-based position index out of total. It mirrors vendoring.UpdateParams.OnProgress's
// signature so it can be passed straight through.
type vendorProgressFunc func(component string, index, total int)

// vendorUpdateWork is the shape of the blocking vendor-update operation (vendoring.Update or
// vendoring.UpdateResolved, wrapped by the caller) that runUpdateWithSpinner drives. It receives a
// nil onProgress when no spinner is shown (no TTY); implementations must tolerate that.
type vendorUpdateWork func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error)

// updateProgressMsg reports the source currently being checked.
type updateProgressMsg struct {
	component string
	index     int
	total     int
}

// updateDoneMsg carries the final result of the (blocking) update operation back to the
// bubbletea event loop.
type updateDoneMsg struct {
	report *vendoring.UpdateReport
	err    error
}

// updateSpinnerModel drives a spinner and progress bar across vendor update's sequential,
// network-bound per-source checks, showing which component is currently being checked. It mirrors
// internal/exec/vendor_model.go's modelVendor (the equivalent model for the sibling
// `atmos vendor pull` command), extended to stream a message per source instead of fetching once.
type updateSpinnerModel struct {
	spinner    spinner.Model
	bar        progress.Model
	width      int
	progress   updateProgressMsg
	report     *vendoring.UpdateReport
	err        error
	done       bool
	progressCh <-chan tea.Msg
	doneCh     <-chan updateDoneMsg
}

func (m *updateSpinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForUpdateMsg(m.progressCh, m.doneCh))
}

func (m *updateSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.bar.Width = progressBarWidthFor(m.width)
		return m, nil
	case tea.KeyMsg:
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		newModel, cmd := m.bar.Update(msg)
		if newModel, ok := newModel.(progress.Model); ok {
			m.bar = newModel
		}
		return m, cmd
	case updateProgressMsg:
		m.progress = msg
		var progressCmd tea.Cmd
		if msg.total > 0 {
			progressCmd = m.bar.SetPercent(float64(msg.index) / float64(msg.total))
		}
		return m, tea.Batch(progressCmd, waitForUpdateMsg(m.progressCh, m.doneCh))
	case updateDoneMsg:
		m.report = msg.report
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m *updateSpinnerModel) View() string {
	if m.done {
		return ""
	}
	if m.progress.component == "" {
		return m.spinner.View() + " Checking vendor sources..."
	}
	if m.progress.total <= 0 {
		return fmt.Sprintf("%s Checking %s...", m.spinner.View(), m.progress.component)
	}

	// Mirrors internal/exec/vendor_model.go's modelVendor.View(): spinner + name on the left, a
	// space-filled gap, then the progress bar and a right-aligned "index/total" counter, all on
	// one line sized to the terminal width.
	w := lipgloss.Width(fmt.Sprintf("%d", m.progress.total))
	count := fmt.Sprintf(" %*d/%*d", w, m.progress.index, w, m.progress.total)
	spin := m.spinner.View() + " "
	bar := m.bar.View()
	// effectiveWidth reserves liveLineMargin trailing columns so the rendered line never touches
	// the terminal's true last column (see liveLineMargin's doc comment).
	effectiveWidth := max(0, m.width-liveLineMargin)
	cellsAvail := max(0, effectiveWidth-lipgloss.Width(spin+bar+count))
	name := theme.GetCurrentStyles().PackageName.Render(m.progress.component)
	// Truncate (never wrap) the "Checking <name>" segment to cellsAvail. lipgloss's own
	// Style.MaxWidth word-wraps once a Style.Width is also set, and is a no-op when cellsAvail is
	// 0, so it can't be relied on to guarantee a single line; truncate.StringWithTail can. See
	// internal/exec/vendor_model.go's modelVendor.View() for the sibling `vendor pull` fix this
	// mirrors (component names here are short, so this is defensive rather than a reported bug).
	info := truncate.StringWithTail("Checking "+name, uint(cellsAvail), ellipsis)
	cellsRemaining := max(0, effectiveWidth-lipgloss.Width(spin+info+bar+count))
	gap := strings.Repeat(" ", cellsRemaining)

	return spin + info + gap + bar + count
}

// waitForUpdateMsg returns a tea.Cmd that blocks for the next message on either channel and
// returns it. The model re-issues this Cmd every time it handles an updateProgressMsg, so the
// event loop keeps listening until an updateDoneMsg ends it — the standard bubbletea idiom for
// streaming progress from a background goroutine into a running program. ProgressCh and doneCh
// are separate channels (see runUpdateWithSpinner's doc comment) so a still-buffered progress
// message can never cause the terminal updateDoneMsg to be dropped.
func waitForUpdateMsg(progressCh <-chan tea.Msg, doneCh <-chan updateDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-progressCh:
			return msg
		case msg := <-doneCh:
			return msg
		}
	}
}

// runUpdateWithSpinner runs doWork, a blocking vendor-update operation that reports progress via
// the onProgress callback it's given. When stderr is not a TTY, doWork runs synchronously with a
// nil onProgress (no spinner, no progress output) — identical to running it directly. When stderr
// is a TTY, doWork runs in a background goroutine while a spinner on stderr shows which component
// is currently being checked, updated live via onProgress.
func runUpdateWithSpinner(doWork vendorUpdateWork) (*vendoring.UpdateReport, error) {
	defer perf.Track(nil, "vendor.runUpdateWithSpinner")()

	if !isatty.IsTerminal(os.Stderr.Fd()) && !isatty.IsCygwinTerminal(os.Stderr.Fd()) {
		// No TTY - run without a spinner.
		return doWork(nil)
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	fps.Apply(&s)

	bar := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(progressBarWidth),
		progress.WithoutPercentage(),
	)

	// progressCh streams best-effort progress from the goroutine performing the blocking,
	// sequential network checks to the bubbletea event loop. Buffered by 1 so the goroutine
	// never blocks handing off an update; sends use a non-blocking select so that if the
	// program has already exited (e.g. the user pressed a key, or a send raced ahead of the
	// reader) the goroutine drops the message and moves on rather than deadlocking - it still
	// runs doWork to completion (bounded by resolveLatest's own per-source timeout) and simply
	// has nothing left to notify once done.
	//
	// doneCh is a separate channel (not multiplexed onto progressCh) so the single, terminal
	// updateDoneMsg can never be dropped by a still-buffered progress message occupying
	// progressCh's slot: doneCh is buffered by 1 and receives exactly one send, so that send
	// always succeeds immediately regardless of whether the model has read it yet.
	progressCh := make(chan tea.Msg, 1)
	doneCh := make(chan updateDoneMsg, 1)
	m := &updateSpinnerModel{spinner: s, bar: bar, progressCh: progressCh, doneCh: doneCh}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))

	go func() {
		report, err := doWork(func(component string, index, total int) {
			select {
			case progressCh <- updateProgressMsg{component: component, index: index, total: total}:
			default:
			}
		})
		doneCh <- updateDoneMsg{report: report, err: err}
	}()

	finalModel, runErr := p.Run()
	if runErr != nil {
		return nil, fmt.Errorf("spinner execution failed: %w", runErr)
	}

	if finalModel == nil {
		return nil, fmt.Errorf("%w: spinner completed but returned nil model during vendor update", errUtils.ErrSpinnerReturnedNilModel)
	}

	final, ok := finalModel.(*updateSpinnerModel)
	if !ok {
		return nil, fmt.Errorf("%w: got %T", errUtils.ErrSpinnerUnexpectedModelType, finalModel)
	}

	return final.report, final.err
}
