package exec

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner/fps"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

// This file is the Bubble Tea TUI layer only: the live progress/spinner model that drives
// `atmos vendor pull`/`update`'s interactive output. Every fetch/copy/materialization-check/
// lock-record concern lives in pkg/vendoring/install (see install.Install and
// install.FilterPending), which has no Bubble Tea dependency and is directly unit-testable on its
// own. Each tea.Cmd below (ExecuteInstall) is a thin closure translating install.Install's plain
// (install.Result, error) return into this package's own installedPkgMsg tea.Msg.

const (
	// ProgressBarWidth is the floor the progress bar renders at on narrow terminals; see
	// progressBarWidthFor, which scales it up on wider ones.
	progressBarWidth = 30
	// ProgressBarMaxWidth is the ceiling the progress bar grows to on wide terminals, so it
	// doesn't stay pinned at progressBarWidth (and look artificially small) when there's plenty
	// of room to spare.
	progressBarMaxWidth = 60
	// MaxWidth is a sanity ceiling on the live status line's width, not the effective width most
	// terminals hit in practice: it exists only to bound absurdly wide layouts (e.g. an
	// accidentally huge reported terminal width), not to visually cap right-alignment on ordinary
	// wide terminals the way a low value would. Right-alignment always uses the real, current
	// terminal width up to this ceiling.
	maxWidth = 250
	// LiveLineMargin reserves this many trailing columns so the live status line's rendered
	// content never reaches the terminal's true last column. Writing content that exactly fills a
	// terminal's width puts some terminal emulators into a "pending autowrap" state whose
	// cursor-column rendering is inconsistent across implementations, independent of whether the
	// cursor is hidden via DECTCEM (\x1b[?25l); reserving a small margin sidesteps that ambiguity
	// entirely. Matches cmd/vendor/update_spinner.go's identically named const.
	liveLineMargin = 1
	// FallbackModelWidth is the live status line's width when the real terminal width can't be
	// detected (terminal.Width reports 0, e.g. a PTY that hasn't been given a window size yet, as
	// in the CLI acceptance test harness). Matches pkg/toolchain/info.go's identically valued
	// fallbackTerminalWidth. Without this, cellsAvail (this file's View()) would compute to 0 for
	// the run's entire lifetime -- both initialModelWidth and Update's tea.WindowSizeMsg handler
	// must apply it, see their doc comments -- and truncate.StringWithTail truncates "Pulling
	// <name>" down to a bare ellipsis at width 0 instead of the full text.
	fallbackModelWidth = 120

	// Package status format string for per-package status messages.
	pkgStatusFmt = "%s %s"

	// Ellipsis marks a package/mixin name that was truncated to fit the live
	// status line (e.g. a long mixin source URL).
	ellipsis = "…"
)

// progressBarWidthFor returns how wide the progress bar should render for a live status line of
// the given total terminal width: progressBarWidth on narrow terminals, growing toward
// progressBarMaxWidth as more room becomes available. Matches
// cmd/vendor/update_spinner.go's identically named helper.
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

var (
	currentPkgNameStyle = theme.Styles.PackageName
	doneStyle           = lipgloss.NewStyle().Margin(1, 2)
	checkMark           = theme.Styles.Checkmark
	xMark               = theme.Styles.XMark
	grayColor           = theme.Styles.GrayText
)

// installedPkgMsg is this package's own tea.Msg shape, translated from install.Result by
// ExecuteInstall.
type installedPkgMsg struct {
	err  error
	name string
}

type modelVendor struct {
	packages       []install.VendorPackage
	index          int
	width          int
	height         int
	spinner        spinner.Model
	progress       progress.Model
	done           bool
	dryRun         bool
	failedPkg      int
	failedMixins   int // subset of failedPkg whose package is a mixin; see failedComponentCount.
	failedPkgNames []string
	atmosConfig    *schema.AtmosConfiguration
	isTTY          bool
}

// executeVendorModel runs the interactive install TUI for packages, translating opts into each
// package's install.Install call.
func executeVendorModel(
	packages []install.VendorPackage,
	opts install.InstallOptions,
	atmosConfig *schema.AtmosConfiguration,
) error {
	if len(packages) == 0 {
		return nil
	}

	model, err := newModelVendor(packages, opts.DryRun, atmosConfig)
	if err != nil {
		return fmt.Errorf("%w: %v (verify terminal capabilities and permissions)", errUtils.ErrTUIModel, err)
	}

	progOpts := []tea.ProgramOption{tea.WithOutput(iolib.MaskWriter(os.Stdout))}
	if !term.IsTTYSupportForStdout() {
		progOpts = append(progOpts, tea.WithoutRenderer(), tea.WithInput(nil))
		log.Debug("No TTY detected. Falling back to basic output. This can happen when no terminal is attached or when commands are pipelined.")
	} else if !terminal.HasRealTTYInput() {
		// TTY mode is forced (screenshots, cast recordings): keep the renderer,
		// but don't let bubbletea open /dev/tty for input — there isn't one.
		progOpts = append(progOpts, tea.WithInput(nil))
	}

	if _, err := tea.NewProgram(&model, progOpts...).Run(); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	if model.failedPkg > 0 {
		return vendorFailureError(model.failedPkg, len(model.packages), model.failedPkgNames)
	}
	return nil
}

// vendorFailureError builds a descriptive error listing the names of the
// components that failed to vendor.
func vendorFailureError(failedCount, totalCount int, failedNames []string) error {
	explanation := fmt.Sprintf("Failed to vendor %d of %d components: %s",
		failedCount, totalCount, strings.Join(failedNames, ", "))
	return errUtils.Build(ErrVendorComponents).
		WithExplanation(explanation).
		Err()
}

// newModelVendor constructs a modelVendor prepared to run vendor installations from packages. It
// initializes the progress bar and spinner and sets dryRun, atmosConfig, and TTY detection on the
// returned model. If packages is empty the returned model has done set to true. The function
// never performs network or filesystem operations.
func newModelVendor(
	packages []install.VendorPackage,
	dryRun bool,
	atmosConfig *schema.AtmosConfiguration,
) (modelVendor, error) {
	width := initialModelWidth()
	p := progress.New(
		progress.WithGradient(theme.GetSpinnerColor(), theme.GetSuccessColor()),
		progress.WithWidth(progressBarWidthFor(width)),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = theme.GetCurrentStyles().Spinner
	fps.Apply(&s)

	if len(packages) == 0 {
		return modelVendor{done: true}, nil
	}

	return modelVendor{
		packages:    packages,
		spinner:     s,
		progress:    p,
		dryRun:      dryRun,
		atmosConfig: atmosConfig,
		isTTY:       term.IsTTYSupportForStdout(),
		width:       width,
	}, nil
}

// initialModelWidth returns the real terminal width to use for the live status
// line, so View() can truncate/pad against it from the very first frame,
// before bubbletea's own asynchronous WindowSizeMsg (sent by a background
// goroutine, see tea.Program.checkResize) has had a chance to arrive.
//
// The program's output (see executeVendorModel) is wrapped in a masking
// io.Writer (for secret masking); maskedWriter.Fd() (pkg/io/streams.go)
// forwards to the real underlying file descriptor so bubbletea's own TTY/size
// detection still works through it. But a PTY that hasn't been given a
// window size yet (e.g. one opened via pty.Start without Setsize, as the CLI
// acceptance test harness's simulateTtyCommand does) reports 0x0 to both this
// direct query and bubbletea's own -- silently disabling all width-based
// truncation/padding in View() and letting long lines (e.g. a mixin's full
// source URL) overflow the real terminal, wrap, and corrupt the single-line
// spinner redraw (each frame appends a new scrollback line instead of
// updating in place). FallbackModelWidth covers that case here; the
// WindowSizeMsg handler in Update must apply the same fallback (by ignoring
// a non-positive reported size rather than adopting it) since it can
// otherwise still overwrite this width once its own message arrives.
func initialModelWidth() int {
	defer perf.Track(nil, "exec.initialModelWidth")()

	width := terminal.New().Width(terminal.Stdout)
	if width <= 0 {
		return fallbackModelWidth
	}
	if width > maxWidth {
		width = maxWidth
	}
	return width
}

func (m *modelVendor) Init() tea.Cmd {
	if len(m.packages) == 0 {
		m.done = true
		return nil
	}
	return tea.Batch(ExecuteInstall(m.packages[0], install.InstallOptions{DryRun: m.dryRun}, m.atmosConfig), m.spinner.Tick)
}

func (m *modelVendor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer perf.Track(nil, "exec.Update")()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// A zero/negative width means the terminal genuinely doesn't have a size yet (e.g. a
		// freshly opened PTY that hasn't been resized, as in the CLI acceptance test harness's
		// simulateTtyCommand): keep the fallback initialModelWidth already picked rather than
		// stomping it down to 0, which would wipe out every truncation-sensitive render for the
		// run's whole lifetime (see initialModelWidth's and fallbackModelWidth's doc comments).
		if msg.Width > 0 {
			m.width = msg.Width
			if m.width > maxWidth {
				m.width = maxWidth
			}
			m.progress.Width = progressBarWidthFor(m.width)
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}

	case tea.KeyMsg:
		if cmd := m.handleKeyPress(msg); cmd != nil {
			return m, cmd
		}

	case installedPkgMsg:
		return m.handleInstalledPkgMsg(&msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		newModel, cmd := m.progress.Update(msg)
		if newModel, ok := newModel.(progress.Model); ok {
			m.progress = newModel
		}
		return m, cmd
	}
	return m, nil
}

func (m *modelVendor) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		return tea.Quit
	}
	return nil
}

func (m *modelVendor) handleInstalledPkgMsg(msg *installedPkgMsg) (tea.Model, tea.Cmd) {
	// ensure index is within bounds
	if m.index >= len(m.packages) {
		return m, nil
	}
	pkg := m.packages[m.index]

	mark := checkMark
	errMsg := ""
	if msg.err != nil {
		errMsg = fmt.Sprintf("Failed to vendor %s: error : %s", pkg.Name, msg.err)
		if !m.isTTY {
			ui.Error(errMsg)
		}
		mark = xMark
		m.failedPkg++
		if pkg.IsMixin() {
			m.failedMixins++
		}
		m.failedPkgNames = append(m.failedPkgNames, pkg.Name)
	}
	version := ""
	if pkg.Version != "" {
		version = fmt.Sprintf("(%s)", pkg.Version)
	}
	if m.index >= len(m.packages)-1 {
		// Everything's been installed. We're done!
		m.done = true
		m.logNonNTYFinalStatus(pkg, msg.err != nil)
		version := grayColor.Render(version)
		return m, tea.Sequence(
			tea.Printf("%s %s %s %s", mark, pkg.Name, version, errMsg),
			tea.Quit,
		)
	}
	if !m.isTTY {
		if msg.err != nil {
			ui.Errorf(pkgStatusFmt, pkg.Name, version)
		} else {
			ui.Successf(pkgStatusFmt, pkg.Name, version)
		}
	}
	m.index++
	// Update progress bar
	progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.packages)))

	version = grayColor.Render(version)
	return m, tea.Batch(
		progressCmd,
		tea.Printf("%s %s %s %s", mark, pkg.Name, version, errMsg),                                   // print message above our program
		ExecuteInstall(m.packages[m.index], install.InstallOptions{DryRun: m.dryRun}, m.atmosConfig), // download the next package
	)
}

// componentCount returns how many entries in m.packages are genuine top-level
// components, excluding mixins. Mixins are appended into the same packages
// slice as their owning component (see the vendorComponentSpec.Mixins
// handling in vendor_component_utils.go), so len(m.packages) over-counts
// "components" whenever any component declares mixins. Excluding them here
// keeps "Vendored N components" consistent with what `vendor update` already
// reported as "Updated N component(s)".
func (m *modelVendor) componentCount() int {
	count := 0
	for _, pkg := range m.packages {
		if pkg.IsMixin() {
			continue
		}
		count++
	}
	return count
}

// mixinCount returns how many entries in m.packages are mixins rather than
// genuine top-level components.
func (m *modelVendor) mixinCount() int {
	return len(m.packages) - m.componentCount()
}

// failedComponentCount returns how many of the failed packages tracked by
// m.failedPkg are genuine top-level components, excluding mixin failures
// (tracked separately in m.failedMixins). This keeps the "vendored" and
// "failed" halves of the completion summary using the same definition of
// "component" that componentCount uses.
func (m *modelVendor) failedComponentCount() int {
	return m.failedPkg - m.failedMixins
}

func (m *modelVendor) logNonNTYFinalStatus(pkg install.VendorPackage, failed bool) {
	if m.isTTY {
		return
	}

	m.logPackageStatusLine(pkg, failed)
	m.logComponentSummary()
}

// logPackageStatusLine logs pkg's individual completion status line (and the dry-run notice, when
// applicable) for the non-TTY final-status output.
func (m *modelVendor) logPackageStatusLine(pkg install.VendorPackage, failed bool) {
	version := ""
	if pkg.Version != "" {
		version = fmt.Sprintf("(%s)", pkg.Version)
	}

	if failed {
		ui.Errorf(pkgStatusFmt, pkg.Name, version)
	} else {
		ui.Successf(pkgStatusFmt, pkg.Name, version)
	}

	if m.dryRun {
		ui.Info("Done! Dry run completed. No components vendored")
	}
}

// logComponentSummary logs the aggregate vendored/failed/mixin component counts for the non-TTY
// final-status output.
func (m *modelVendor) logComponentSummary() {
	componentTotal := m.componentCount()
	switch {
	case m.failedPkg > 0:
		failedComponents := m.failedComponentCount()
		switch {
		case failedComponents > 0 && m.failedMixins > 0:
			ui.Errorf("Vendored components (success: %d, failed: %d, mixins failed: %d)", componentTotal-failedComponents, failedComponents, m.failedMixins)
		case failedComponents > 0:
			ui.Errorf("Vendored components (success: %d, failed: %d)", componentTotal-failedComponents, failedComponents)
		default:
			// Only mixins failed: components-only counts stay honest ("failed: 0" would
			// otherwise be indistinguishable from full success), but the failure itself must
			// still be surfaced -- vendorFailureError still fails the command for this case.
			ui.Errorf("Vendored components (success: %d, mixins failed: %d)", componentTotal, m.failedMixins)
		}
	case m.mixinCount() > 0:
		ui.Successf("Vendored components (success: %d, mixins: %d)", componentTotal, m.mixinCount())
	default:
		ui.Successf("Vendored components (success: %d)", componentTotal)
	}
}

func (m *modelVendor) View() string {
	defer perf.Track(nil, "exec.View")()

	n := len(m.packages)
	w := lipgloss.Width(fmt.Sprintf("%d", n))
	if m.done {
		if m.dryRun {
			return doneStyle.Render("Done! Dry run completed. No components vendored.\n")
		}
		componentTotal := m.componentCount()
		if m.failedPkg > 0 {
			failedComponents := m.failedComponentCount()
			switch {
			case failedComponents > 0 && m.failedMixins > 0:
				return doneStyle.Render(fmt.Sprintf("Vendored %d components. Failed to vendor %d components and %d mixins.\n", componentTotal-failedComponents, failedComponents, m.failedMixins))
			case failedComponents > 0:
				return doneStyle.Render(fmt.Sprintf("Vendored %d components. Failed to vendor %d components.\n", componentTotal-failedComponents, failedComponents))
			default:
				// Only mixins failed: the components-only count stays honest, but the failure
				// must still be surfaced rather than reading as "Failed to vendor 0 components."
				// (indistinguishable from full success).
				return doneStyle.Render(fmt.Sprintf("Vendored %d components. Failed to vendor %d mixins.\n", componentTotal, m.failedMixins))
			}
		}
		if mixins := m.mixinCount(); mixins > 0 {
			return doneStyle.Render(fmt.Sprintf("Vendored %d components (%d mixins).\n", componentTotal, mixins))
		}
		return doneStyle.Render(fmt.Sprintf("Vendored %d components.\n", componentTotal))
	}

	pkgCount := fmt.Sprintf(" %*d/%*d", w, m.index, w, n)
	spin := m.spinner.View() + " "
	prog := m.progress.View()
	// effectiveWidth reserves liveLineMargin trailing columns so the rendered line never touches
	// the terminal's true last column (see liveLineMargin's doc comment).
	effectiveWidth := max(0, m.width-liveLineMargin)
	cellsAvail := max(0, effectiveWidth-lipgloss.Width(spin+prog+pkgCount))
	if m.index >= len(m.packages) {
		return ""
	}
	pkgName := currentPkgNameStyle.Render(m.packages[m.index].Name)

	// Truncate (never wrap) the "Pulling <name>" segment to cellsAvail. A
	// mixin's name is its full source URL (100+ chars, one unbroken token with
	// no spaces), so lipgloss's own Style.MaxWidth -- which word-wraps once a
	// Style.Width is also set, and is a no-op when cellsAvail is 0 -- can't be
	// relied on here: truncate.StringWithTail guarantees a single line that
	// never exceeds cellsAvail, appending an ellipsis when content is cut.
	info := truncate.StringWithTail("Pulling "+pkgName, uint(cellsAvail), ellipsis) //nolint:gosec // cellsAvail is clamped to >= 0 above.

	cellsRemaining := max(0, effectiveWidth-lipgloss.Width(spin+info+prog+pkgCount))
	gap := strings.Repeat(" ", cellsRemaining)

	return spin + info + gap + prog + pkgCount
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ExecuteInstall is the thin tea.Cmd translating install.Install's plain (install.Result, error)
// return into this package's own installedPkgMsg tea.Msg.
func ExecuteInstall(pkg install.VendorPackage, opts install.InstallOptions, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	defer perf.Track(atmosConfig, "exec.ExecuteInstall")()

	return func() tea.Msg {
		result, err := install.Install(atmosConfig, pkg, opts)
		if err != nil {
			return installedPkgMsg{err: err, name: pkg.Name}
		}
		return installedPkgMsg{err: result.Err, name: result.Name}
	}
}
