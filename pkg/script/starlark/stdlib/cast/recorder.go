package cast

import (
	"slices"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/castplan"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

const hiddenAction = "hidden"

type recorder struct {
	host     Host
	plan     *castplan.Plan
	policy   starlark.Callable
	dir      string
	terminal *terminal
	frozen   bool
	finished bool
	group    bool
}

func (r *recorder) String() string {
	defer perf.Track(nil, "cast.recorder.String")()

	return "cast.recorder"
}

func (r *recorder) Type() string {
	defer perf.Track(nil, "cast.recorder.Type")()

	return "cast.recorder"
}

func (r *recorder) Freeze() {
	defer perf.Track(nil, "cast.recorder.Freeze")()

	r.frozen = true
}

func (r *recorder) Truth() starlark.Bool {
	defer perf.Track(nil, "cast.recorder.Truth")()

	return starlark.True
}

func (r *recorder) Hash() (uint32, error) {
	defer perf.Track(nil, "cast.recorder.Hash")()

	return 0, convert.InvalidArgument("cast.recorder is not hashable")
}

func (r *recorder) AttrNames() []string {
	defer perf.Track(nil, "cast.recorder.AttrNames")()

	return []string{"cd", "cleanup", "comment", "generate", hiddenAction, "pause", "prompt", "run", "screenshot", "setup", simulateAction, "step", "terminal", "validate"}
}

func (r *recorder) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "cast.recorder.Attr")()

	if !slices.Contains(r.AttrNames(), name) {
		return nil, nil
	}
	if name == "terminal" {
		if r.terminal == nil {
			r.terminal = &terminal{rec: r}
		}
		return r.terminal, nil
	}
	return starlark.NewBuiltin("rec."+name, func(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return r.call(t, name, args, kwargs)
	}), nil
}

func (r *recorder) call(t *starlark.Thread, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := r.mutable(); err != nil {
		return nil, err
	}
	if name == "generate" || name == "validate" {
		return r.finish(t, name, args, kwargs)
	}
	if r.terminal != nil && r.terminal.open {
		return nil, convert.InvalidArgument("close rec.terminal before rec.%s", name)
	}
	if slices.Contains([]string{"setup", hiddenAction, "cleanup"}, name) {
		return r.addGroup(t, name, args, kwargs)
	}
	action, err := actionFields(t, name, args, kwargs)
	if err != nil {
		return nil, err
	}
	if err := r.add(&action); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (r *recorder) mutable() error {
	if r.frozen || r.finished {
		return convert.InvalidArgument("cannot modify a frozen or finalized cast recorder")
	}
	return nil
}

func (r *recorder) add(action *castplan.Action) error {
	switch action.Kind {
	case "comment":
		text := stringField(action.Fields, "text")
		if text == "" {
			return convert.InvalidArgument("rec.comment requires text")
		}
		action.Kind, action.Fields["text"] = simulateAction, commentText(text)
	case "prompt":
		action.Kind, action.Fields["mode"] = simulateAction, "prompt"
	case "cd":
		path := stringField(action.Fields, "path")
		if path == "" {
			return convert.InvalidArgument("rec.cd requires a path")
		}
		// Single quotes prevent displayed shell metacharacters changing the meaning of the transition.
		action.Kind = simulateAction
		action.Fields = map[string]any{"text": "cd " + shellQuote(path)}
		r.dir = relativeDirectory(r.dir, path)
	}
	if _, exists := action.Fields["working_directory"]; !exists && r.dir != "" {
		action.Fields["working_directory"] = r.dir
	}
	r.plan.Actions = append(r.plan.Actions, *action)
	return nil
}

func (r *recorder) addGroup(t *starlark.Thread, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.group && name != hiddenAction {
		return nil, convert.InvalidArgument("setup and cleanup belong on the root recorder")
	}
	var fn starlark.Callable
	if err := starlark.UnpackArgs("rec."+name, args, kwargs, "fn", &fn); err != nil {
		return nil, err
	}
	group := &recorder{host: r.host, plan: &castplan.Plan{Configuration: r.plan.Configuration}, dir: r.dir, group: true}
	_, err := starlark.Call(t, fn, starlark.Tuple{group}, nil)
	group.frozen = true
	if err != nil {
		return nil, err
	}
	switch name {
	case "setup":
		r.plan.Setup = append(r.plan.Setup, group.plan.Actions...)
	case "cleanup":
		r.plan.Cleanup = append(r.plan.Cleanup, group.plan.Actions...)
	case hiddenAction:
		r.plan.Actions = append(r.plan.Actions, castplan.Action{Kind: hiddenAction, Children: group.plan.Actions})
	}
	return starlark.None, nil
}

func (r *recorder) finish(t *starlark.Thread, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.group {
		return nil, convert.InvalidArgument("only the root recorder can generate or validate")
	}
	if len(args) != 0 || len(kwargs) != 0 {
		return nil, convert.InvalidArgument("rec.%s takes no arguments", name)
	}
	if r.host == nil {
		return nil, convert.InvalidArgument("the host has not provided cast services")
	}
	if err := r.host.Check(t, r.plan); err != nil {
		return nil, err
	}
	r.finished = true
	var path string
	var err error
	if name == "generate" {
		path, err = r.host.Generate(t, r.plan, r.policy)
	} else {
		path, err = r.host.Validate(t, r.plan, r.policy)
	}
	if err != nil {
		return nil, err
	}
	return starlark.String(path), nil
}
