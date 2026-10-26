package config

import (
	"context"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

func (r *configRule) validate(ctx context.Context, value starlark.Value, path string, depth int) (starlark.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if depth > configMaxDepth {
		return nil, configIssue(path, "configuration exceeds nesting limit or contains a cycle")
	}
	if value == starlark.None && r.optional {
		return starlark.None, nil
	}
	switch r.kind {
	case "record":
		return r.validateRecord(ctx, value, path, depth)
	case "list":
		return r.validateList(ctx, value, path, depth)
	case "enum":
		return r.validateEnum(value, path)
	default:
		return r.validateScalar(value, path)
	}
}

func (r *configRule) validateScalar(value starlark.Value, path string) (starlark.Value, error) {
	expected := r.kind
	if expected == "str" {
		expected = "string"
	}
	if value.Type() != expected || !configEnumScalar(value) {
		return nil, configIssue(path, "expected %s, got %s", expected, value.Type())
	}
	for _, limit := range []struct {
		bound   starlark.Value
		op      syntax.Token
		message string
	}{
		{r.minimum, syntax.LT, "at least"}, {r.maximum, syntax.GT, "at most"},
	} {
		if limit.bound == nil {
			continue
		}
		outside, err := starlark.Compare(limit.op, value, limit.bound)
		if err != nil {
			return nil, err
		}
		if outside {
			return nil, configIssue(path, "expected %s %s", limit.message, limit.bound)
		}
	}
	return value, nil
}

func (r *configRule) validateRecord(ctx context.Context, value starlark.Value, path string, depth int) (starlark.Value, error) {
	fields, ok := value.(starlark.IterableMapping)
	if !ok {
		return nil, configIssue(path, "expected object, got %s", value.Type())
	}
	if err := r.checkUnknownFields(fields, path); err != nil {
		return nil, err
	}
	result := starlark.NewDict(len(r.fields))
	for _, name := range r.names {
		rule := r.fields[name]
		raw, found, err := fields.Get(starlark.String(name))
		if err != nil {
			return nil, err
		}
		field := configPointer(path, name)
		if !found {
			switch {
			case rule.fallback != nil:
				raw = rule.fallback
			case rule.optional:
				raw = starlark.None
			default:
				return nil, configIssue(field, "required field is missing")
			}
		}
		checked, err := rule.validate(ctx, raw, field, depth+1)
		if err != nil {
			return nil, err
		}
		_ = result.SetKey(starlark.String(name), checked)
	}
	result.Freeze()
	return &configRecord{result}, nil
}

func (r *configRule) validateList(ctx context.Context, value starlark.Value, path string, depth int) (starlark.Value, error) {
	list, ok := value.(*starlark.List)
	if !ok {
		return nil, configIssue(path, "expected list, got %s", value.Type())
	}
	items := make([]starlark.Value, list.Len())
	for i := range items {
		item, err := r.item.validate(ctx, list.Index(i), configPointer(path, fmt.Sprint(i)), depth+1)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	result := starlark.NewList(items)
	result.Freeze()
	return result, nil
}

// configRecords copies JSON-compatible values, adding attribute access to mappings.
func configRecords(ctx context.Context, value starlark.Value, depth int) (starlark.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if depth > configMaxDepth {
		return nil, convert.InvalidArgument("configuration exceeds nesting limit or contains a cycle")
	}
	switch value := value.(type) {
	case starlark.IterableMapping:
		return configRecordCopy(ctx, value, depth)
	case *starlark.List:
		items := make([]starlark.Value, value.Len())
		for i := range items {
			child, err := configRecords(ctx, value.Index(i), depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = child
		}
		result := starlark.NewList(items)
		result.Freeze()
		return result, nil
	default:
		if !configEnumScalar(value) {
			return nil, convert.InvalidArgument("unsupported configuration value: %s", value.Type())
		}
		return value, nil
	}
}

func (r *configRule) validateEnum(value starlark.Value, path string) (starlark.Value, error) {
	for _, choice := range r.choices {
		if value.Type() != choice.Type() {
			continue
		}
		equal, err := starlark.Equal(value, choice)
		if err != nil {
			return nil, err
		}
		if equal {
			return value, nil
		}
	}
	return nil, configIssue(path, "expected one of %s, got %s", r.choices, value.Type())
}

func (r *configRule) checkUnknownFields(fields starlark.IterableMapping, path string) error {
	for _, pair := range fields.Items() {
		key, ok := starlark.AsString(pair[0])
		if !ok {
			return configIssue(path, "object keys must be strings")
		}
		if _, exists := r.fields[key]; !exists {
			return configIssue(configPointer(path, key), "unknown field")
		}
	}
	return nil
}

func configRecordCopy(ctx context.Context, value starlark.IterableMapping, depth int) (starlark.Value, error) {
	fields := starlark.NewDict(len(value.Items()))
	for _, pair := range value.Items() {
		if _, ok := starlark.AsString(pair[0]); !ok {
			return nil, convert.InvalidArgument("configuration object keys must be strings")
		}
		child, err := configRecords(ctx, pair[1], depth+1)
		if err != nil {
			return nil, err
		}
		_ = fields.SetKey(pair[0], child)
	}
	fields.Freeze()
	return &configRecord{fields}, nil
}
