// Package convert shares Starlark value conversion and argument diagnostics
// between the runtime and independent standard-library bindings.
package convert

import (
	"fmt"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

type argumentError struct {
	message string
	cause   error
}

func (e *argumentError) Error() string {
	defer perf.Track(nil, "convert.argumentError.Error")()
	return e.message
}

func (e *argumentError) Unwrap() error {
	defer perf.Track(nil, "convert.argumentError.Unwrap")()
	return e.cause
}

func (e *argumentError) Is(target error) bool {
	defer perf.Track(nil, "convert.argumentError.Is")()
	return target == errUtils.ErrStarlarkInvalidArgument || target == errUtils.ErrStarlark
}

// InvalidArgument classifies an argument error without duplicating sentinel text
// in the eventual script traceback.
func InvalidArgument(format string, args ...any) error {
	defer perf.Track(nil, "convert.InvalidArgument")()
	return ArgumentCause(nil, format, args...)
}

// ArgumentCause additionally preserves an underlying conversion or parse error.
func ArgumentCause(cause error, format string, args ...any) error {
	defer perf.Track(nil, "convert.ArgumentCause")()
	return &argumentError{message: fmt.Sprintf(format, args...), cause: cause}
}

// Sequence copies a Starlark list or tuple, rejecting other value types.
func Sequence(value starlark.Value) ([]starlark.Value, error) {
	defer perf.Track(nil, "convert.Sequence")()
	switch value := value.(type) {
	case starlark.Tuple:
		return append([]starlark.Value{}, value...), nil
	case *starlark.List:
		values := make([]starlark.Value, value.Len())
		for i := range values {
			values[i] = value.Index(i)
		}
		return values, nil
	default:
		return nil, InvalidArgument("expected a list or tuple, got %s", value.Type())
	}
}
