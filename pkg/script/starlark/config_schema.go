package starlark

import (
	"math"
	"sort"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/perf"
)

// configRule is an immutable snapshot of an inline schema. Defaults are copied and
// validated when declared, so sharing schemas never freezes the caller's data.
type configRule struct {
	kind             string
	fields           map[string]*configRule
	names            []string
	item             *configRule
	choices          starlark.Tuple
	optional         bool
	fallback         starlark.Value
	minimum, maximum starlark.Value
}

func (r *configRule) String() string {
	defer perf.Track(nil, "starlark.configRule.String")()
	return "<schema " + r.kind + ">"
}

func (r *configRule) Type() string {
	defer perf.Track(nil, "starlark.configRule.Type")()
	return "schema"
}

func (r *configRule) Freeze() {
	defer perf.Track(nil, "starlark.configRule.Freeze")()
}

func (r *configRule) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.configRule.Truth")()
	return starlark.True
}

func (r *configRule) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.configRule.Hash")()
	return 0, invalidArg("unhashable: schema")
}

func configSchemaModule() starlark.Value {
	return module("schema", starlark.StringDict{
		"field":    starlark.NewBuiltin("schema.field", configSchemaField),
		"optional": starlark.NewBuiltin("schema.optional", configSchemaOptional),
		"list":     starlark.NewBuiltin("schema.list", configSchemaList),
		"enum":     starlark.NewBuiltin("schema.enum", configSchemaEnum),
	})
}

func compileConfigRule(value starlark.Value, depth int) (*configRule, error) {
	if depth > configMaxDepth {
		return nil, invalidArg("schema exceeds nesting limit or contains a cycle")
	}
	if rule, ok := value.(*configRule); ok {
		return rule, nil
	}
	for _, kind := range []string{"str", "int", "float", "bool"} {
		if value == starlark.Universe[kind] {
			return &configRule{kind: kind}, nil
		}
	}
	fields, ok := value.(*starlark.Dict)
	if !ok {
		return nil, invalidArg("schema: expected str, int, float, bool, a field dictionary, or schema helper; got %s", value.Type())
	}
	rule := &configRule{kind: "record", fields: make(map[string]*configRule)}
	for _, pair := range fields.Items() {
		name, ok := starlark.AsString(pair[0])
		if !ok {
			return nil, invalidArg("schema: field names must be strings")
		}
		child, err := compileConfigRule(pair[1], depth+1)
		if err != nil {
			return nil, invalidArg("schema field %q: %s", name, err)
		}
		rule.fields[name] = child
		rule.names = append(rule.names, name)
	}
	sort.Strings(rule.names)
	return rule, nil
}

func configSchemaList(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "type", &value); err != nil {
		return nil, err
	}
	item, err := compileConfigRule(value, 0)
	if err != nil {
		return nil, err
	}
	return &configRule{kind: "list", item: item}, nil
}

func configSchemaOptional(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "type", &value); err != nil {
		return nil, err
	}
	base, err := compileConfigRule(value, 0)
	if err != nil {
		return nil, err
	}
	rule := *base
	rule.optional = true
	return &rule, nil
}

func configSchemaEnum(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) != 0 || len(args) == 0 {
		return nil, invalidArg("%s: expected one or more positional scalar choices", b.Name())
	}
	for _, choice := range args {
		if !configEnumScalar(choice) {
			return nil, invalidArg("schema.enum: choices must be strings, finite numbers, bools, or None")
		}
	}
	return &configRule{kind: "enum", choices: append(starlark.Tuple{}, args...)}, nil
}

func configEnumScalar(value starlark.Value) bool {
	switch value := value.(type) {
	case starlark.String, starlark.Int, starlark.Bool, starlark.NoneType:
		return true
	case starlark.Float:
		return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	default:
		return false
	}
}

func configSchemaField(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 1 {
		return nil, invalidArg("schema.field: default, min, and max must be keyword arguments")
	}
	var value, fallback, minimum, maximum starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "type", &value, "default?", &fallback, "min?", &minimum, "max?", &maximum); err != nil {
		return nil, err
	}
	base, err := compileConfigRule(value, 0)
	if err != nil {
		return nil, err
	}
	rule := *base
	if minimum != nil {
		rule.minimum = minimum
	}
	if maximum != nil {
		rule.maximum = maximum
	}
	if err := rule.checkBounds(); err != nil {
		return nil, err
	}
	if fallback == nil {
		fallback = rule.fallback
	}
	if fallback != nil {
		checked, err := rule.validate(threadContext(thread), fallback, "default", 0)
		if err != nil {
			return nil, invalidArg("schema.field: %s", err)
		}
		checked.Freeze()
		rule.fallback = checked
	}
	return &rule, nil
}

func (r *configRule) checkBounds() error {
	for _, bound := range []starlark.Value{r.minimum, r.maximum} {
		if bound == nil {
			continue
		}
		if !r.validBound(bound) {
			return invalidArg("schema.field: min/max require a numeric type and finite numeric bounds")
		}
	}
	if r.minimum != nil && r.maximum != nil {
		greater, err := starlark.Compare(syntax.GT, r.minimum, r.maximum)
		if err != nil {
			return err
		}
		if greater {
			return invalidArg("schema.field: min must not exceed max")
		}
	}
	return nil
}

func (r *configRule) validBound(bound starlark.Value) bool {
	return (r.kind == "int" || r.kind == "float") && isNumber(bound) && configEnumScalar(bound)
}
