// Package convert shares Starlark value conversion and argument diagnostics
// between the runtime and independent standard-library bindings.
package convert

import (
	"fmt"
	"maps"
	"slices"

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

// Dictionary copies native command inputs into an immutable Starlark dictionary
// with sorted keys, keeping iteration and printed output deterministic.
func Dictionary(values map[string]any) (*starlark.Dict, error) {
	defer perf.Track(nil, "convert.Dictionary")()

	result := starlark.NewDict(len(values))
	for _, key := range slices.Sorted(maps.Keys(values)) {
		raw := values[key]
		var value starlark.Value
		switch v := raw.(type) {
		case nil:
			value = starlark.None
		case string:
			value = starlark.String(v)
		case int:
			value = starlark.MakeInt(v)
		case bool:
			value = starlark.Bool(v)
		case []string:
			items := make([]starlark.Value, len(v))
			for i, item := range v {
				items[i] = starlark.String(item)
			}
			value = starlark.NewList(items)
		default:
			return nil, InvalidArgument("unsupported input type %T for %q", raw, key)
		}
		_ = result.SetKey(starlark.String(key), value)
	}
	result.Freeze()
	return result, nil
}
