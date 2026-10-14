package starlark

import (
	"fmt"
	"maps"
	"slices"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

type staticValueMap map[string]any

func (m staticValueMap) Keys() []string {
	defer perf.Track(nil, "starlark.staticValueMap.Keys")()

	return slices.Sorted(maps.Keys(m))
}

func (m staticValueMap) Get(key string) (any, bool, error) {
	defer perf.Track(nil, "starlark.staticValueMap.Get")()

	value, ok := m[key]
	return value, ok, nil
}

// configurationMap resolves individual keys, including nested maps, on demand.
// It offers read-only dict operations without exposing the host's mutable maps.
type configurationMap struct {
	values     script.ValueMap
	attributes bool
}

func (m *configurationMap) String() string {
	defer perf.Track(nil, "starlark.configurationMap.String")()

	return "<configuration>"
}

func (m *configurationMap) Type() string {
	defer perf.Track(nil, "starlark.configurationMap.Type")()

	return "configuration"
}

func (m *configurationMap) Freeze() {
	defer perf.Track(nil, "starlark.configurationMap.Freeze")()
}

func (m *configurationMap) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.configurationMap.Truth")()

	return starlark.Bool(m.Len() > 0)
}

func (m *configurationMap) Len() int {
	defer perf.Track(nil, "starlark.configurationMap.Len")()

	if m.values == nil {
		return 0
	}
	return len(m.values.Keys())
}

func (m *configurationMap) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.configurationMap.Hash")()

	return 0, fmt.Errorf("%w: configuration mappings are unhashable", errUtils.ErrStarlark)
}

func (m *configurationMap) Get(key starlark.Value) (starlark.Value, bool, error) {
	defer perf.Track(nil, "starlark.configurationMap.Get")()

	name, ok := starlark.AsString(key)
	if !ok {
		return nil, false, fmt.Errorf("%w: configuration keys must be strings", errUtils.ErrStarlark)
	}
	if m.values == nil {
		return nil, false, nil
	}
	raw, found, err := m.values.Get(name)
	if err != nil || !found {
		return nil, found, err
	}
	value, err := configurationValue(raw)
	return value, true, err
}

func (m *configurationMap) Iterate() starlark.Iterator {
	defer perf.Track(nil, "starlark.configurationMap.Iterate")()

	return m.keys().Iterate()
}

func (m *configurationMap) keys() *starlark.List {
	var keys []starlark.Value
	if m.values != nil {
		for _, key := range m.values.Keys() {
			keys = append(keys, starlark.String(key))
		}
	}
	return starlark.NewList(keys)
}

func (m *configurationMap) AttrNames() []string {
	defer perf.Track(nil, "starlark.configurationMap.AttrNames")()

	if m.attributes && m.values != nil {
		return m.values.Keys()
	}
	return []string{"get", "items", "keys", "values"}
}

func (m *configurationMap) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.configurationMap.Attr")()

	if m.attributes {
		value, _, err := m.Get(starlark.String(name))
		return value, err
	}
	if slices.Contains(m.AttrNames(), name) {
		return starlark.NewBuiltin("configuration."+name, m.method), nil
	}
	return nil, nil
}

func (m *configurationMap) method(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if b.Name() == "configuration.get" {
		return m.getDefault(b.Name(), args, kwargs)
	}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	keys := m.keys()
	if b.Name() == "configuration.keys" {
		return keys, nil
	}
	values := make([]starlark.Value, 0, keys.Len())
	for i := range keys.Len() {
		value, found, err := m.Get(keys.Index(i))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if b.Name() == "configuration.items" {
			value = starlark.Tuple{keys.Index(i), value}
		}
		values = append(values, value)
	}
	return starlark.NewList(values), nil
}

func (m *configurationMap) getDefault(name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key starlark.Value
	fallback := starlark.Value(starlark.None)
	if err := starlark.UnpackArgs(name, args, kwargs, "key", &key, "default?", &fallback); err != nil {
		return nil, err
	}
	value, found, err := m.Get(key)
	if err != nil {
		return nil, err
	}
	if !found {
		return fallback, nil
	}
	return value, nil
}
