package yamlsource

import (
	"reflect"

	"gopkg.in/yaml.v3"
)

// RestoreTags restores explicit tags for public YAML output and YAML template
// passes. Plain strings, including quoted function expressions, are unchanged.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func RestoreTags(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode {
		if source, ok := Decode(node.Value); ok {
			node.Tag, node.Value = source.Tag, source.Argument
			node.Style = yaml.TaggedStyle | yaml.DoubleQuotedStyle
		}
	}
	for _, child := range node.Content {
		RestoreTags(child)
	}
}

// Display returns an ordinary representation for output formats without YAML
// tags. Internal JSON cloning should retain the transport strings instead.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Display(value any) any {
	switch v := value.(type) {
	case string:
		return Expression(v)
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			result[key] = Display(item)
		}
		return result
	case map[any]any:
		result := make(map[any]any, len(v))
		for key, item := range v {
			result[key] = Display(item)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = Display(item)
		}
		return result
	default:
		return displayTyped(value)
	}
}

// displayTyped converts transported sources inside typed containers such as
// map[string]string or []string. The container type is preserved, and the input is
// returned unchanged unless a nested value actually differs.
func displayTyped(value any) any {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map && rv.Kind() != reflect.Slice {
		return value
	}
	if rv.IsNil() || !holdsDisplayable(rv.Type().Elem()) {
		return value
	}
	if rv.Kind() == reflect.Map {
		return displayMap(rv)
	}
	return displaySlice(rv)
}

// displayMap converts the values of a typed map, copying it only when one changes.
func displayMap(rv reflect.Value) any {
	elemType := rv.Type().Elem()
	out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
	changed := false
	for iter := rv.MapRange(); iter.Next(); {
		item, itemChanged := displayElem(iter.Value(), elemType)
		changed = changed || itemChanged
		out.SetMapIndex(iter.Key(), item)
	}
	if !changed {
		return rv.Interface()
	}
	return out.Interface()
}

// displaySlice converts the elements of a typed slice, copying it only when one changes.
func displaySlice(rv reflect.Value) any {
	elemType := rv.Type().Elem()
	out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
	changed := false
	for i := range rv.Len() {
		item, itemChanged := displayElem(rv.Index(i), elemType)
		changed = changed || itemChanged
		out.Index(i).Set(item)
	}
	if !changed {
		return rv.Interface()
	}
	return out.Interface()
}

// holdsDisplayable reports whether values of the type can contain a transported source.
// Byte slices and other scalar containers are skipped without being walked.
func holdsDisplayable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface, reflect.Map, reflect.Slice:
		return true
	case reflect.String:
		return t == reflect.TypeOf("")
	default:
		return false
	}
}

// displayElem converts one container element, keeping the original when the result
// cannot be stored back in the element type.
func displayElem(item reflect.Value, elemType reflect.Type) (reflect.Value, bool) {
	converted := Display(item.Interface())
	if converted == nil {
		return item, false
	}
	cv := reflect.ValueOf(converted)
	if !cv.Type().AssignableTo(elemType) {
		return item, false
	}
	return cv, !reflect.DeepEqual(converted, item.Interface())
}
