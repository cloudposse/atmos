package starlark

import (
	"strings"
	"time"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

type task struct {
	name     string
	function starlark.Callable
	args     starlark.Tuple
	kwargs   []starlark.Tuple
	retry    *schema.RetryConfig
	timeout  time.Duration
}

func (t *task) String() string {
	defer perf.Track(nil, "starlark.task.String")()
	return "<task " + t.name + ">"
}

func (t *task) Type() string {
	defer perf.Track(nil, "starlark.task.Type")()
	return "task"
}

func (t *task) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.task.Truth")()
	return true
}

func (t *task) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.task.Hash")()

	return 0, invalidArg("task is unhashable")
}

func (t *task) Freeze() {
	defer perf.Track(nil, "starlark.task.Freeze")()

	t.function.Freeze()
	t.args.Freeze()
	for _, kw := range t.kwargs {
		kw.Freeze()
	}
}

func newTask(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	t := &task{}
	var positional, keywords, retryValue starlark.Value = starlark.None, starlark.None, starlark.None
	var timeout string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"name", &t.name, "function", &t.function, "args?", &positional,
		"kwargs?", &keywords, "retry?", &retryValue, "timeout?", &timeout); err != nil {
		return nil, err
	}
	if strings.TrimSpace(t.name) == "" {
		return nil, invalidArg("task name must not be empty")
	}
	if positional != starlark.None {
		values, err := sequence(positional)
		if err != nil {
			return nil, err
		}
		t.args = starlark.Tuple(values)
	}
	var err error
	t.kwargs, err = taskKeywords(keywords)
	if err != nil {
		return nil, err
	}
	if timeout != "" {
		t.timeout, err = time.ParseDuration(timeout)
		if err != nil || t.timeout <= 0 {
			return nil, invalidArg("timeout must be a positive duration")
		}
	}
	t.retry, err = parseRetry(thread, retryValue)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func taskKeywords(value starlark.Value) ([]starlark.Tuple, error) {
	if value == starlark.None {
		return nil, nil
	}
	d, ok := value.(*starlark.Dict)
	if !ok {
		return nil, invalidArg("kwargs must be a dictionary")
	}
	items := d.Items()
	for _, item := range items {
		if _, ok := starlark.AsString(item[0]); !ok {
			return nil, invalidArg("kwargs keys must be strings")
		}
	}
	return items, nil
}

func parseRetry(thread *starlark.Thread, value starlark.Value) (*schema.RetryConfig, error) {
	if value == starlark.None {
		return nil, nil
	}
	if _, ok := value.(*starlark.Dict); !ok {
		return nil, invalidArg("retry must be a dictionary")
	}
	encoded, err := starlark.Call(thread, starjson.Module.Members["encode"], starlark.Tuple{value}, nil)
	if err != nil {
		return nil, err
	}
	var cfg schema.RetryConfig
	decoder := yaml.NewDecoder(strings.NewReader(string(encoded.(starlark.String))))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, failWith(errUtils.ErrStarlarkInvalidArgument, err, "invalid retry: %s", err)
	}
	if len(cfg.Conditions) > 0 {
		return nil, invalidArg("retry conditions apply to subprocess output, not function tasks")
	}
	switch cfg.BackoffStrategy {
	case "", schema.BackoffConstant, schema.BackoffLinear, schema.BackoffExponential:
	default:
		return nil, invalidArg("unknown retry backoff_strategy %q", cfg.BackoffStrategy)
	}
	if err := retry.Validate(&cfg); err != nil {
		return nil, failWith(errUtils.ErrStarlarkInvalidArgument, err, "invalid retry: %s", err)
	}
	return &cfg, nil
}

func sequence(value starlark.Value) ([]starlark.Value, error) {
	return convert.Sequence(value)
}

// freezeInputs also follows function globals: Function.Freeze alone only freezes
// default arguments and closure cells, leaving module globals mutable.
func freezeInputs(tasks []*task) {
	w := freezer{seen: make(map[starlark.Value]bool)}
	for _, t := range tasks {
		w.visit(t)
	}
}

type freezer struct{ seen map[starlark.Value]bool }

func (w *freezer) visit(value starlark.Value) {
	if value == nil {
		return
	}
	if tuple, ok := value.(starlark.Tuple); ok {
		for _, v := range tuple {
			w.visit(v)
		}
		return
	}
	if w.seen[value] {
		return
	}
	w.seen[value] = true
	value.Freeze()
	w.children(value)
}

func (w *freezer) children(value starlark.Value) {
	switch v := value.(type) {
	case *starlark.Function:
		w.function(v)
	case *starlark.List:
		for i := range v.Len() {
			w.visit(v.Index(i))
		}
	case *starlark.Dict:
		for _, item := range v.Items() {
			w.visit(item)
		}
	case *task:
		w.visit(v.function)
		w.visit(v.args)
		for _, kw := range v.kwargs {
			w.visit(kw)
		}
	}
}

func (w *freezer) function(fn *starlark.Function) {
	for _, global := range fn.Globals() {
		w.visit(global)
	}
	for _, global := range fn.Module().Predeclared() {
		w.visit(global)
	}
	for i := range fn.NumFreeVars() {
		_, free := fn.FreeVar(i)
		w.visit(free)
	}
	for i := range fn.NumParams() {
		w.visit(fn.ParamDefault(i))
	}
}
