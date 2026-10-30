package starlark

import (
	"errors"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

// raiseFailure preserves the standard fail() argument and value formatting rules.
func raiseFailure(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	sep := " "
	if err := starlark.UnpackArgs("fail", nil, kwargs, "sep?", &sep); err != nil {
		return nil, err
	}
	_, err := starlark.Universe["fail"].(*starlark.Builtin).CallInternal(thread, args, kwargs)
	return nil, failWith(errUtils.ErrScriptRaised, err, "%s", strings.TrimPrefix(err.Error(), "fail: "))
}

// raised reports only the outer classification. A mixed aggregate containing one
// deliberate rejection and one runtime error must still retain runtime diagnostics.
func raised(err error) bool {
	var f *failure
	return errors.As(err, &f) && errors.Is(f.kind, errUtils.ErrScriptRaised)
}

func failureKind(err error) error {
	if raised(err) {
		return errUtils.ErrScriptRaised
	}
	return errUtils.ErrStarlark
}

func combinedFailureKind(errs []error) error {
	for _, err := range errs {
		if !raised(err) {
			return errUtils.ErrStarlark
		}
	}
	return errUtils.ErrScriptRaised
}
