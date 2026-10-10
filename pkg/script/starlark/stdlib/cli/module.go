package cli

import (
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

// Parser supplies the host's command handling for the calling interpreter thread.
type Parser func(*starlark.Thread, script.CommandSpec) (script.CommandInput, error)

type commandModule struct {
	parse Parser
	mu    sync.Mutex
	used  bool
}

// New creates one invocation's declarations and command entry point.
func New(parse Parser) starlark.Value {
	defer perf.Track(nil, "cli.New")()

	m := &commandModule{parse: parse}
	return &starlarkstruct.Module{Name: "cli", Members: starlark.StringDict{
		"arg":     starlark.NewBuiltin("cli.arg", argument),
		"flag":    starlark.NewBuiltin("cli.flag", flag),
		"command": starlark.NewBuiltin("cli.command", m.command),
	}}
}

func (m *commandModule) command(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var spec script.CommandSpec
	var run starlark.Callable
	var validate starlark.Value = starlark.None
	var arguments, flagValues starlark.Value = starlark.Tuple{}, starlark.Tuple{}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"run", &run, "name?", &spec.Name, "description?", &spec.Description,
		"args?", &arguments, "flags?", &flagValues, "validate?", &validate); err != nil {
		return nil, err
	}
	if err := validateCommand(spec.Name, validate); err != nil {
		return nil, err
	}
	if err := argumentDeclarations(&spec, arguments); err != nil {
		return nil, err
	}
	if err := flagDeclarations(&spec, flagValues); err != nil {
		return nil, err
	}
	m.mu.Lock()
	used := m.used
	m.used = true
	m.mu.Unlock()
	if used {
		return nil, convert.InvalidArgument("cli.command may be called only once per invocation")
	}
	if m.parse == nil {
		return nil, convert.InvalidArgument("cli.command requires a standalone command parser")
	}
	input, err := m.parse(t, spec)
	if err != nil {
		return nil, err
	}
	if input.Help {
		return starlark.None, nil
	}
	return call(t, input, validate, run)
}

func call(t *starlark.Thread, input script.CommandInput, validate starlark.Value, run starlark.Callable) (starlark.Value, error) {
	parsedArgs, err := convert.Dictionary(input.Args)
	if err != nil {
		return nil, err
	}
	parsedFlags, err := convert.Dictionary(input.Flags)
	if err != nil {
		return nil, err
	}
	callbackArgs := starlark.Tuple{parsedArgs, parsedFlags}
	if validate != starlark.None {
		valid, err := starlark.Call(t, validate, callbackArgs, nil)
		if err != nil {
			return nil, err
		}
		if valid == starlark.False {
			return nil, convert.InvalidArgument("cli.command: input validation failed")
		}
		if valid != starlark.None && valid != starlark.True {
			return nil, convert.InvalidArgument("cli.command: validate must return None or a bool")
		}
	}
	return starlark.Call(t, run, callbackArgs, nil)
}

func argumentDeclarations(spec *script.CommandSpec, arguments starlark.Value) error {
	args, err := convert.Sequence(arguments)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	optional := false
	for _, item := range args {
		d, ok := item.(*declaration)
		if !ok || d.argument == nil {
			return convert.InvalidArgument("cli.command: args must contain cli.arg declarations")
		}
		if names[d.argument.Name] || (optional && d.argument.Required) {
			return convert.InvalidArgument("cli.command: argument names must be unique and required arguments must precede optional arguments")
		}
		names[d.argument.Name] = true
		optional = !d.argument.Required
		spec.Args = append(spec.Args, d.argument)
	}
	return nil
}

func flagDeclarations(spec *script.CommandSpec, flagValues starlark.Value) error {
	values, err := convert.Sequence(flagValues)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	shorthands := map[string]bool{}
	for _, item := range values {
		d, ok := item.(*declaration)
		if !ok || d.flag == nil {
			return convert.InvalidArgument("cli.command: flags must contain cli.flag declarations")
		}
		name, short := d.flag.GetName(), d.flag.GetShorthand()
		if names[name] || (short != "" && shorthands[short]) {
			return convert.InvalidArgument("cli.command: flag names and shorthands must be unique")
		}
		names[name], shorthands[short] = true, true
		spec.Flags = append(spec.Flags, d.flag)
	}
	return nil
}

func validateCommand(name string, validate starlark.Value) error {
	if name != "" && !inputName.MatchString(name) {
		return convert.InvalidArgument("cli.command: invalid name %q", name)
	}
	if validate != starlark.None {
		if _, ok := validate.(starlark.Callable); !ok {
			return convert.InvalidArgument("cli.command: validate must be callable")
		}
	}
	return nil
}
