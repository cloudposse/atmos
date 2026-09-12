// Package testreport renders test execution using the shared Atmos tree geometry.
package testreport

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/ui/tree"
)

// Test case lifecycle states shared by the executor and renderer.
const (
	Passed        = "passed"
	Failed        = "failed"
	Skipped       = "skipped"
	Canceled      = "canceled"
	Pending       = "pending"
	Running       = "running"
	defaultWidth  = 80
	staticWidth   = 120
	progressWidth = 24
	newline       = "\n"
)

// Node is a group or an individual expanded test case.
type Node struct {
	ID, Name, Status string
	Duration         time.Duration
	Children         []*Node
}

// Reporter owns the display for one test group; updates may arrive concurrently.
type Reporter struct {
	mu      sync.Mutex
	title   string
	roots   []*Node
	nodes   map[string]*Node
	output  io.Writer
	program *tea.Program
	done    chan error
	cancel  context.CancelFunc
	err     error

	startedAt  time.Time
	finishedAt time.Time
}

// New constructs a reporter. The execution tree is fixed before any tests run.
func New(title string, roots []*Node, output io.Writer) *Reporter {
	r := &Reporter{title: title, roots: roots, nodes: map[string]*Node{}, output: output, startedAt: time.Now()}
	var visit func([]*Node)
	visit = func(nodes []*Node) {
		for _, n := range nodes {
			r.nodes[n.ID] = n
			visit(n.Children)
		}
	}
	visit(roots)
	return r
}

// Start enables live rendering only when the caller owns an interactive terminal.
func (r *Reporter) Start(live bool, cancel context.CancelFunc) {
	r.cancel = cancel
	if !live {
		return
	}
	m := model{report: r, width: defaultWidth, spinner: spinner.New(spinner.WithSpinner(spinner.Dot))}
	r.program = tea.NewProgram(&m, tea.WithOutput(r.output), tea.WithInput(nil), tea.WithoutSignalHandler())
	r.done = make(chan error, 1)
	go func() {
		_, err := r.program.Run()
		if err != nil {
			cancel()
		}
		r.done <- err
	}()
}

// Update changes a case state. Output blocks are published once on completion.
func (r *Reporter) Update(id, status string, duration time.Duration, logs string) {
	r.mu.Lock()
	if n := r.nodes[id]; n != nil {
		if status != "" {
			n.Status = status
		}
		n.Duration = duration
	}
	block := ""
	if logs != "" {
		block = r.failureBlock(id, status, logs)
	}
	r.mu.Unlock()
	// Never hold the model lock while sending to Bubble Tea: View takes the same lock.
	if r.program != nil {
		if block != "" {
			r.program.Println(block)
		}
		r.program.Send(refreshMsg{})
	} else if block != "" {
		r.mu.Lock()
		_, err := fmt.Fprintln(r.output, block)
		if err != nil {
			r.err = err
		}
		r.mu.Unlock()
	}
}

// failureBlock labels and masks a completed case or suite setup failure for permanent output.
func (r *Reporter) failureBlock(id, status, logs string) string {
	label := id
	if n := r.nodes[id]; n != nil {
		label = n.Name
	}
	var locate func([]*Node, []string) []string
	locate = func(nodes []*Node, path []string) []string {
		for _, n := range nodes {
			next := append(append([]string{}, path...), n.Name)
			if n.ID == id {
				return next
			}
			if found := locate(n.Children, next); found != nil {
				return found
			}
		}
		return nil
	}
	heading := strings.Join(locate(r.roots, nil), " / ")
	if heading == "" {
		heading = id
	}
	if id == "" {
		heading = r.title
		label = "Setup"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "     %s\n", theme.GetCurrentStyles().Body.Bold(true).Render(iolib.MaskString(heading)))
	fmt.Fprintf(&b, "  %s  %s%s\n", r.symbol(status, ""), theme.GetCurrentStyles().Muted.Render("└── "), iolib.MaskString(label))
	for _, line := range strings.Split(strings.TrimRight(iolib.MaskString(logs), newline), newline) {
		fmt.Fprintf(&b, "             %s\n", ansi.Strip(line))
	}
	return strings.TrimRight(b.String(), newline)
}

// Finish leaves a permanent tree and summary in scrollback.
func (r *Reporter) Finish() error {
	// Freeze wall-clock duration before terminal cleanup, including concurrent tests only once.
	r.mu.Lock()
	if r.finishedAt.IsZero() {
		r.finishedAt = time.Now()
	}
	r.mu.Unlock()
	if r.program != nil {
		r.program.Send(finishMsg{})
		if err := <-r.done; err != nil {
			return err
		}
	} else {
		_, err := io.WriteString(r.output, r.View(staticWidth, "", true))
		if err != nil {
			return err
		}
	}
	return r.err
}

// Counts returns terminal leaf counts, never counting group nodes twice.
func (r *Reporter) Counts() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts()
}

// counts aggregates leaf states while the caller holds the reporter lock.
func (r *Reporter) counts() map[string]int {
	c := map[string]int{Passed: 0, Failed: 0, Skipped: 0, Canceled: 0, "total": 0}
	for _, n := range r.nodes {
		if len(n.Children) == 0 {
			c["total"]++
			c[n.Status]++
		}
	}
	return c
}

// View builds a connected tree and bottom progress bar.
func (r *Reporter) View(width int, spinning string, final bool) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	styles := theme.GetCurrentStyles()
	fmt.Fprintf(&b, "     %s\n", styles.Body.Bold(true).Render(iolib.MaskString(r.title)))
	var render func([]*Node, tree.Path)
	render = func(nodes []*Node, path tree.Path) {
		for i, n := range nodes {
			p := append(append(tree.Path{}, path...), i == len(nodes)-1)
			label := styles.Body.Render(iolib.MaskString(n.Name))
			if n.Duration > 0 {
				label += styles.Muted.Render(fmt.Sprintf(" (%.1fs)", n.Duration.Seconds()))
			}
			status := nodeStatus(n)
			if final || spinning == "" {
				label += styles.Muted.Render(" [" + status + "]")
			}
			prefix := "  " + r.symbol(status, spinning) + "  " + styles.Muted.Render(tree.Connector(p))
			fmt.Fprintln(&b, prefix+ansi.Truncate(label, max(8, width-lipgloss.Width(prefix)-1), "…"))
			render(n.Children, p)
		}
	}
	render(r.roots, nil)
	c := r.counts()
	complete := c[Passed] + c[Failed] + c[Skipped] + c[Canceled]
	fmt.Fprintln(&b)
	if !final {
		bar := progress.New(
			progress.WithDefaultGradient(),
			progress.WithWidth(max(4, min(progressWidth, width-20))),
			progress.WithColorProfile(ui.GetColorProfile()),
		)
		fraction := 0.0
		if c["total"] > 0 {
			fraction = float64(complete) / float64(c["total"])
		}
		fmt.Fprintf(&b, "     %s\n", bar.ViewAs(fraction))
	}
	end := r.finishedAt
	if end.IsZero() {
		end = time.Now()
	}
	fmt.Fprintf(&b, "     %s%s%s%s\n",
		styles.Body.Bold(true).Render(fmt.Sprintf("%d/%d", complete, c["total"])),
		styles.Muted.Render(" · "), renderCounts(c),
		styles.Muted.Render(fmt.Sprintf(" · %.1fs elapsed", end.Sub(r.startedAt).Seconds())))
	return b.String()
}

// renderCounts emphasizes nonzero outcomes while keeping empty categories quiet.
func renderCounts(counts map[string]int) string {
	styles := theme.GetCurrentStyles()
	parts := make([]string, 0, 4)
	for _, status := range []string{Passed, Failed, Skipped, Canceled} {
		style := styles.Muted
		if counts[status] > 0 {
			switch status {
			case Passed:
				style = styles.Success
			case Failed:
				style = styles.Error
			}
		}
		parts = append(parts, style.Render(fmt.Sprintf("%d %s", counts[status], status)))
	}
	return strings.Join(parts, styles.Muted.Render(", "))
}

// nodeStatus derives group status from its children while preserving explicit group failures.
func nodeStatus(n *Node) string {
	if len(n.Children) == 0 || n.Status == Failed {
		if n.Status == "" {
			return Pending
		}
		return n.Status
	}
	states := map[string]bool{}
	for _, child := range n.Children {
		states[nodeStatus(child)] = true
	}
	for _, status := range []string{Failed, Running, Pending, Canceled, Passed, Skipped} {
		if states[status] {
			return status
		}
	}
	return Pending
}

// symbol selects the themed outcome marker or current spinner frame.
func (r *Reporter) symbol(status, spin string) string {
	switch status {
	case Passed:
		return theme.GetCurrentStyles().Success.Render("●")
	case Failed:
		return theme.GetCurrentStyles().Error.Render("●")
	case Running:
		if spin != "" {
			return strings.TrimSpace(spin)
		}
		return "◌"
	default:
		return theme.GetCurrentStyles().Muted.Render("○")
	}
}

type (
	refreshMsg struct{}
	finishMsg  struct{}
	model      struct {
		report  *Reporter
		width   int
		spinner spinner.Model
		done    bool
	}
)

// Init starts the spinner that refreshes the live suite view.
func (m *model) Init() tea.Cmd { return m.spinner.Tick }

// Update handles resizing, cancellation, and publishing the complete final tree to scrollback.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case finishMsg:
		m.done = true
		// Print the full result tree into scrollback. Rendering it as the last live
		// frame would crop suites taller than the terminal's viewport.
		final := strings.TrimRight(m.report.View(m.width, "", true), newline)
		return m, tea.Sequence(tea.Println(final), tea.Quit)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.report.cancel()
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View hides the live frame after its final results have been printed.
func (m *model) View() string {
	if m.done {
		return ""
	}
	return m.report.View(m.width, m.spinner.View(), false)
}
