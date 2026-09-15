// Package ordered runs bounded preparation concurrently and commits in input order.
package ordered

import (
	"context"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Options supplies the isolated preparation and ordered commit operations.
// Commit receives preparation errors, allowing callers to report and continue.
type Options[T any] struct {
	Count       int
	Concurrency int
	Barrier     func(int) bool
	Prepare     func(context.Context, int) (T, error)
	Commit      func(int, T, error) error
	Dispose     func(T)
}

type result[T any] struct {
	value T
	err   error
}

type runner[T any] struct {
	ctx     context.Context
	opts    Options[T]
	jobs    chan int
	results []chan result[T]
	workers sync.WaitGroup
	window  int
}

// Run prepares at most Concurrency jobs at once and retains at most twice that
// many uncommitted results. A barrier is prepared only after its predecessors commit.
// Cancellation joins workers and disposes every uncommitted result before returning.
func Run[T any](ctx context.Context, opts Options[T]) error {
	defer perf.Track(nil, "ordered.Run")()
	if opts.Concurrency < 1 {
		return errUtils.ErrInvalidFlagValue
	}
	width := min(opts.Concurrency, opts.Count)
	if width == 0 {
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(ctx)
	batch := newRunner(ctx, opts, width)
	defer func() {
		cancel()
		batch.close()
	}()
	next := 0
	for commit := 0; commit < opts.Count; commit++ {
		if err := batch.ctx.Err(); err != nil {
			return err
		}
		next = batch.schedule(next, commit)
		if err := batch.commit(commit); err != nil {
			return err
		}
	}
	return batch.ctx.Err()
}

func newRunner[T any](ctx context.Context, opts Options[T], width int) *runner[T] {
	batch := &runner[T]{ctx: ctx, opts: opts, jobs: make(chan int, width*2), results: make([]chan result[T], opts.Count), window: width * 2}
	if opts.Concurrency == 1 {
		batch.window = 1
	}
	for i := range batch.results {
		batch.results[i] = make(chan result[T], 1)
	}
	for range width {
		batch.workers.Add(1)
		go batch.prepare()
	}
	return batch
}

func (r *runner[T]) prepare() {
	defer r.workers.Done()
	for i := range r.jobs {
		if r.ctx.Err() != nil {
			continue
		}
		value, err := r.opts.Prepare(r.ctx, i)
		r.results[i] <- result[T]{value, err}
	}
}

func (r *runner[T]) schedule(next, commit int) int {
	for next < r.opts.Count && next < commit+r.window {
		if r.opts.Barrier != nil && r.opts.Barrier(next) && next != commit {
			break
		}
		r.jobs <- next
		next++
	}
	return next
}

func (r *runner[T]) commit(index int) error {
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case prepared := <-r.results[index]:
		defer r.dispose(prepared.value)
		if err := r.ctx.Err(); err != nil {
			return err
		}
		return r.opts.Commit(index, prepared.value, prepared.err)
	}
}

func (r *runner[T]) dispose(value T) {
	if r.opts.Dispose != nil {
		r.opts.Dispose(value)
	}
}

func (r *runner[T]) close() {
	close(r.jobs)
	r.workers.Wait()
	for _, results := range r.results {
		select {
		case prepared := <-results:
			r.dispose(prepared.value)
		default:
		}
	}
}
