package vendor

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestUpdateSpinnerModel_View_NeverWraps is a regression test for the sibling of
// internal/exec/vendor_model.go's mixin line-stacking bug: a long, unbroken component name (no
// spaces to word-wrap at) must never make the live "Checking <name>" status line contain a hard
// line break, since bubbletea's single-line, carriage-return-based redraw corrupts (stacks
// duplicate lines in the scrollback) if the rendered line wraps in the real terminal. At widths
// that exceed the fixed spinner+bar+count overhead the whole composed line must also fit strictly
// within the terminal width (never equal to it -- see liveLineMargin); at extremely narrow widths
// (smaller than that fixed overhead, which this fix doesn't shrink) only the no-wrap guarantee is
// checked.
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

// TestUpdateSpinnerModel_View_NeverTouchesLastColumn is a boundary-focused regression test for the
// "cursor overlapping the progress bar" bug: at widths exactly matching the fixed
// spinner+bar+count overhead (so the padded gap saturates the line), the rendered line must still
// stop at least liveLineMargin columns short of the terminal's true last column, never landing
// exactly on it.
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
