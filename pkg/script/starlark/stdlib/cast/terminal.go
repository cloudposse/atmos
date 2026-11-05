package cast

import (
	"slices"
	"strings"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/castplan"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

type terminal struct {
	rec   *recorder
	open  bool
	index int
}

func (v *terminal) String() string {
	defer perf.Track(nil, "cast.terminal.String")()

	return "cast.terminal"
}

func (v *terminal) Type() string {
	defer perf.Track(nil, "cast.terminal.Type")()

	return "cast.terminal"
}

func (v *terminal) Freeze() {
	defer perf.Track(nil, "cast.terminal.Freeze")()

	v.rec.Freeze()
}

func (v *terminal) Truth() starlark.Bool {
	defer perf.Track(nil, "cast.terminal.Truth")()

	return starlark.True
}

func (v *terminal) Hash() (uint32, error) {
	defer perf.Track(nil, "cast.terminal.Hash")()

	return 0, convert.InvalidArgument("cast.terminal is not hashable")
}

func (v *terminal) AttrNames() []string {
	defer perf.Track(nil, "cast.terminal.AttrNames")()

	return []string{"close", "hide", "key", "pause", "screenshot", "send", "show", "wait", "write"}
}

func (v *terminal) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "cast.terminal.Attr")()

	if !slices.Contains(v.AttrNames(), name) {
		return nil, nil
	}
	return starlark.NewBuiltin("rec.terminal."+name, func(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := v.rec.mutable(); err != nil {
			return nil, err
		}
		if name == "close" {
			if len(args) != 0 || len(kwargs) != 0 {
				return nil, convert.InvalidArgument("terminal.close takes no arguments")
			}
			v.open = false
			return starlark.None, nil
		}
		action, err := actionFields(t, name, args, kwargs)
		if err != nil {
			return nil, err
		}
		if !v.open {
			v.index = len(v.rec.plan.Actions)
			v.rec.plan.Actions = append(v.rec.plan.Actions, castplan.Action{Kind: "session", Fields: map[string]any{"working_directory": v.rec.dir}})
			v.open = true
		}
		session := &v.rec.plan.Actions[v.index]
		if name == "send" {
			if stringField(action.Fields, "text") == "" {
				return nil, convert.InvalidArgument("terminal.send requires text")
			}
			action.Kind = "write"
			session.Children = append(session.Children, action, castplan.Action{Kind: "key", Fields: map[string]any{"key": "enter"}})
		} else {
			session.Children = append(session.Children, action)
		}
		return starlark.None, nil
	}), nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
