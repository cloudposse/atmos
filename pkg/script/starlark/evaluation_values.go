package starlark

import (
	"fmt"
	"math"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

// itemErrors records the first failure hit while enumerating a mapping through Items, which
// cannot return an error itself. EvaluateValue surfaces it once the script returns.
type itemErrors struct {
	err error
}

func (s *itemErrors) record(err error) {
	if s != nil && s.err == nil {
		s.err = err
	}
}

func configurationValue(raw any, sink *itemErrors) (starlark.Value, error) {
	switch value := raw.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(value), nil
	case string:
		return starlark.String(value), nil
	case int:
		return starlark.MakeInt(value), nil
	case int64:
		return starlark.MakeInt64(value), nil
	case uint64:
		return starlark.MakeUint64(value), nil
	case float64:
		return starlark.Float(value), nil
	default:
		return configurationInputCollection(raw, sink)
	}
}

func configurationInputCollection(raw any, sink *itemErrors) (starlark.Value, error) {
	switch value := raw.(type) {
	case script.ValueMap:
		return &configurationMap{values: value, sink: sink}, nil
	case map[string]any:
		return configurationValue(staticValueMap(value), sink)
	case []any:
		values := make([]starlark.Value, len(value))
		for i, item := range value {
			converted, err := configurationValue(item, sink)
			if err != nil {
				return nil, err
			}
			values[i] = converted
		}
		list := starlark.NewList(values)
		list.Freeze()
		return list, nil
	default:
		return nil, fmt.Errorf("%w: unsupported configuration input %T", errUtils.ErrStarlark, raw)
	}
}

func configurationResult(value starlark.Value, visiting map[starlark.Value]bool) (any, error) {
	switch value := value.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(value), nil
	case starlark.String:
		return string(value), nil
	case starlark.Int, starlark.Float:
		return configurationNumber(value)
	default:
		return configurationResultCollection(value, visiting)
	}
}

func configurationNumber(value starlark.Value) (any, error) {
	switch number := value.(type) {
	case starlark.Int:
		if n, ok := number.Int64(); ok {
			return n, nil
		}
		if n, ok := number.Uint64(); ok {
			return n, nil
		}
	case starlark.Float:
		if !math.IsInf(float64(number), 0) && !math.IsNaN(float64(number)) {
			return float64(number), nil
		}
	}
	return nil, invalidConfigurationResult(value)
}

func configurationResultCollection(value starlark.Value, visiting map[starlark.Value]bool) (any, error) {
	switch value.(type) {
	case *starlark.List, *starlark.Dict, *configurationMap:
		if visiting[value] {
			return nil, fmt.Errorf("%w: cyclic return value", errUtils.ErrStarlark)
		}
		visiting[value] = true
		defer delete(visiting, value)
		return configurationCollection(value, visiting)
	default:
		return nil, invalidConfigurationResult(value)
	}
}

func invalidConfigurationResult(value starlark.Value) error {
	return fmt.Errorf("%w: expected null, bool, string, finite number, list, or string-keyed dictionary; got %s", errUtils.ErrStarlark, value.Type())
}

func configurationCollection(value starlark.Value, visiting map[starlark.Value]bool) (any, error) {
	if list, ok := value.(*starlark.List); ok {
		result := make([]any, list.Len())
		for i := range list.Len() {
			item, err := configurationResult(list.Index(i), visiting)
			if err != nil {
				return nil, err
			}
			result[i] = item
		}
		return result, nil
	}
	mapping := value.(starlark.Mapping)
	iter := value.(starlark.Iterable).Iterate()
	defer iter.Done()
	result := make(map[string]any)
	var key starlark.Value
	for iter.Next(&key) {
		name, ok := starlark.AsString(key)
		if !ok {
			return nil, fmt.Errorf("%w: dictionary keys must be strings", errUtils.ErrStarlark)
		}
		item, found, err := mapping.Get(key)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		result[name], err = configurationResult(item, visiting)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
