package toolchain

import (
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// batchLineStyle selects which themed ui.* function renders a completed batch item's line.
type batchLineStyle int

const (
	batchLineSuccess batchLineStyle = iota
	batchLineInfo
	batchLineError
)

func (s batchLineStyle) print(line string) {
	switch s {
	case batchLineInfo:
		ui.Info(line)
	case batchLineError:
		ui.Error(line)
	case batchLineSuccess:
		fallthrough
	default:
		ui.Success(line)
	}
}

// liveBatchRenderer live-renders N concurrent workers' status as a small, redrawn region: one
// spinner line per in-flight label plus an overall progress bar, with completed items printed
// as static toast lines (via ui.Success/ui.Info/ui.Error) above the redrawn region as they
// finish. It is the sole writer to the terminal -- workers must never print directly.
//
// Modeled directly on install.go's batchRenderer (the only other place in this package needing
// live concurrent progress), generalized to a plain string label instead of toolInfo since
// callers here don't need per-byte download progress.
type liveBatchRenderer struct {
	spinner       bspinner.Model
	progressBar   progress.Model
	active        []string
	completed     int
	total         int
	renderedLines int
}

func newLiveBatchRenderer(total int) *liveBatchRenderer {
	spinner := bspinner.New()
	spinner.Spinner = bspinner.Dot
	spinner.Style = theme.GetCurrentStyles().Spinner
	return &liveBatchRenderer{
		spinner:     spinner,
		progressBar: progress.New(progress.WithGradient(theme.GetSpinnerColor(), theme.GetSuccessColor())),
		total:       total,
	}
}

func (r *liveBatchRenderer) start(label string) {
	r.clear()
	r.active = append(r.active, label)
	r.render()
}

func (r *liveBatchRenderer) complete(label, line string, style batchLineStyle) {
	r.clear()
	for i, active := range r.active {
		if active == label {
			r.active = append(r.active[:i], r.active[i+1:]...)
			break
		}
	}
	r.completed++
	style.print(line)
	r.render()
}

func (r *liveBatchRenderer) tick() {
	r.clear()
	updated, _ := r.spinner.Update(bspinner.TickMsg{})
	r.spinner = updated
	r.render()
}

func (r *liveBatchRenderer) clear() {
	if r.renderedLines == 0 {
		return
	}
	ui.Writef("\033[%dA", r.renderedLines)
	for i := 0; i < r.renderedLines; i++ {
		ui.Write("\r\033[K")
		if i < r.renderedLines-1 {
			ui.Write("\n")
		}
	}
	if r.renderedLines > 1 {
		ui.Writef("\033[%dA", r.renderedLines-1)
	}
	r.renderedLines = 0
}

func (r *liveBatchRenderer) render() {
	for _, label := range r.active {
		ui.Writef("%s %s\n", r.spinner.View(), label)
		r.renderedLines++
	}
	if len(r.active) > 0 {
		percent := float64(r.completed) / float64(r.total)
		ui.Writef("%s %d/%d complete, %d running\n", r.progressBar.ViewAs(percent), r.completed, r.total, len(r.active))
		r.renderedLines++
	}
}

// liveBatchDisplay wraps liveBatchRenderer with a non-TTY/debug-log fallback, matching
// install.go's batchDisplay: outside a real terminal (or with debug logging enabled, which would
// otherwise interleave with the redrawn region), skip live rendering -- completion lines still
// print via the matching ui.* function, just without the live spinner region.
type liveBatchDisplay struct {
	renderer *liveBatchRenderer
}

func newLiveBatchDisplay(total int) *liveBatchDisplay {
	d := &liveBatchDisplay{}
	if isTTY() && log.GetLevel() > log.DebugLevel {
		d.renderer = newLiveBatchRenderer(total)
	}
	return d
}

func (d *liveBatchDisplay) start(label string) {
	if d.renderer != nil {
		d.renderer.start(label)
	}
}

func (d *liveBatchDisplay) complete(label, line string, style batchLineStyle) {
	if d.renderer != nil {
		d.renderer.complete(label, line, style)
		return
	}
	style.print(line)
}

func (d *liveBatchDisplay) tick() {
	if d.renderer != nil {
		d.renderer.tick()
	}
}

func (d *liveBatchDisplay) clear() {
	if d.renderer != nil {
		d.renderer.clear()
	}
}

type batchProgressEvent[T any] struct {
	index   int
	label   string
	started bool
	result  T
}

// runConcurrentBatchWithLiveProgress runs work for each item with up to maxConcurrency workers
// in flight, live-rendering progress exactly like `atmos toolchain install`'s batch mode
// (spinner per in-flight item + an overall N/M progress bar), instead of silently buffering every
// result and printing them all at once after the whole batch finishes. The work callback must
// not print to the terminal directly. The labelFor callback formats an item for display while
// it's in flight; render maps a completed result to its final display line and which themed
// style prints it.
//
// Results are printed as each item completes (completion order, matching install's own
// convention) -- the returned slice preserves the original items order for callers that need to
// tally outcomes deterministically regardless of completion order.
func runConcurrentBatchWithLiveProgress[I, T any](
	items []I,
	maxConcurrency int,
	labelFor func(item I) string,
	work func(item I) T,
	render func(result T) (line string, style batchLineStyle),
) []T {
	results := make([]T, len(items))
	jobs := make(chan int)
	events := make(chan batchProgressEvent[T], len(items)*2)
	var workers sync.WaitGroup

	for range min(maxConcurrency, len(items)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				label := labelFor(items[i])
				events <- batchProgressEvent[T]{index: i, label: label, started: true}
				result := work(items[i])
				events <- batchProgressEvent[T]{index: i, label: label, result: result}
			}
		}()
	}
	go func() {
		for i := range items {
			jobs <- i
		}
		close(jobs)
		workers.Wait()
		close(events)
	}()

	display := newLiveBatchDisplay(len(items))
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	completed := 0
	for completed < len(items) {
		select {
		case event, ok := <-events:
			if !ok {
				completed = len(items)
				continue
			}
			if event.started {
				display.start(event.label)
				continue
			}
			results[event.index] = event.result
			line, style := render(event.result)
			display.complete(event.label, line, style)
			completed++
		case <-ticker.C:
			display.tick()
		}
	}
	display.clear()

	return results
}
