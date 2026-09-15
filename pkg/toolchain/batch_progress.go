package toolchain

import (
	"sync"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/batch"
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
	display   *batch.Renderer
	active    []string
	ids       []int
	completed int
}

func newLiveBatchRenderer(total int) *liveBatchRenderer {
	return &liveBatchRenderer{display: batch.New(total, true, batch.WithToolchainStyle())}
}

func (r *liveBatchRenderer) start(label string) {
	id := r.completed + len(r.active)
	r.active = append(r.active, label)
	r.ids = append(r.ids, id)
	r.display.Update(&batch.Event{ID: id, Label: label})
}

func (r *liveBatchRenderer) complete(label, line string, style batchLineStyle) {
	for i, active := range r.active {
		if active == label {
			id := r.ids[i]
			r.active = append(r.active[:i], r.active[i+1:]...)
			r.ids = append(r.ids[:i], r.ids[i+1:]...)
			r.completed++
			r.display.Complete(id, func() { style.print(line) })
			return
		}
	}
	r.completed++
	r.display.Complete(r.completed+len(r.active)-1, func() { style.print(line) })
}

func (r *liveBatchRenderer) tick()   { r.display.Tick() }
func (r *liveBatchRenderer) clear()  { r.display.Clear() }
func (r *liveBatchRenderer) render() { r.display.Tick() }

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
