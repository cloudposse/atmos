package starlark

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/script"
)

func hookValues(hook *script.HookContext) (starlark.Value, starlark.Value) {
	if hook == nil {
		return starlark.None, starlark.None
	}
	h := starlarkstruct.FromStringDict(starlark.String("hook"), starlark.StringDict{
		"name": starlark.String(hook.Name), "event": starlark.String(hook.Event),
	})
	var code starlark.Value = starlark.None
	if hook.Operation.ExitCode != nil {
		code = starlark.MakeInt(*hook.Operation.ExitCode)
	}
	op := starlarkstruct.FromStringDict(starlark.String("operation"), starlark.StringDict{
		"command": starlark.String(hook.Operation.Command),
		"status":  optionalString(hook.Operation.Status), "error": optionalString(hook.Operation.Error),
		"exit_code": code, "stdout": optionalString(hook.Operation.Stdout), "stderr": optionalString(hook.Operation.Stderr),
	})
	return h, op
}

func optionalString(value *string) starlark.Value {
	if value == nil {
		return starlark.None
	}
	return starlark.String(*value)
}
