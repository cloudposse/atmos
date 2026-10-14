package cli

import (
	"fmt"
	"strings"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	errUtils "github.com/cloudposse/atmos/errors"
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
			return nil, validationFailure(input)
		}
		if valid != starlark.None && valid != starlark.True {
			return nil, convert.InvalidArgument("cli.command: validate must return None or a bool")
		}
	}
	return starlark.Call(t, run, callbackArgs, nil)
}

// validationFailure reports a validate callback that returned False. The user's input was rejected,
// so it is a usage error with the usage line and a --help hint, not a script crash with a traceback.
func validationFailure(input script.CommandInput) error {
	cause := fmt.Errorf("%w: input validation failed", errUtils.ErrScriptUsage)
	if input.Usage == nil {
		return &script.UsageFailure{Err: cause}
	}
	return &script.UsageFailure{Err: input.Usage(cause)}
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
		// Parsed values are keyed case-insensitively, so names differing only by case collide.
		key := strings.ToLower(d.argument.Name)
		if names[key] || (optional && d.argument.Required) {
			return convert.InvalidArgument("cli.command: argument names must be unique (ignoring case) and required arguments must precede optional arguments")
		}
		names[key] = true
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
	// Viper keys are case-insensitive, so flags named `Stage` and `stage` would share one value.
	// `help` is reserved for the generated help flag.
	names := map[string]bool{"help": true}
	shorthands := map[string]bool{}
	for _, item := range values {
		d, ok := item.(*declaration)
		if !ok || d.flag == nil {
			return convert.InvalidArgument("cli.command: flags must contain cli.flag declarations")
		}
		name, short := d.flag.GetName(), d.flag.GetShorthand()
		key := strings.ToLower(name)
		if names[key] || (short != "" && shorthands[short]) {
			return convert.InvalidArgument("cli.command: flag names (ignoring case) and shorthands must be unique, got %q", name)
		}
		names[key], shorthands[short] = true, true
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
