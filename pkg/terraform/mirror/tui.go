// Package mirror's TUI renders a package-manager-style progress view for provider
// mirroring (spinner + per-provider×platform checkmark lines), matching the look of
// `atmos vendor pull` (internal/exec/vendor_model.go). Progress is driven by parsing
// the `terraform/tofu providers mirror` output stream so each provider/platform that
// finishes downloading scrolls past as its own line.
package mirror

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/ui/spinner/fps"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	mirrorMaxWidth         = 120
	mirrorProgressBarWidth = 30
	space                  = " "
)

// pkgEvent is one provider/platform package that finished mirroring.
type pkgEvent struct {
	provider string
	platform string
}

// pkgDoneMsg is sent to the program when a provider/platform package is mirrored.
type pkgDoneMsg pkgEvent

// componentDoneMsg is sent when a whole component's mirror invocation finishes.
type componentDoneMsg struct {
	target   Target
	err      error
	mirrored int // number of per-package events the component emitted.
}

// allDoneMsg is sent after the last component completes.
type allDoneMsg struct{}

// mirrorModel is the Bubble Tea model for the provider-mirror progress UI. The
// determinate progress bar advances per component (count known up front); the
// scrolling checkmark lines are per provider/platform package.
type mirrorModel struct {
	isTTY     bool
	width     int
	spinner   spinner.Model
	progress  progress.Model
	platforms int    // configured platform count, used to shape the progress curve.
	current   string // label of the package currently downloading.
	count     int    // packages (provider×platform) mirrored so far.
	done      bool
	failed    []string
}

// newMirrorModel constructs the model; platforms is the configured platform count
// used to shape the (total-unknown) progress curve.
func newMirrorModel(platforms int, isTTY bool) *mirrorModel {
	s := spinner.New()
	s.Style = theme.GetCurrentStyles().Spinner
	fps.Apply(&s)
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(mirrorProgressBarWidth),
		progress.WithoutPercentage(),
	)
	if platforms < 1 {
		platforms = 1
	}
	return &mirrorModel{isTTY: isTTY, spinner: s, progress: p, platforms: platforms}
}

// Init starts the spinner ticker.
func (m *mirrorModel) Init() tea.Cmd {
	defer perf.Track(nil, "mirror.mirrorModel.Init")()

	return m.spinner.Tick
}

// Update handles program messages.
func (m *mirrorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer perf.Track(nil, "mirror.mirrorModel.Update")()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setWidth(msg.Width)
	case tea.KeyMsg:
		if isQuitKey(msg.String()) {
			return m, tea.Quit
		}
	case pkgDoneMsg:
		return m.handlePkgDone(pkgEvent(msg))
	case componentDoneMsg:
		return m.handleComponentDone(msg)
	case allDoneMsg:
		m.done = true
		return m, tea.Quit
	default:
		return m.updateAnimations(msg)
	}
	return m, nil
}

// setWidth clamps the render width to the maximum.
func (m *mirrorModel) setWidth(w int) {
	m.width = w
	if m.width > mirrorMaxWidth {
		m.width = mirrorMaxWidth
	}
}

// isQuitKey reports whether the key should abort the TUI.
func isQuitKey(key string) bool {
	return key == "ctrl+c" || key == "esc" || key == "q"
}

// updateAnimations advances the spinner and progress bar.
func (m *mirrorModel) updateAnimations(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		newModel, cmd := m.progress.Update(msg)
		if pm, ok := newModel.(progress.Model); ok {
			m.progress = pm
		}
		return m, cmd
	}
	return m, nil
}

// handlePkgDone records a finished provider/platform package, prints its line, and
// advances the progress bar. The package total is unknown until the run ends, so the
// bar follows an asymptotic curve (count / (count + platforms)) that always moves
// forward and approaches — without prematurely hitting — full.
func (m *mirrorModel) handlePkgDone(ev pkgEvent) (tea.Model, tea.Cmd) {
	styles := theme.GetCurrentStyles()
	m.count++
	m.current = ev.provider + space + ev.platform

	percent := float64(m.count) / float64(m.count+m.platforms)
	progressCmd := m.progress.SetPercent(percent)

	label := ev.provider + space + styles.Muted.Render(ev.platform)
	if !m.isTTY {
		log.Info(styles.Checkmark.String() + space + label)
		return m, progressCmd
	}
	return m, tea.Batch(progressCmd, tea.Printf("%s %s", styles.Checkmark, label))
}

// handleComponentDone handles the end of a component's mirror. When the component
// emitted no per-package events (e.g. tofu output format drift, or everything already
// present), it falls back to a single checkmark line for the component so the UI never
// shows nothing.
func (m *mirrorModel) handleComponentDone(msg componentDoneMsg) (tea.Model, tea.Cmd) {
	styles := theme.GetCurrentStyles()
	name := msg.target.Component + " (" + msg.target.Stack + ")"

	if msg.err != nil {
		m.failed = append(m.failed, name)
		if !m.isTTY {
			log.Error(styles.XMark.String() + space + name + " — " + msg.err.Error())
			return m, nil
		}
		return m, tea.Printf("%s %s — %s", styles.XMark, name, msg.err.Error())
	}

	if msg.mirrored == 0 {
		// Fallback: no parsed packages, show the component line.
		if !m.isTTY {
			log.Info(styles.Checkmark.String() + space + name)
			return m, nil
		}
		return m, tea.Printf("%s %s", styles.Checkmark, name)
	}
	return m, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// View renders the spinner + current package + determinate (per-component) progress
// bar with an n/total count, matching the bubbletea package-manager example.
func (m *mirrorModel) View() string {
	defer perf.Track(nil, "mirror.mirrorModel.View")()

	if m.done {
		return ""
	}
	styles := theme.GetCurrentStyles()
	width := maxWidthOr(m.width)

	// Total is unknown mid-run, so show the running count of mirrored packages.
	count := fmt.Sprintf(" %d", m.count)
	spin := m.spinner.View() + space
	prog := m.progress.View()

	label := m.current
	if label == "" {
		label = "providers"
	}
	cellsAvail := maxInt(0, width-lipgloss.Width(spin+prog+count))
	info := lipgloss.NewStyle().MaxWidth(cellsAvail).Render("Mirroring " + styles.PackageName.Render(label))

	cellsRemaining := maxInt(0, width-lipgloss.Width(spin+info+prog+count))
	gap := strings.Repeat(space, cellsRemaining)

	return spin + info + gap + prog + count
}

func maxWidthOr(w int) int {
	if w <= 0 {
		return mirrorMaxWidth
	}
	return w
}

// mirrorScanner is an io.Writer that parses `providers mirror` output and emits a
// pkgEvent each time a provider/platform package finishes downloading.
type mirrorScanner struct {
	buf      []byte
	provider string
	platform string
	count    int
	emit     func(pkgEvent)
}

// newMirrorScanner builds a scanner that calls emit per finished package.
func newMirrorScanner(emit func(pkgEvent)) *mirrorScanner {
	return &mirrorScanner{emit: emit}
}

// Write implements io.Writer, parsing complete lines as they arrive.
func (s *mirrorScanner) Write(p []byte) (int, error) {
	defer perf.Track(nil, "mirror.mirrorScanner.Write")()

	s.buf = append(s.buf, p...)
	for {
		idx := bytes.IndexByte(s.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(s.buf[:idx])
		s.buf = s.buf[idx+1:]
		s.handleLine(line)
	}
	return len(p), nil
}

// handleLine parses one line of mirror output. Matching is on stable substrings so it
// tolerates minor formatting differences between Terraform and OpenTofu versions.
func (s *mirrorScanner) handleLine(line string) {
	t := strings.TrimSpace(strings.TrimRight(line, "\r"))
	switch {
	case strings.HasPrefix(t, "- Mirroring "):
		s.provider = strings.TrimSuffix(strings.TrimPrefix(t, "- Mirroring "), "...")
		s.platform = ""
	case strings.HasPrefix(t, "- Downloading package for "):
		s.platform = strings.TrimSuffix(strings.TrimPrefix(t, "- Downloading package for "), "...")
	case strings.HasPrefix(t, "- Package authenticated"):
		if s.provider != "" && s.platform != "" {
			s.emit(pkgEvent{provider: s.provider, platform: s.platform})
			s.count++
			s.platform = ""
		}
	}
}

// executeMirrorModel runs the streaming mirror TUI. The mirror work runs in a
// goroutine that sends per-package and per-component events to the program; the model
// renders them. Falls back to the no-renderer mode for non-TTY / CI, matching
// executeVendorModel.
func executeMirrorModel(targets []Target, args []string, cacheSetup *tfcache.Setup, platforms int) error {
	if len(targets) == 0 {
		return nil
	}

	isTTY := term.IsTTYSupportForStdout()
	model := newMirrorModel(platforms, isTTY)

	var opts []tea.ProgramOption
	if !isTTY {
		opts = append(opts, tea.WithoutRenderer(), tea.WithInput(nil))
		log.Debug("No TTY detected. Falling back to basic output for mirror TUI.")
	} else if !terminal.HasRealTTYInput() {
		// TTY mode is forced (screenshots, cast recordings): keep the renderer,
		// but don't let bubbletea open /dev/tty for input — there isn't one.
		opts = append(opts, tea.WithInput(nil))
	}
	p := tea.NewProgram(model, opts...)

	// In TTY mode the TUI owns the screen, so suppress mid-render UI writes (e.g. the
	// per-component toolchain "Using ..." line) that would corrupt the sticky spinner.
	// This scopes the suppression to the pkg/io UI stream only — os.Stderr, the logger,
	// and other goroutines' direct stderr writes are untouched. The proxy
	// "listening"/"cert" lines are emitted once before this, by the caller.
	if isTTY {
		restore := io.SuppressUI()
		defer restore()
	}

	go func() {
		for i := range targets {
			t := targets[i]
			scanner := newMirrorScanner(func(ev pkgEvent) { p.Send(pkgDoneMsg(ev)) })
			err := mirrorComponent(t, args, cacheSetup, scanner)
			p.Send(componentDoneMsg{target: t, err: err, mirrored: scanner.count})
		}
		p.Send(allDoneMsg{})
	}()

	if _, err := p.Run(); err != nil {
		return errUtils.Build(errUtils.ErrTUIRun).WithCause(err).Err()
	}

	if len(model.failed) > 0 {
		return errUtils.Build(errUtils.ErrTerraformExecFailed).
			WithExplanationf("Failed to mirror providers for %d of %d component(s): %s",
				len(model.failed), len(targets), strings.Join(model.failed, ", ")).
			Err()
	}
	return nil
}
