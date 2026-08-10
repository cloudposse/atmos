package config

import (
	"reflect"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// InferValueType walks a dot-notation path (e.g. "mcp.enabled") against
// schema.AtmosConfiguration's struct tags to determine the atmosyaml.TypeXXX
// `atmos config set` should use, so a user setting a known bool/int/float
// field doesn't need to pass --type explicitly.
//
// Returns ("", false) when the path can't be resolved against the schema --
// e.g. a free-form section like vars, a typo, or a path containing an array
// index ("foo[0].bar", not yet supported). Callers should fall back to their
// own default (atmosyaml.TypeString) in that case, not treat it as an error:
// atmos.yaml routinely holds content the static schema doesn't model.
func InferValueType(dotPath string) (string, bool) {
	defer perf.Track(nil, "config.InferValueType")()

	trimmed := strings.TrimSpace(dotPath)
	if trimmed == "" || strings.Contains(trimmed, "[") {
		return "", false
	}

	t := reflect.TypeOf(schema.AtmosConfiguration{})
	for _, segment := range strings.Split(trimmed, ".") {
		if segment == "" {
			return "", false
		}
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		field, ok := findFieldByTag(t, segment)
		if !ok {
			return "", false
		}
		t = field.Type
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return valueTypeForKind(t.Kind())
}

// findFieldByTag finds a struct field of t whose "yaml" (or, failing that,
// "mapstructure") tag name -- the part before any ",omitempty"-style options
// -- matches name case-insensitively. These two tags name the same key on
// every field in schema.AtmosConfiguration; yaml is checked first since it's
// the tag that directly corresponds to the document structure dot-paths
// address.
func findFieldByTag(t reflect.Type, name string) (reflect.StructField, bool) {
	if t.Kind() != reflect.Struct {
		return reflect.StructField{}, false
	}
	for i := range t.NumField() {
		field := t.Field(i)
		if tagNameMatches(field.Tag.Get("yaml"), name) || tagNameMatches(field.Tag.Get("mapstructure"), name) {
			return field, true
		}
	}
	return reflect.StructField{}, false
}

func tagNameMatches(tag, name string) bool {
	tagName, _, _ := strings.Cut(tag, ",")
	return tagName != "" && tagName != "-" && strings.EqualFold(tagName, name)
}

// valueTypeForKind maps a resolved field's reflect.Kind to the atmosyaml
// TypeXXX constant that should write a value of that shape.
func valueTypeForKind(kind reflect.Kind) (string, bool) {
	switch kind {
	case reflect.Bool:
		return atmosyaml.TypeBool, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return atmosyaml.TypeInt, true
	case reflect.Float32, reflect.Float64:
		return atmosyaml.TypeFloat, true
	case reflect.String:
		return atmosyaml.TypeString, true
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		return atmosyaml.TypeYAML, true
	default:
		return "", false
	}
}
