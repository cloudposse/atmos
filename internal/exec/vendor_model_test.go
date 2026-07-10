package exec

import (
	"bytes"
	"errors"
	stdio "io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Note on testing bubbletea's TTY/cursor/line-clearing behavior:
//
// executeVendorModel's fix for the "cursor visible" / "progress bar leaves
// artifacts" bugs lives in pkg/io's maskedWriter (see streams.go and
// streams_test.go's TestMaskedWriter_TermFileCapability), which makes
// iolib.MaskWriter(os.Stdout) transparently satisfy bubbletea's term.File
// capability check. That fix is covered by real unit tests using a real pty
// (via github.com/creack/pty) in pkg/io.
//
// The downstream effect -- bubbletea's internal renderer correctly detecting
// terminal size (WindowSizeMsg), hiding/showing the cursor, and emitting
// \x1b[K to clear stale content when a shorter line replaces a longer one --
// happens deep inside tea.Program.Run(), which opens real TTY file
// descriptors and spawns background goroutines (checkResize, readLoop). This
// isn't practical to unit test without a real pty attached to the process's
// actual stdout, and doing so here would just re-test bubbletea's own
// renderer rather than atmos code. That behavior was instead verified
// manually end-to-end: running `atmos vendor pull` under a real pty (macOS
// `script -q /dev/null <cmd>`) with a git-clone-backed vendor.yaml and a
// mixins-based component.yaml, capturing raw terminal bytes, and confirming
// (a) exactly one \x1b[?25l / \x1b[?25h pair bracketing the whole run and (b)
// \x1b[K correctly appears after every completed line shorter than the
// terminal width (and is correctly omitted when a line exactly fills the
// terminal width) -- both zero before this fix, in an otherwise identical
// capture.
func TestVendorFailureError(t *testing.T) {
	// Regression test: the vendor error must contain a descriptive explanation
	// listing the failed component names, not just a bare integer count.
	t.Run("single failed component", func(t *testing.T) {
		err := vendorFailureError(1, 3, []string{"my-vpc"})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVendorComponents)

		config := errUtils.DefaultFormatterConfig()
		formatted := errUtils.Format(err, config)

		assert.NotContains(t, formatted, "## Explanation")
		assert.Contains(t, formatted, "my-vpc")
		assert.Contains(t, formatted, "Failed to vendor 1 of 3 components")
	})

	t.Run("multiple failed components", func(t *testing.T) {
		err := vendorFailureError(3, 5, []string{"my-vpc", "my-rds", "my-s3"})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVendorComponents)

		config := errUtils.DefaultFormatterConfig()
		formatted := errUtils.Format(err, config)

		assert.NotContains(t, formatted, "## Explanation")
		assert.Contains(t, formatted, "my-vpc")
		assert.Contains(t, formatted, "my-rds")
		assert.Contains(t, formatted, "my-s3")
		assert.Contains(t, formatted, "Failed to vendor 3 of 5 components")
	})
}

func TestHandleInstalledPkgMsg_TracksFailedNames(t *testing.T) {
	// Verify that handleInstalledPkgMsg appends failed package names.
	m := &modelVendor{
		packages: []pkgVendor{
			{name: "vpc"},
			{name: "rds"},
		},
		index: 0,
		isTTY: false,
	}

	// Simulate a failed install message.
	msg := &installedPkgMsg{
		err:  errors.New("download failed"),
		name: "vpc",
	}
	m.handleInstalledPkgMsg(msg)

	assert.Equal(t, 1, m.failedPkg)
	assert.Equal(t, []string{"vpc"}, m.failedPkgNames)

	// Simulate a second package succeeding.
	m.index = 1
	msg2 := &installedPkgMsg{name: "rds"}
	m.handleInstalledPkgMsg(msg2)

	// Failed count should not change.
	assert.Equal(t, 1, m.failedPkg)
	assert.Equal(t, []string{"vpc"}, m.failedPkgNames)
}

func TestHandleInstalledPkgMsg_NonTTYStatusOutput(t *testing.T) {
	stderr, cleanup := setupVendorModelTestUI(t)
	defer cleanup()

	t.Run("non-final success logs package status", func(t *testing.T) {
		stderr.Reset()
		m := &modelVendor{
			packages: []pkgVendor{
				{name: "vpc", version: "1.0.0"},
				{name: "rds", version: "2.0.0"},
			},
			isTTY: false,
		}

		_, cmd := m.handleInstalledPkgMsg(&installedPkgMsg{name: "vpc"})

		assert.NotNil(t, cmd)
		assert.Equal(t, 1, m.index)
		assert.Contains(t, atmosansi.Strip(stderr.String()), "vpc (1.0.0)")
	})

	t.Run("final failure logs failed package and summary", func(t *testing.T) {
		stderr.Reset()
		m := &modelVendor{
			packages: []pkgVendor{
				{name: "vpc", version: "1.0.0"},
			},
			isTTY: false,
		}

		_, cmd := m.handleInstalledPkgMsg(&installedPkgMsg{
			name: "vpc",
			err:  errors.New("download failed"),
		})

		assert.NotNil(t, cmd)
		assert.True(t, m.done)
		output := atmosansi.Strip(stderr.String())
		assert.Contains(t, output, "Failed to vendor vpc")
		assert.Contains(t, output, "vpc (1.0.0)")
		assert.Contains(t, output, "Vendored components (success: 0, failed: 1)")
	})
}

func TestLogNonTTYFinalStatus_DryRunSuccessSummary(t *testing.T) {
	stderr, cleanup := setupVendorModelTestUI(t)
	defer cleanup()

	m := &modelVendor{
		packages: []pkgVendor{
			{name: "vpc", version: "1.0.0"},
		},
		dryRun: true,
		isTTY:  false,
	}

	m.logNonNTYFinalStatus(m.packages[0], false)

	output := atmosansi.Strip(stderr.String())
	assert.Contains(t, output, "vpc (1.0.0)")
	assert.Contains(t, output, "Done! Dry run completed. No components vendored")
	assert.Contains(t, output, "Vendored components (success: 1)")
}

type vendorModelTestStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *vendorModelTestStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *vendorModelTestStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *vendorModelTestStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *vendorModelTestStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *vendorModelTestStreams) RawError() stdio.Writer  { return ts.stderr }

// newVendorModelForView builds a modelVendor with an initialized spinner/progress bar so View()
// can be exercised directly in tests without going through newModelVendor (which would query the
// real terminal for its initial width).
func newVendorModelForView(packages []pkgVendor, width int) *modelVendor {
	return &modelVendor{
		packages: packages,
		spinner:  spinner.New(),
		progress: progress.New(progress.WithWidth(progressBarWidth)),
		width:    width,
	}
}

// TestModelVendor_ComponentCount_ExcludesMixins is a regression test: mixins are appended into
// m.packages alongside their owning component (see buildComponentVendorPackages in
// vendor_component_utils.go), so len(m.packages) over-counts "components" whenever any component
// declares mixins. The componentCount and mixinCount helpers must separate the two so completion
// messages ("Vendored N components") match what `vendor update` already reported (e.g. "Updated N
// component(s)").
func TestModelVendor_ComponentCount_ExcludesMixins(t *testing.T) {
	m := &modelVendor{
		packages: []pkgVendor{
			{name: "vpc", componentPackage: &pkgComponentVendor{name: "vpc"}},
			{name: "mixin https://example.com/a.tf", componentPackage: &pkgComponentVendor{name: "mixin https://example.com/a.tf", IsMixins: true}},
			{name: "mixin https://example.com/b.tf", componentPackage: &pkgComponentVendor{name: "mixin https://example.com/b.tf", IsMixins: true}},
			{name: "rds", componentPackage: &pkgComponentVendor{name: "rds"}},
		},
	}

	assert.Equal(t, 2, m.componentCount())
	assert.Equal(t, 2, m.mixinCount())
	assert.Equal(t, 4, len(m.packages), "sanity: the raw package list still contains mixins")
}

// TestModelVendor_View_DoneState_CountsExcludeMixins proves the TTY "done" summary in View() uses
// componentCount, not len(m.packages), so a component with mixins doesn't inflate the number of
// "components" reported as vendored.
func TestModelVendor_View_DoneState_CountsExcludeMixins(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := newVendorModelForView([]pkgVendor{
			{name: "vpc", componentPackage: &pkgComponentVendor{name: "vpc"}},
			{name: "mixin https://example.com/a.tf", componentPackage: &pkgComponentVendor{name: "mixin https://example.com/a.tf", IsMixins: true}},
		}, 80)
		m.done = true

		view := atmosansi.Strip(m.View())

		assert.Contains(t, view, "Vendored 1 components (1 mixins).")
		assert.NotContains(t, view, "Vendored 2 components")
	})

	t.Run("failure", func(t *testing.T) {
		m := newVendorModelForView([]pkgVendor{
			{name: "vpc", componentPackage: &pkgComponentVendor{name: "vpc"}},
			{name: "mixin https://example.com/a.tf", componentPackage: &pkgComponentVendor{name: "mixin https://example.com/a.tf", IsMixins: true}},
		}, 80)
		m.done = true
		m.failedPkg = 1
		m.failedMixins = 1 // the mixin failed, not the component.

		view := atmosansi.Strip(m.View())

		// The component succeeded; only the mixin failed, so the component-only counts must read
		// "1 vendored, 0 failed" rather than folding the mixin failure into "components".
		assert.Contains(t, view, "Vendored 1 components. Failed to vendor 0 components.")
	})
}

// TestModelVendor_LogNonTTYFinalStatus_CountsExcludeMixins is logNonNTYFinalStatus's counterpart
// to TestModelVendor_View_DoneState_CountsExcludeMixins for the non-TTY summary line.
func TestModelVendor_LogNonTTYFinalStatus_CountsExcludeMixins(t *testing.T) {
	stderr, cleanup := setupVendorModelTestUI(t)
	defer cleanup()

	m := &modelVendor{
		packages: []pkgVendor{
			{name: "vpc", version: "1.0.0", componentPackage: &pkgComponentVendor{name: "vpc"}},
			{name: "mixin https://example.com/a.tf", componentPackage: &pkgComponentVendor{name: "mixin https://example.com/a.tf", IsMixins: true}},
		},
		isTTY: false,
	}

	m.logNonNTYFinalStatus(m.packages[0], false)

	output := atmosansi.Strip(stderr.String())
	assert.Contains(t, output, "Vendored components (success: 1, mixins: 1)")
	assert.NotContains(t, output, "success: 2")
}

// TestModelVendor_View_LiveLine_NeverWraps is a regression test for the mixin line-stacking bug: a
// mixin's name is its full source URL (100+ chars, one unbroken token with no spaces), so the live
// "Pulling <name>" status line must never contain a hard line break or exceed the available width,
// regardless of how narrow cellsAvail is. Bubbletea's single-line, carriage-return-based redraw
// (used for this non-altscreen progress line) stacks duplicate lines in the scrollback if the
// rendered line ever wraps in the real terminal. The rendered line must also never reach the
// terminal's true last column (never equal to width -- see liveLineMargin).
func TestModelVendor_View_LiveLine_NeverWraps(t *testing.T) {
	longURL := "https://raw.githubusercontent.com/cloudposse/terraform-components/mixins/v0.3.2/src/mixins/account-verification.mixin.tf"
	packages := []pkgVendor{
		{name: "mixin " + longURL, version: "v0.3.2", componentPackage: &pkgComponentVendor{name: "mixin " + longURL, IsMixins: true}},
	}

	for _, width := range []int{0, 1, 10, 40, 80, 200} {
		m := newVendorModelForView(packages, width)

		view := m.View()

		assert.NotContainsf(t, view, "\n", "width=%d: live status line must never wrap onto a second line", width)
		if width >= 60 {
			assert.Lessf(t, lipgloss.Width(view), width, "width=%d: rendered line must never reach the terminal's true last column (liveLineMargin must always leave at least one trailing column)", width)
		}
	}
}

// TestModelVendor_View_NeverTouchesLastColumn is a boundary-focused regression test for the
// pending-autowrap hazard: at widths exactly matching the fixed spinner+bar+count overhead (so
// the padded gap saturates the line), the rendered line must still stop at least liveLineMargin
// columns short of the terminal's true last column, never landing exactly on it.
func TestModelVendor_View_NeverTouchesLastColumn(t *testing.T) {
	packages := []pkgVendor{
		{name: "datadog-monitor", componentPackage: &pkgComponentVendor{name: "datadog-monitor"}},
		{name: "other", componentPackage: &pkgComponentVendor{name: "other"}},
	}

	for _, width := range []int{60, 61, 79, 80, 81, 120, 200, 250} {
		m := newVendorModelForView(packages, width)
		m.progress.Width = progressBarWidthFor(width)

		view := m.View()

		assert.Lessf(t, lipgloss.Width(view), width, "width=%d: rendered line must stop short of the true last column", width)
		assert.LessOrEqualf(t, lipgloss.Width(view), width-liveLineMargin, "width=%d: rendered line must respect the full liveLineMargin", width)
	}
}

// TestProgressBarWidthFor proves the pull-phase bar scales up on wide terminals instead of
// staying pinned at the narrow-terminal floor (the "artificially small progress bar" complaint),
// while never shrinking below the floor or exceeding the ceiling.
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

// TestModelVendor_Update_WindowSizeMsg_RightAlignsAgainstRealWidth is a regression test for the
// "progress bar feels artificially small" bug: a wide terminal's WindowSizeMsg must right-align
// the live line against the real reported width (up to the generous maxWidth sanity ceiling), not
// an aggressively low fixed cap, and the bar itself must grow wider accordingly.
func TestModelVendor_Update_WindowSizeMsg_RightAlignsAgainstRealWidth(t *testing.T) {
	packages := []pkgVendor{
		{name: "datadog-monitor", componentPackage: &pkgComponentVendor{name: "datadog-monitor"}},
	}
	m := newVendorModelForView(packages, 0)

	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})

	assert.Equal(t, 200, m.width, "a 200-column terminal must not be clamped down to the old 120-column cap")
	assert.Equal(t, progressBarWidthFor(200), m.progress.Width, "the bar's configured width must scale with the new terminal width")
	assert.Greater(t, m.progress.Width, progressBarWidth, "the bar must be wider than the narrow-terminal floor on a 200-column terminal")

	view := lipgloss.Width(m.View())
	assert.Greaterf(t, view, 150, "the rendered live line must extend well past the old 120-column cap on a 200-column terminal, got width %d", view)
	assert.Less(t, view, 200, "the rendered live line must still stop short of the true last column")
}

// TestModelVendor_Update_WindowSizeMsg_ZeroWidthDoesNotResetKnownWidth is a regression test for a
// hang observed in the CLI acceptance test harness: simulateTtyCommand (tests/cli_test.go) opens a
// PTY via pty.Start without ever calling Setsize, so term.GetSize on that PTY's fd reports 0x0.
// Bubbletea's checkResize (tty.go) still queries and delivers that as a real WindowSizeMsg{0, 0}
// once maskedWriter.Fd() (pkg/io/streams.go) started forwarding to the real fd so bubbletea could
// detect the terminal at all -- so a zero-size WindowSizeMsg is a real message that must not wipe
// out the width initialModelWidth already established (with its own fallback), or "Pulling <name>"
// truncates down to a bare ellipsis (truncate.StringWithTail at width 0) for the run's whole
// lifetime, since tea.WindowSizeMsg only fires once (bubbletea has no SIGWINCH-driven resends here).
func TestModelVendor_Update_WindowSizeMsg_ZeroWidthDoesNotResetKnownWidth(t *testing.T) {
	packages := []pkgVendor{
		{name: "datadog-monitor", componentPackage: &pkgComponentVendor{name: "datadog-monitor"}},
	}
	m := newVendorModelForView(packages, fallbackModelWidth)

	m.Update(tea.WindowSizeMsg{Width: 0, Height: 0})

	assert.Equal(t, fallbackModelWidth, m.width, "a zero-size WindowSizeMsg must not overwrite the already-known width")

	view := m.View()
	assert.Contains(t, view, "Pulling", "the live status line must still show 'Pulling <name>', not collapse to a bare ellipsis")
}

// TestModelVendor_View_LiveLine_TruncatesLongName proves the live status line truncates (rather
// than silently no-op'ing, which lipgloss's own Style.MaxWidth does at width 0) and marks the cut
// with an ellipsis when the package name doesn't fit.
func TestModelVendor_View_LiveLine_TruncatesLongName(t *testing.T) {
	longName := strings.Repeat("x", 200)
	packages := []pkgVendor{
		{name: longName, componentPackage: &pkgComponentVendor{name: longName}},
	}
	m := newVendorModelForView(packages, 60)

	view := m.View()

	assert.NotContains(t, view, longName, "the full 200-char name must not appear verbatim in a 60-column line")
	assert.Contains(t, view, ellipsis)
}

func setupVendorModelTestUI(t *testing.T) (stderr *bytes.Buffer, cleanup func()) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	streams := &vendorModelTestStreams{
		stdin:  strings.NewReader(""),
		stdout: stdout,
		stderr: stderr,
	}

	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)

	ui.InitFormatter(ioCtx)

	return stderr, ui.Reset
}
