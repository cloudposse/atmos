package starlark

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/scheduler"
)

// TaskEvent describes one branch's lifecycle. Attempt is one-based for attempts;
// status is running, attempt, succeeded, failed, or skipped.
type TaskEvent struct {
	Name    string
	Status  string
	Attempt int
	Err     error
}

func (s *session) event(event TaskEvent) {
	if s.engine.observe != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.engine.observe(event)
	}
}

func (s *session) parallel(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var functions, descriptors starlark.Value = starlark.None, starlark.None
	maxConcurrency, failFast := 4, false
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "functions?", &functions, "tasks?", &descriptors,
		"max_concurrency?", &maxConcurrency, "fail_fast?", &failFast); err != nil {
		return nil, err
	}
	if maxConcurrency <= 0 {
		return nil, invalidArg("max_concurrency must be positive")
	}
	tasks, err := parallelTasks(functions, descriptors)
	if err != nil {
		return nil, err
	}
	if err := threadContext(thread).Err(); err != nil {
		return nil, err
	}
	freezeInputs(tasks)
	return s.runParallel(threadContext(thread), threadPrefix(thread), tasks, maxConcurrency, failFast)
}

func parallelTasks(functions, descriptors starlark.Value) ([]*task, error) {
	if (functions == starlark.None) == (descriptors == starlark.None) {
		return nil, invalidArg("provide exactly one of functions or tasks")
	}
	input := descriptors
	if functions != starlark.None {
		input = functions
	}
	values, err := sequence(input)
	if err != nil {
		return nil, err
	}
	tasks := make([]*task, len(values))
	names := make(map[string]bool)
	for i, value := range values {
		if functions != starlark.None {
			fn, ok := value.(starlark.Callable)
			if !ok {
				return nil, invalidArg("functions[%d] must be callable", i)
			}
			tasks[i] = &task{name: fmt.Sprintf("%s[%d]", fn.Name(), i), function: fn}
		} else {
			t, ok := value.(*task)
			if !ok {
				return nil, invalidArg("tasks[%d] must be a steps.task", i)
			}
			tasks[i] = t
		}
		if names[tasks[i].name] {
			return nil, invalidArg("duplicate task name %q", tasks[i].name)
		}
		names[tasks[i].name] = true
	}
	return tasks, nil
}

func (s *session) runParallel(ctx context.Context, prefix string, tasks []*task, concurrency int, failFast bool) (starlark.Value, error) {
	graph := dependency.NewGraph()
	for i, t := range tasks {
		if err := graph.AddNode(&dependency.Node{ID: fmt.Sprintf("%09d", i), Metadata: map[string]any{"task": t}}); err != nil {
			return nil, err
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	dispatcher := scheduler.DispatcherFunc(func(ctx context.Context, node *dependency.Node) (scheduler.Result, error) {
		t := node.Metadata["task"].(*task)
		value, err := s.runTask(ctx, taskPrefix(prefix, t, len(tasks)), t)
		if err != nil && failFast {
			cancel()
		}
		return scheduler.Result{Value: value}, err
	})
	aggregate := scheduler.New(
		graph, dispatcher,
		scheduler.WithMaxConcurrency(concurrency), scheduler.WithFailFast(failFast),
		scheduler.WithNodeCompleteHook(func(node *dependency.Node, result scheduler.Result) {
			s.event(TaskEvent{Name: node.Metadata["task"].(*task).name, Status: string(result.Status), Err: result.Err})
		}),
	).Run(runCtx)
	for i := range aggregate.Results {
		result := &aggregate.Results[i]
		if result.Status == scheduler.StatusSkipped {
			s.event(TaskEvent{Name: result.Node.Metadata["task"].(*task).name, Status: string(result.Status), Err: result.Err})
		}
	}
	if err := taskFailures(ctx, aggregate); err != nil {
		return nil, err
	}
	values := make([]starlark.Value, len(tasks))
	for i := range aggregate.Results {
		values[i] = aggregate.Results[i].Value.(starlark.Value)
	}
	return starlark.NewList(values), nil
}

// taskPrefix labels each task's lines when siblings run side by side; a lone task inherits
// the enclosing prefix so nested groups stay attributable.
func taskPrefix(parent string, t *task, count int) string {
	if count > 1 {
		return parent + "[" + t.name + "] "
	}
	return parent
}

// taskFailures reduces scheduler results to one readable message per failed task.
// Tasks canceled only because fail-fast stopped the group are omitted when a real failure exists.
func taskFailures(ctx context.Context, aggregate *scheduler.AggregateResult) error {
	var err error
	if failed := failedTaskErrors(ctx, aggregate); len(failed) > 0 {
		err = combineFailures(failed)
	} else if aggregate.Err != nil {
		err = aggregate.Err
	}
	if err == nil {
		return ctx.Err()
	}
	return withContext(ctx, err)
}

func failedTaskErrors(ctx context.Context, aggregate *scheduler.AggregateResult) []error {
	var failed, canceled []error
	for i := range aggregate.Results {
		result := &aggregate.Results[i]
		switch {
		case result.Status != scheduler.StatusFailed || result.Err == nil:
		case ctx.Err() == nil && errors.Is(result.Err, context.Canceled):
			canceled = append(canceled, result.Err)
		default:
			failed = append(failed, result.Err)
		}
	}
	if len(failed) == 0 {
		return canceled
	}
	return failed
}

// combineFailures keeps a single failure intact and merges several into one message,
// preserving every explanation and the error chains of each task.
func combineFailures(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	messages := make([]string, len(errs))
	details := make([]string, 0, len(errs))
	for i, err := range errs {
		messages[i] = err.Error()
		if detail := joinDetails(err); detail != "" {
			details = append(details, detail)
		}
	}
	return &failure{
		kind: errUtils.ErrStarlark, msg: fmt.Sprintf("%d tasks failed: %s", len(errs), strings.Join(messages, "; ")),
		causes: errs, detail: strings.Join(details, "\n\n"),
	}
}

func (s *session) runTask(ctx context.Context, prefix string, t *task) (starlark.Value, error) {
	parent := ctx
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}
	s.event(TaskEvent{Name: t.name, Status: "running"})
	cfg := retry.DefaultConfig()
	if t.retry != nil {
		cfg = *t.retry
	}
	var value starlark.Value
	var last error
	attempt := 0
	err := retry.New(cfg, retry.WithClock(s.engine.clock)).ExecuteWithPredicate(ctx, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt++
		s.event(TaskEvent{Name: t.name, Status: "attempt", Attempt: attempt})
		var err error
		value, err = s.attempt(ctx, prefix, t)
		last = err
		return err
	}, func(err error) bool {
		return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
	})
	if err != nil {
		return nil, taskError(parent, ctx, t, taskOutcome{err: err, last: last, attempts: attempt}, s.spec.ProjectRoot)
	}
	value.Freeze()
	return value, nil
}

// attempt runs one invocation of the task function on its own thread and output sink.
func (s *session) attempt(ctx context.Context, prefix string, t *task) (starlark.Value, error) {
	out := s.newTaskOutput(prefix)
	thread, stop := s.thread(ctx, t.name, out)
	defer func() {
		stop()
		out.flush()
	}()
	value, err := starlark.Call(thread, t.function, t.args, t.kwargs)
	if err != nil {
		return nil, withContext(ctx, err)
	}
	return value, ctx.Err()
}

// taskOutcome is what the retry loop concluded for one task.
type taskOutcome struct {
	err      error // Final error reported by the retry executor.
	last     error // Error of the last attempt, without retry wrapping.
	attempts int
}

// taskError builds the single message for one failed task: name plus the innermost script
// message, with backtraces in the explanation instead of recursively nested prose.
func taskError(parent, ctx context.Context, t *task, outcome taskOutcome, projectRoot string) error {
	if t.timeout > 0 && parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		failed := failWithAll(errUtils.ErrStarlarkTaskTimeout, []error{context.DeadlineExceeded, outcome.last}, "task %q timed out after %s", t.name, t.timeout)
		return withDetail(failed, taskDetail(t, outcome.last, projectRoot))
	}
	if outcome.last == nil || !errors.Is(outcome.err, outcome.last) {
		return failWith(errUtils.ErrStarlark, outcome.err, "task %q: %s", t.name, displayPaths(projectRoot, outcome.err.Error()))
	}
	message := fmt.Sprintf("task %q: %s", t.name, displayPaths(projectRoot, evalSummary(outcome.last)))
	if outcome.attempts > 1 {
		message += fmt.Sprintf(" (after %d attempts)", outcome.attempts)
	}
	kind := errUtils.ErrStarlark
	if isRecursionError(outcome.last) {
		kind = errUtils.ErrStarlarkRecursionLimit
	}
	failed := failWith(kind, outcome.last, "%s", message)
	return withDetail(failed, taskDetail(t, outcome.last, projectRoot))
}

func taskDetail(t *task, err error, projectRoot string) string {
	trace := evalDetail(err, projectRoot)
	if trace == "" {
		return ""
	}
	return fmt.Sprintf("Traceback for task %q:\n%s", t.name, trace)
}
