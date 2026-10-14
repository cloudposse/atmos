// Package cli exposes native Atmos command definitions to standalone programs.
package cli

import (
	"regexp"
	"strings"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

var inputName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

type declaration struct {
	argument *flags.PositionalArgSpec
	flag     flags.Flag
}

func (d *declaration) String() string {
	defer perf.Track(nil, "cli.declaration.String")()

	if d.argument != nil {
		return "cli.arg(" + d.argument.Name + ")"
	}
	return "cli.flag(" + d.flag.GetName() + ")"
}

func (d *declaration) Type() string {
	defer perf.Track(nil, "cli.declaration.Type")()

	if d.argument != nil {
		return "cli.arg"
	}
	return "cli.flag"
}

func (*declaration) Freeze() {
	defer perf.Track(nil, "cli.declaration.Freeze")()
}

func (*declaration) Truth() starlark.Bool {
	defer perf.Track(nil, "cli.declaration.Truth")()

	return starlark.True
}

func (d *declaration) Hash() (uint32, error) {
	defer perf.Track(nil, "cli.declaration.Hash")()

	return 0, convert.InvalidArgument("unhashable type: %s", d.Type())
}

func argument(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	spec := &flags.PositionalArgSpec{Required: true}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"name", &spec.Name, "description?", &spec.Description, "required?", &spec.Required); err != nil {
		return nil, err
	}
	if !inputName.MatchString(spec.Name) {
		return nil, convert.InvalidArgument("cli.arg: invalid name %q", spec.Name)
	}
	return &declaration{argument: spec}, nil
}

type flagOptions struct {
	name, kind, shorthand, description, env string
	required                                bool
	value, choices                          starlark.Value
}

func flag(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	o := flagOptions{kind: "string", value: starlark.None, choices: starlark.Tuple{}}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"name", &o.name, "type?", &o.kind, "default?", &o.value,
		"shorthand?", &o.shorthand, "description?", &o.description,
		"required?", &o.required, "choices?", &o.choices, "env?", &o.env); err != nil {
		return nil, err
	}
	if !inputName.MatchString(o.name) || strings.EqualFold(o.name, "help") {
		return nil, convert.InvalidArgument("cli.flag: invalid or reserved name %q", o.name)
	}
	if !validShorthand(o.shorthand) {
		return nil, convert.InvalidArgument("cli.flag: shorthand must be one letter other than h")
	}
	if o.required && o.value != starlark.None {
		return nil, convert.InvalidArgument("cli.flag: required flags cannot have a default")
	}
	native, err := o.native()
	if err != nil {
		return nil, err
	}
	return &declaration{flag: native}, nil
}

func validShorthand(short string) bool {
	return short == "" || (len(short) == 1 && inputName.MatchString(short) && short != "h")
}

func (o *flagOptions) native() (flags.Flag, error) {
	choices, err := stringSequence(o.choices)
	if err != nil {
		return nil, err
	}
	if !o.acceptsChoices(choices) {
		return nil, convert.InvalidArgument("cli.flag: choices require type string or string_list")
	}
	var env []string
	if o.env != "" {
		env = []string{o.env}
	}
	switch o.kind {
	case "string":
		return o.stringFlag(env, choices)
	case "int":
		return o.intFlag(env)
	case "bool":
		return o.boolFlag(env)
	case "string_list":
		return o.listFlag(env, choices)
	default:
		return nil, convert.InvalidArgument("cli.flag: unsupported type %q (use string, int, bool, or string_list)", o.kind)
	}
}

func (o *flagOptions) acceptsChoices(choices []string) bool {
	return len(choices) == 0 || o.kind == "string" || o.kind == "string_list"
}

func (o *flagOptions) stringFlag(env, choices []string) (flags.Flag, error) {
	value, err := o.stringDefault()
	if err != nil {
		return nil, err
	}
	return &flags.StringFlag{
		Name: o.name, Shorthand: o.shorthand, Description: o.description,
		Default: value, Required: o.required, EnvVars: env, ValidValues: choices,
	}, nil
}

func (o *flagOptions) intFlag(env []string) (flags.Flag, error) {
	value := 0
	if o.value != starlark.None {
		if err := starlark.AsInt(o.value, &value); err != nil {
			return nil, convert.ArgumentCause(err, "cli.flag: default for %q must fit an integer", o.name)
		}
	}
	return &flags.IntFlag{
		Name: o.name, Shorthand: o.shorthand, Description: o.description,
		Default: value, Required: o.required, EnvVars: env,
	}, nil
}

func (o *flagOptions) boolFlag(env []string) (flags.Flag, error) {
	value, err := o.boolDefault()
	if err != nil {
		return nil, err
	}
	return &flags.BoolFlag{
		Name: o.name, Shorthand: o.shorthand, Description: o.description,
		Default: bool(value), EnvVars: env,
	}, nil
}

func (o *flagOptions) listFlag(env, choices []string) (flags.Flag, error) {
	var value []string
	if o.value != starlark.None {
		var err error
		value, err = stringSequence(o.value)
		if err != nil {
			return nil, err
		}
	}
	return &flags.StringSliceFlag{
		Name: o.name, Shorthand: o.shorthand, Description: o.description,
		Default: value, Required: o.required, EnvVars: env, ValidValues: choices,
	}, nil
}

func (o *flagOptions) stringDefault() (string, error) {
	if o.value == starlark.None {
		return "", nil
	}
	value, ok := starlark.AsString(o.value)
	if !ok {
		return "", convert.InvalidArgument("cli.flag: default for %q must be a string", o.name)
	}
	return value, nil
}

func (o *flagOptions) boolDefault() (starlark.Bool, error) {
	if o.required {
		return false, convert.InvalidArgument("cli.flag: boolean flags use defaults and cannot be required")
	}
	if o.value == starlark.None {
		return starlark.False, nil
	}
	value, ok := o.value.(starlark.Bool)
	if !ok {
		return false, convert.InvalidArgument("cli.flag: default for %q must be a bool", o.name)
	}
	return value, nil
}

func stringSequence(value starlark.Value) ([]string, error) {
	items, err := convert.Sequence(value)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(items))
	for i, item := range items {
		text, ok := starlark.AsString(item)
		if !ok {
			return nil, convert.InvalidArgument("cli.flag: expected strings, got %s", item.Type())
		}
		result[i] = text
	}
	return result, nil
}
