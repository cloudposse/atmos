package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/scheduler"
)

// ExecutionOptions supplies dependencies and streams without mutating process state.
type ExecutionOptions struct {
	Executable     string
	Directory      string
	GlobalArgs     []string
	Environment    []string
	Streams        process.Streams
	Runner         process.Runner
	Concurrency    int
	KeepGoing      bool
	Grouped        bool
	ResourceGuards map[string]func(*dependency.Node) func()
}

// Execute runs a prevalidated plan with the shared ready-queue scheduler.
func Execute(ctx context.Context, plan *Plan, opts *ExecutionOptions) *scheduler.AggregateResult {
	defer perf.Track(nil, "lifecycle.Execute")()

	if opts == nil || plan == nil || plan.Graph == nil || opts.Concurrency < 1 || opts.Runner == nil {
		return &scheduler.AggregateResult{Err: fmt.Errorf("%w: invalid lifecycle execution options", errUtils.ErrInvalidConfig)}
	}
	d := &dispatcher{plan: plan, opts: opts, logs: make(map[string]*groupedLog)}
	defer d.cleanup()
	result := scheduler.New(
		plan.Graph, d,
		scheduler.WithMaxConcurrency(opts.Concurrency),
		scheduler.WithFailFast(!opts.KeepGoing),
	).Run(ctx)
	if opts.Grouped {
		result.Err = errors.Join(result.Err, d.flush(result))
	}
	return result
}

type dispatcher struct {
	plan *Plan
	opts *ExecutionOptions
	mu   sync.Mutex
	logs map[string]*groupedLog
}

// groupedLog buffers one child's output per stream, so a later flush can route
// stdout to the data channel and stderr to the UI channel.
type groupedLog struct {
	stdout, stderr *os.File
}

func (g *groupedLog) close() {
	for _, f := range []*os.File{g.stdout, g.stderr} {
		if f == nil {
			continue
		}
		_ = f.Close()
		// The path is owned by os.CreateTemp, never component input.
		_ = os.Remove(f.Name())
	}
}

func (d *dispatcher) Dispatch(ctx context.Context, node *dependency.Node) (scheduler.Result, error) {
	defer perf.Track(nil, "lifecycle.dispatcher.Dispatch")()
	result := scheduler.Result{NodeID: node.ID, Node: *node, Status: scheduler.StatusSucceeded}
	if guard := d.opts.ResourceGuards[node.Type]; guard != nil {
		unlock := guard(node)
		defer unlock()
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	streams, err := d.streams(node)
	if err != nil {
		return result, err
	}
	args := append(slices.Clone(d.opts.GlobalArgs), d.plan.Commands[node.ID]...)
	env := childEnvironment(d.opts.Environment)
	if handler := d.plan.Handlers[node.Type]; handler != nil && handler.WorkerEnvironment != nil {
		env = overrideEnvironment(env, handler.WorkerEnvironment(d.opts.Concurrency))
	}
	processResult := d.opts.Runner.Run(ctx, process.TaskSpec{
		Command: d.opts.Executable, Args: args, Dir: d.opts.Directory,
		Env: env, Streams: streams,
	})
	result.Value = processResult.ExitCode
	if !processResult.Success() {
		failure := fmt.Errorf("%w: %s (exit %d)", errUtils.ErrComponentExecutionFailed, Label(node), processResult.ExitCode)
		if exitedNonZero(&processResult) {
			return result, failure
		}
		return result, errors.Join(failure, processResult.Err)
	}
	return result, nil
}

// exitedNonZero reports a child that ran to completion and returned a plain
// non-zero status. Its wait error only restates the exit code, so the dispatcher
// omits it. Start failures, cancellations, and signals keep their error.
func exitedNonZero(r *process.Result) bool {
	return r.Started && r.ExitCode > 0 && !r.Canceled && !r.Signaled
}

func childEnvironment(base []string) []string {
	// Selectors belong to the parent. Inheriting them can recurse into bulk
	// execution or incorrectly filter a prerequisite explicitly added by closure.
	removed := map[string]bool{
		"ATMOS_AFFECTED": true, "ATMOS_ALL": true, "ATMOS_COMPONENTS": true,
		"ATMOS_QUERY": true, "ATMOS_TAGS": true, "ATMOS_LABELS": true,
		"ATMOS_INCLUDE_DEPENDENCIES": true, "ATMOS_INCLUDE_DEPENDENTS": true,
		"ATMOS_STACK_DEPLOY_AFFECTED": true, "ATMOS_STACK_DESTROY_AFFECTED": true,
		// The parent consumed dry-run; inheriting it would silently dry-run children
		// after the parent reported them as executed.
		"ATMOS_DRY_RUN": true,
	}
	result := make([]string, 0, len(base)+1)
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		bulk := strings.HasPrefix(key, "ATMOS_TERRAFORM_") && (strings.HasSuffix(key, "_ALL") || strings.HasSuffix(key, "_AFFECTED") || strings.HasSuffix(key, "_COMPONENTS") || strings.HasSuffix(key, "_INCLUDE_DEPENDENCIES") || strings.HasSuffix(key, "_INCLUDE_DEPENDENTS"))
		if !removed[key] && !bulk {
			result = append(result, item)
		}
	}
	return result
}

func overrideEnvironment(base, overrides []string) []string {
	replaced := make(map[string]bool, len(overrides))
	for _, value := range overrides {
		key, _, _ := strings.Cut(value, "=")
		replaced[key] = true
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, value := range base {
		key, _, _ := strings.Cut(value, "=")
		if !replaced[key] {
			result = append(result, value)
		}
	}
	return append(result, overrides...)
}

func (d *dispatcher) streams(node *dependency.Node) (process.Streams, error) {
	streams := d.opts.Streams
	// With one worker, output is already grouped. Keep live streams so native
	// confirmation prompts remain visible and retain terminal detection.
	if d.opts.Concurrency == 1 {
		return streams, nil
	}
	if d.opts.Concurrency > 1 {
		streams.Stdin = nil
	}
	if d.opts.Grouped {
		log, err := newGroupedLog()
		if err != nil {
			return streams, err
		}
		d.mu.Lock()
		d.logs[node.ID] = log
		d.mu.Unlock()
		streams.Stdout = &lockedWriter{mu: &d.mu, output: log.stdout}
		streams.Stderr = &lockedWriter{mu: &d.mu, output: log.stderr}
		return streams, nil
	}
	prefix := "[" + Label(node) + "] "
	streams.Stdout = &lockedWriter{mu: &d.mu, output: streams.Stdout, prefix: prefix, lineStart: true}
	streams.Stderr = &lockedWriter{mu: &d.mu, output: streams.Stderr, prefix: prefix, lineStart: true}
	return streams, nil
}

func newGroupedLog() (*groupedLog, error) {
	log := &groupedLog{}
	var err error
	if log.stdout, err = os.CreateTemp("", "atmos-stack-log-*"); err != nil {
		return nil, err
	}
	if log.stderr, err = os.CreateTemp("", "atmos-stack-log-*"); err != nil {
		log.close()
		return nil, err
	}
	return log, nil
}

// flush replays each component's buffered output once all components finished.
// The header and stderr content go to the UI stream; stdout goes to the data stream.
func (d *dispatcher) flush(result *scheduler.AggregateResult) error {
	var errs []error
	for i := range result.Results {
		r := &result.Results[i]
		log := d.logs[r.NodeID]
		if log == nil {
			continue
		}
		if _, err := io.WriteString(writerOrDiscard(d.opts.Streams.Stderr), "["+Label(&r.Node)+"]\n"); err != nil {
			errs = append(errs, err)
		}
		errs = append(errs, replay(log.stdout, d.opts.Streams.Stdout), replay(log.stderr, d.opts.Streams.Stderr))
	}
	return errors.Join(errs...)
}

func replay(f *os.File, w io.Writer) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(writerOrDiscard(w), f)
	return err
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func (d *dispatcher) cleanup() {
	for _, log := range d.logs {
		log.close()
	}
}

type lockedWriter struct {
	mu        *sync.Mutex
	output    io.Writer
	prefix    string
	lineStart bool
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "lifecycle.lockedWriter.Write")()

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.output == nil {
		return len(p), nil
	}
	if w.prefix == "" {
		return w.output.Write(p)
	}
	written := 0
	for len(p) > 0 {
		if w.lineStart {
			if _, err := io.WriteString(w.output, w.prefix); err != nil {
				return written, err
			}
			w.lineStart = false
		}
		n := strings.IndexByte(string(p), '\n') + 1
		if n == 0 {
			n = len(p)
		}
		count, err := w.output.Write(p[:n])
		written += count
		if err != nil {
			return written, err
		}
		if count != n {
			return written, io.ErrShortWrite
		}
		w.lineStart = p[n-1] == '\n'
		p = p[n:]
	}
	return written, nil
}
