package vendor

import (
	"context"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// TestRunUpdateWithSpinner_NonTTY_RunsDirectly proves that when stderr isn't a TTY (the case for
// `go test`, matching cmd/version/list_test.go's TestFetchReleasesWithSpinner_Mock convention for
// the analogous spinner wrapper), doWork runs synchronously with a nil onProgress and its result
// is returned unchanged — no spinner/goroutine/channel machinery is exercised.
func TestRunUpdateWithSpinner_NonTTY_RunsDirectly(t *testing.T) {
	want := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "vpc", Status: vendoring.StatusUpToDate},
	}}

	var gotOnProgress vendorProgressFunc
	var onProgressWasCalled bool
	report, err := runUpdateWithSpinner(func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error) {
		gotOnProgress = onProgress
		if onProgress != nil {
			onProgress("vpc", 1, 1)
			onProgressWasCalled = true
		}
		return want, nil
	})

	require.NoError(t, err)
	assert.Same(t, want, report)
	assert.Nil(t, gotOnProgress, "no TTY present, so onProgress must be nil (no spinner/channel is set up)")
	assert.False(t, onProgressWasCalled)
}

// TestRunUpdateWithSpinner_NonTTY_PropagatesError proves an error from doWork is returned
// unchanged (not wrapped/swallowed) on the non-TTY path.
func TestRunUpdateWithSpinner_NonTTY_PropagatesError(t *testing.T) {
	report, err := runUpdateWithSpinner(func(vendorProgressFunc) (*vendoring.UpdateReport, error) {
		return nil, assert.AnError
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Nil(t, report)
}

// TestUpdateSpinnerModel_View_NeverWraps proves a long, unbroken component name (no spaces to
// word-wrap at) never makes the live "Checking <name>" status line contain a hard line break,
// since bubbletea's single-line, carriage-return-based redraw corrupts (stacks duplicate lines in
// the scrollback) if the rendered line wraps in the real terminal. At widths that exceed the fixed
// spinner+bar+count overhead the whole composed line must also fit strictly within the terminal
// width (never equal to it -- see liveLineMargin); at extremely narrow widths (smaller than that
// fixed overhead, which this fix doesn't shrink) only the no-wrap guarantee is checked.
//
// Historical note: this is the sibling of internal/exec/vendor_model.go's mixin line-stacking bug.
func TestUpdateSpinnerModel_View_NeverWraps(t *testing.T) {
	longName := "https://raw.githubusercontent.com/cloudposse/terraform-components/mixins/v0.3.2/src/mixins/account-verification.mixin.tf"

	for _, width := range []int{0, 1, 10, 40, 80, 200} {
		m := &updateSpinnerModel{
			spinner: spinner.New(),
			bar:     progress.New(progress.WithWidth(progressBarWidthFor(width))),
			width:   width,
			progress: updateProgressMsg{
				component: longName,
				index:     3,
				total:     10,
			},
		}

		view := m.View()

		assert.NotContainsf(t, view, "\n", "width=%d: live status line must never wrap onto a second line", width)
		if width >= 60 {
			assert.Lessf(t, lipgloss.Width(view), width, "width=%d: rendered line must never reach the terminal's true last column (liveLineMargin must always leave at least one trailing column)", width)
		}
	}
}

// TestUpdateSpinnerModel_View_NeverTouchesLastColumn proves that at widths exactly matching the
// fixed spinner+bar+count overhead (so the padded gap saturates the line), the rendered line still
// stops at least liveLineMargin columns short of the terminal's true last column, never landing
// exactly on it.
//
// Historical note: boundary case for the "cursor overlapping the progress bar" bug.
func TestUpdateSpinnerModel_View_NeverTouchesLastColumn(t *testing.T) {
	for _, width := range []int{60, 61, 79, 80, 81, 120, 200, 250} {
		m := &updateSpinnerModel{
			spinner: spinner.New(),
			bar:     progress.New(progress.WithWidth(progressBarWidthFor(width))),
			width:   width,
			progress: updateProgressMsg{
				component: "aws-sso-permission-sets",
				index:     13,
				total:     48,
			},
		}

		view := m.View()

		assert.Lessf(t, lipgloss.Width(view), width, "width=%d: rendered line must stop short of the true last column", width)
		assert.LessOrEqualf(t, lipgloss.Width(view), width-liveLineMargin, "width=%d: rendered line must respect the full liveLineMargin", width)
	}
}

// TestProgressBarWidthFor proves the check-phase bar scales up on wide terminals instead of
// staying pinned at the narrow-terminal floor, while never shrinking below it or exceeding the
// ceiling.
func TestProgressBarWidthFor(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{width: 0, want: progressBarWidth},
		{width: 80, want: progressBarWidth},
		{width: 120, want: progressBarWidth},
		{width: 200, want: 50},
		{width: 300, want: progressBarMaxWidth},
	}

	for _, tt := range tests {
		got := progressBarWidthFor(tt.width)
		assert.Equalf(t, tt.want, got, "progressBarWidthFor(%d)", tt.width)
		assert.GreaterOrEqualf(t, got, progressBarWidth, "progressBarWidthFor(%d) must never be below the floor", tt.width)
		assert.LessOrEqualf(t, got, progressBarMaxWidth, "progressBarWidthFor(%d) must never exceed the ceiling", tt.width)
	}

	assert.Greaterf(t, progressBarWidthFor(200), progressBarWidthFor(80), "the bar must render wider on a wide terminal than on a narrow one")
}

// TestUpdateSpinnerModel_View_TruncatesLongName proves the live status line truncates (rather than
// silently no-op'ing, which lipgloss's own Style.MaxWidth does at width 0) and marks the cut with
// an ellipsis when the component name doesn't fit.
func TestUpdateSpinnerModel_View_TruncatesLongName(t *testing.T) {
	longName := strings.Repeat("x", 200)
	m := &updateSpinnerModel{
		spinner: spinner.New(),
		bar:     progress.New(progress.WithWidth(progressBarWidth)),
		width:   60,
		progress: updateProgressMsg{
			component: longName,
			index:     1,
			total:     2,
		},
	}

	view := m.View()

	assert.NotContains(t, view, longName, "the full 200-char name must not appear verbatim in a 60-column line")
	assert.Contains(t, view, ellipsis)
}

// newTestUpdateSpinnerModel builds a model with real (but unconnected) progress/done channels,
// matching how runUpdateWithSpinner constructs one, for tests that drive Init/Update directly
// without running a full tea.Program.
func newTestUpdateSpinnerModel() *updateSpinnerModel {
	progressCh := make(chan tea.Msg, 1)
	doneCh := make(chan updateDoneMsg, 1)
	m := &updateSpinnerModel{
		spinner:    spinner.New(),
		bar:        progress.New(progress.WithWidth(progressBarWidth)),
		progressCh: progressCh,
		doneCh:     doneCh,
	}
	return m
}

// TestUpdateSpinnerModel_Init proves Init starts both the spinner's own tick and a listener for
// the update goroutine's messages, batched together (bubbletea's tea.Batch) rather than only one
// or the other.
func TestUpdateSpinnerModel_Init(t *testing.T) {
	m := newTestUpdateSpinnerModel()

	cmd := m.Init()
	require.NotNil(t, cmd)

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok, "Init must batch the spinner tick and waitForUpdateMsg together, got %T", msg)
	assert.Len(t, batch, 2)
}

// TestUpdateSpinnerModel_Update_WindowSizeMsg proves a WindowSizeMsg updates both the model's
// width and the progress bar's configured width to match (mirroring
// internal/exec/vendor_model.go's equivalent handling).
func TestUpdateSpinnerModel_Update_WindowSizeMsg(t *testing.T) {
	m := newTestUpdateSpinnerModel()

	newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})

	updated, ok := newModel.(*updateSpinnerModel)
	require.True(t, ok)
	assert.Equal(t, 200, updated.width)
	assert.Equal(t, progressBarWidthFor(200), updated.bar.Width)
	assert.Nil(t, cmd)
}

// TestUpdateSpinnerModel_Update_KeyMsg proves ordinary keypresses are ignored. A status display
// must not silently terminate an update because the user happened to press a key.
func TestUpdateSpinnerModel_Update_KeyMsg(t *testing.T) {
	m := newTestUpdateSpinnerModel()

	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	updated, ok := newModel.(*updateSpinnerModel)
	require.True(t, ok)
	assert.False(t, updated.canceled)
	assert.Nil(t, cmd)
}

// TestUpdateSpinnerModel_Update_CtrlC proves the one explicit cancellation control quits the
// spinner so runUpdateWithSpinner can return a visible cancellation error to the CLI.
func TestUpdateSpinnerModel_Update_CtrlC(t *testing.T) {
	m := newTestUpdateSpinnerModel()

	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	updated, ok := newModel.(*updateSpinnerModel)
	require.True(t, ok)
	assert.True(t, updated.canceled)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestUpdateSpinnerResult_Canceled(t *testing.T) {
	report, err := updateSpinnerResult(&updateSpinnerModel{canceled: true})

	assert.Nil(t, report)
	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "vendor update canceled")
}

// TestUpdateSpinnerResult_NilModel proves a nil final model (bubbletea's Run returning nil
// without error, which shouldn't happen but must not be silently mistaken for success) surfaces
// a clear error instead of a nil pointer dereference further down the call chain.
func TestUpdateSpinnerResult_NilModel(t *testing.T) {
	report, err := updateSpinnerResult(nil)

	assert.Nil(t, report)
	require.ErrorIs(t, err, errUtils.ErrSpinnerReturnedNilModel)
}

// unexpectedTeaModel is a minimal tea.Model stand-in used only to prove updateSpinnerResult
// rejects a final model of the wrong concrete type instead of panicking on a failed type
// assertion.
type unexpectedTeaModel struct{}

func (unexpectedTeaModel) Init() tea.Cmd                       { return nil }
func (unexpectedTeaModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return unexpectedTeaModel{}, nil }
func (unexpectedTeaModel) View() string                        { return "" }

// TestUpdateSpinnerResult_UnexpectedModelType proves a final model of the wrong concrete type
// (a programming error, since only *updateSpinnerModel is ever run) surfaces a clear error
// instead of a panicking type assertion.
func TestUpdateSpinnerResult_UnexpectedModelType(t *testing.T) {
	report, err := updateSpinnerResult(unexpectedTeaModel{})

	assert.Nil(t, report)
	require.ErrorIs(t, err, errUtils.ErrSpinnerUnexpectedModelType)
}

// TestUpdateSpinnerModel_Update_SpinnerAndProgressFrames proves the spinner.TickMsg and
// progress.FrameMsg cases delegate to their respective sub-models without panicking, mirroring
// bubbletea's standard "delegate to child model, keep its updated copy" pattern.
func TestUpdateSpinnerModel_Update_SpinnerAndProgressFrames(t *testing.T) {
	m := newTestUpdateSpinnerModel()

	newModel, _ := m.Update(m.spinner.Tick())
	_, ok := newModel.(*updateSpinnerModel)
	assert.True(t, ok)

	newModel, _ = m.Update(progress.FrameMsg{})
	_, ok = newModel.(*updateSpinnerModel)
	assert.True(t, ok)
}

// TestUpdateSpinnerModel_Update_ProgressMsg proves an updateProgressMsg records the current
// component and, only when total is known (> 0), advances the bar and re-arms waitForUpdateMsg so
// the event loop keeps listening for the next message.
func TestUpdateSpinnerModel_Update_ProgressMsg(t *testing.T) {
	t.Run("with total", func(t *testing.T) {
		m := newTestUpdateSpinnerModel()

		newModel, cmd := m.Update(updateProgressMsg{component: "vpc", index: 1, total: 3})

		updated, ok := newModel.(*updateSpinnerModel)
		require.True(t, ok)
		assert.Equal(t, "vpc", updated.progress.component)
		assert.Equal(t, 1, updated.progress.index)
		assert.Equal(t, 3, updated.progress.total)
		require.NotNil(t, cmd, "must re-arm waitForUpdateMsg so the loop keeps listening")
	})

	t.Run("without total", func(t *testing.T) {
		m := newTestUpdateSpinnerModel()

		newModel, cmd := m.Update(updateProgressMsg{component: "vpc", index: 0, total: 0})

		updated, ok := newModel.(*updateSpinnerModel)
		require.True(t, ok)
		assert.Equal(t, "vpc", updated.progress.component)
		require.NotNil(t, cmd, "waitForUpdateMsg must still be re-armed even without a known total")
	})
}

// TestUpdateSpinnerModel_Update_DoneMsg proves updateDoneMsg records the final report/error,
// marks the model done, and quits the program -- the terminal message runUpdateWithSpinner's
// caller relies on to read final.report/final.err back out.
func TestUpdateSpinnerModel_Update_DoneMsg(t *testing.T) {
	want := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc"}}}
	m := newTestUpdateSpinnerModel()

	newModel, cmd := m.Update(updateDoneMsg{report: want, err: assert.AnError})

	updated, ok := newModel.(*updateSpinnerModel)
	require.True(t, ok)
	assert.Same(t, want, updated.report)
	assert.ErrorIs(t, updated.err, assert.AnError)
	assert.True(t, updated.done)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

// TestUpdateSpinnerModel_Update_UnknownMsg proves an unrecognized message type is a no-op:
// returned unchanged, with no command, rather than panicking on an unhandled type switch case.
func TestUpdateSpinnerModel_Update_UnknownMsg(t *testing.T) {
	m := newTestUpdateSpinnerModel()

	newModel, cmd := m.Update(struct{}{})

	assert.Same(t, m, newModel)
	assert.Nil(t, cmd)
}

// TestWaitForUpdateMsg_ProgressChannel proves waitForUpdateMsg returns a progress message when
// one is available on progressCh.
func TestWaitForUpdateMsg_ProgressChannel(t *testing.T) {
	progressCh := make(chan tea.Msg, 1)
	doneCh := make(chan updateDoneMsg, 1)
	want := updateProgressMsg{component: "vpc", index: 1, total: 2}
	progressCh <- want

	got := waitForUpdateMsg(progressCh, doneCh)()

	assert.Equal(t, tea.Msg(want), got)
}

// TestWaitForUpdateMsg_DoneChannel proves waitForUpdateMsg returns the terminal message when one
// is available on doneCh -- the regression this whole split-channel design (see
// runUpdateWithSpinner's doc comment) exists to guarantee delivery for.
func TestWaitForUpdateMsg_DoneChannel(t *testing.T) {
	progressCh := make(chan tea.Msg, 1)
	doneCh := make(chan updateDoneMsg, 1)
	want := updateDoneMsg{report: &vendoring.UpdateReport{}}
	doneCh <- want

	got := waitForUpdateMsg(progressCh, doneCh)()

	assert.Equal(t, tea.Msg(want), got)
}

// TestRunUpdateWithSpinner_TTY_RunsSpinnerAndReturnsResult drives runUpdateWithSpinner's TTY
// branch end to end against a real PTY (stderr must report as a terminal for isatty.IsTerminal to
// take this path), proving the spinner program starts, streams a progress update, and returns
// doWork's report/error unchanged once it completes -- the path
// TestRunUpdateWithSpinner_NonTTY_* deliberately don't exercise.
func TestRunUpdateWithSpinner_TTY_RunsSpinnerAndReturnsResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported on Windows")
	}

	ptmx, ttyFile, err := pty.Open()
	require.NoError(t, err)
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = ttyFile.Close() }()

	// bubbletea defaults to os.Stdin for input, and (since go test's own stdin isn't a terminal)
	// falls back to opening /dev/tty directly when it isn't -- unavailable in a headless sandbox
	// with no controlling terminal at all. Pointing stdin at the same PTY avoids that fallback,
	// matching what a real controlling-terminal session (or the CLI acceptance test harness's
	// own PTY, which attaches to all three of the child process's standard streams) looks like.
	origStdin := os.Stdin
	os.Stdin = ttyFile
	defer func() { os.Stdin = origStdin }()

	origStderr := os.Stderr
	os.Stderr = ttyFile
	defer func() { os.Stderr = origStderr }()

	// Drain the PTY's master side so the spinner's writes never block on a full PTY buffer.
	go func() { _, _ = io.Copy(io.Discard, ptmx) }()

	want := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "vpc", Status: vendoring.StatusUpToDate},
	}}

	type result struct {
		report *vendoring.UpdateReport
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		report, err := runUpdateWithSpinner(func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error) {
			if onProgress != nil {
				onProgress("vpc", 1, 1)
			}
			return want, nil
		})
		resultCh <- result{report, err}
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		assert.Same(t, want, got.report)
	case <-time.After(5 * time.Second):
		t.Fatal("runUpdateWithSpinner did not return within 5s under a TTY stderr")
	}
}
