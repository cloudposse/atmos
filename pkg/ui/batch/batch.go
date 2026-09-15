// Package batch renders progress for concurrent operations. Scheduling belongs to callers.
package batch

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/spf13/viper"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	eventBufferSize  = 64
	spinnerInterval  = 80 * time.Millisecond
	defaultWidth     = 80
	defaultHeight    = 24
	maxProgressWidth = 30
	bytesPerUnit     = 1024
)

// Event describes a job's current state. IDs, unlike labels, must be unique within a run.
type Event struct {
	// Warning is a permanent diagnostic and does not count as a job.
	Warning string
	// Reset starts a new phase, clearing counts and active jobs.
	Reset bool
	ID    int
	Count int
	Label string
	// Version is displayed as a muted parenthesized suffix after the label.
	Version           string
	Phase             string
	Downloaded, Total int64
	// Bytes marks coalescible byte-only updates. Terminal events are never dropped.
	Bytes   bool
	Done    bool
	Outcome string
	Err     error
	Attempt int
}

// Observer receives events serially from Run, even when producers are concurrent.
type Observer func(Event)

// Renderer owns an inline region. All methods must be called by its owner goroutine.
type Renderer struct {
	spinner                 spinner.Model
	bar                     progress.Model
	active                  map[int]Event
	order                   []int
	completed, total, lines int
	live                    bool
	toolchainStyle          bool
	size                    func() (int, int, error)
}

// Option customizes the renderer without coupling it to job scheduling.
type Option func(*Renderer)

// WithToolchainStyle preserves the toolchain running-count footer.
func WithToolchainStyle() Option {
	return func(r *Renderer) { r.toolchainStyle = true }
}

// New creates a renderer using the existing Atmos terminal theme.
func New(total int, live bool, options ...Option) *Renderer {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	terminalInfo := terminal.New()
	r := &Renderer{spinner: s, bar: progress.New(progress.WithGradient(theme.GetSpinnerColor(), theme.GetSuccessColor())), active: map[int]Event{}, total: total, live: live, size: func() (int, int, error) {
		return terminalInfo.Width(terminal.Stderr), terminalInfo.Height(terminal.Stderr), nil
	}}
	for _, option := range options {
		option(r)
	}
	return r
}

func supportsLiveProgress() bool {
	settings := viper.New()
	settings.MustBindEnv("terminal", "TERM")
	kind := settings.GetString("terminal")
	return terminal.New().IsTTY(terminal.Stderr) && kind != "dumb" && kind != "unknown" && log.GetLevel() > log.DebugLevel
}

// Run owns rendering while work emits progress. It drains events before returning.
func Run(total int, work func(Observer) error) error {
	defer perf.Track(nil, "batch.Run")()
	r := New(total, supportsLiveProgress())
	defer r.Clear()
	events := make(chan Event, eventBufferSize)
	finished := make(chan error, 1)
	go func() {
		err := work(func(e Event) {
			if e.Bytes {
				select {
				case events <- e:
				default:
				}
				return
			}
			events <- e
		})
		close(events)
		finished <- err
	}()
	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return <-finished
			}
			r.Update(&e)
		case <-ticker.C:
			r.Tick()
		}
	}
}

// Update consumes a non-nil transition without allowing worker output into the live region.
// It snapshots the event immediately; callers retain ownership of their event.
func (r *Renderer) Update(event *Event) {
	e := *event
	if e.Warning != "" {
		r.Clear()
		ui.Warning(e.Warning)
		r.render()
		return
	}
	if e.Reset {
		r.Clear()
		r.active = make(map[int]Event)
		r.order = nil
		r.completed, r.total = 0, e.Count
		return
	}
	if e.Count > 0 {
		r.total = e.Count
	}
	if e.Bytes {
		if old, ok := r.active[e.ID]; ok {
			old.Downloaded, old.Total = e.Downloaded, e.Total
			r.active[e.ID] = old
		}
		return
	}
	if e.Done {
		r.Complete(e.ID, e.printResult)
		return
	}
	r.Clear()
	if _, ok := r.active[e.ID]; !ok {
		r.order = append(r.order, e.ID)
	}
	r.active[e.ID] = e
	r.render()
}

func (e *Event) printResult() {
	// Append styled suffixes after markdown rendering so ANSI escapes remain intact.
	suffix := e.versionSuffix()
	switch {
	case e.Err != nil:
		ui.Writef("%s%s: %s\n", ui.FormatError(e.Label), suffix, ui.FormatInline(e.Err.Error()))
	case e.Outcome == "unchanged", e.Outcome == "skipped", e.Outcome == "canceled":
		ui.Writef("%s %s%s (%s)\n", theme.GetCurrentStyles().Info.Render(theme.IconInfo), ui.FormatInline(e.Label), suffix, e.Outcome)
	case e.Outcome != "" && e.Outcome != "installed":
		ui.Writef("%s%s (%s)\n", ui.FormatSuccess(e.Label), suffix, e.Outcome)
	default:
		ui.Writef("%s%s\n", ui.FormatSuccess(e.Label), suffix)
	}
}

func (e *Event) versionSuffix() string {
	if e.Version == "" {
		return ""
	}
	return " " + theme.GetCurrentStyles().VersionNumber.Render("("+e.Version+")")
}

// activeLabel keeps vendoring stages in a muted suffix and preserves toolchain rows.
func (r *Renderer) activeLabel(e *Event) string {
	if r.toolchainStyle {
		label := strings.TrimSpace(e.Phase + " " + e.Label)
		if e.Attempt > 0 {
			label += fmt.Sprintf(" (attempt %d)", e.Attempt)
		}
		return label
	}
	suffix := ""
	if e.Version != "" {
		suffix = " (" + e.Version + ")"
	}
	if stage := e.stageLabel(); stage != "" {
		suffix += " · " + stage
	}
	label := theme.GetCurrentStyles().PackageName.Render(e.Label) + theme.GetCurrentStyles().VersionNumber.Render(suffix)
	return label
}

func (e *Event) stageLabel() string {
	stage := strings.ToLower(e.Phase)
	if stage == "pulling" {
		stage = "downloading"
	}
	if stage == "downloading" && e.Total > 0 {
		const completePercent = 100
		percent := int(min(float64(completePercent), max(0, float64(e.Downloaded)/float64(e.Total)*completePercent)))
		stage += fmt.Sprintf(" %d%%", percent)
	}
	if e.Attempt > 0 {
		stage += fmt.Sprintf(" · attempt %d", e.Attempt)
	}
	return stage
}

// Complete replaces an active row with caller-formatted permanent output.
// The callback executes synchronously on the renderer's owner goroutine.
func (r *Renderer) Complete(id int, output func()) {
	r.Clear()
	delete(r.active, id)
	for i, activeID := range r.order {
		if activeID == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	r.completed++
	output()
	r.render()
}

// Tick advances the spinner without animation commands that race model updates.
func (r *Renderer) Tick() {
	if !r.live {
		return
	}
	r.Clear()
	r.spinner, _ = r.spinner.Update(spinner.TickMsg{})
	r.render()
}

// Clear removes the live region; completed lines remain in scrollback.
func (r *Renderer) Clear() {
	if r.lines == 0 {
		return
	}
	ui.Writef("\033[%dA\r\033[J", r.lines)
	r.lines = 0
}

func (r *Renderer) render() {
	if !r.live || len(r.active) == 0 {
		return
	}
	width, height := r.dimensions()
	width = max(1, width-1)
	limit := max(0, height-3)
	for _, id := range r.order[:min(len(r.order), limit)] {
		e := r.active[id]
		left := fmt.Sprintf("%s %s", r.spinner.View(), r.activeLabel(&e))
		left = truncate.StringWithTail(left, uint(width), "…")
		ui.Writef("%s\n", AlignProgress(left, e.Downloaded, e.Total, width))
		r.lines++
	}
	if len(r.order) > limit && height > 2 {
		ui.Writef("%s\n", truncate.StringWithTail(fmt.Sprintf("… %d more active", len(r.order)-limit), uint(width), "…"))
		r.lines++
	}
	r.bar.Width = min(maxProgressWidth, max(1, width/3))
	summary := fmt.Sprintf("%s %d/%d complete, %d active, %d queued", r.bar.ViewAs(float64(r.completed)/float64(max(1, r.total))), r.completed, r.total, len(r.active), max(0, r.total-r.completed-len(r.active)))
	if r.toolchainStyle {
		summary = fmt.Sprintf("%s %d/%d complete, %d running", r.bar.ViewAs(float64(r.completed)/float64(max(1, r.total))), r.completed, r.total, len(r.active))
	}
	ui.Writef("%s\n", truncate.StringWithTail(summary, uint(width), "…"))
	r.lines++
}

func (r *Renderer) dimensions() (int, int) {
	width, height, err := r.size()
	if err != nil {
		width, height = defaultWidth, defaultHeight
	}
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}

	return width, height
}

// AlignProgress right-aligns real byte progress, omitting it when the label needs the width.
func AlignProgress(left string, downloaded, total int64, width int) string {
	if downloaded == 0 && total <= 0 {
		return left
	}
	right := fileSize(downloaded)
	if total > 0 {
		right = fmt.Sprintf("%s/%s", right, fileSize(total))
	}
	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 1 {
		return left
	}
	return left + strings.Repeat(" ", padding) + right
}

func fileSize(n int64) string {
	if n < bytesPerUnit {
		return fmt.Sprintf("%d B", n)
	}
	value, unit := float64(n), "B"
	for _, next := range []string{"KB", "MB", "GB", "TB"} {
		value /= bytesPerUnit
		unit = next
		if value < bytesPerUnit {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}
