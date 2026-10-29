// Package component supplies lossless values for component configuration services.
package component

import (
	"errors"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// ErrInvalidValue identifies a value that cannot cross the component boundary.
// Errors deliberately omit payloads, including cty diagnostics containing values.
var ErrInvalidValue = errors.New("invalid component value")

type valueMark string

// Sensitive marks a value that must not be rendered without masking.
const Sensitive valueMark = "sensitive"

// Encode preserves cty types, unknowns, exact numbers, and sensitivity. Other
// marks are rejected rather than silently removing their semantic meaning.
//
//nolint:lintroller // Pure SDK serialization; the standalone SDK cannot import host perf tracking.
func Encode(value cty.Value) (*pluginv1.ComponentValue, error) {
	if value == cty.NilVal {
		return nil, ErrInvalidValue
	}
	plain, marks := value.UnmarkDeepWithPaths()
	typeJSON, err := ctyjson.MarshalType(plain.Type())
	if err != nil {
		return nil, ErrInvalidValue
	}
	payload, err := msgpack.Marshal(plain, plain.Type())
	if err != nil {
		return nil, ErrInvalidValue
	}
	result := &pluginv1.ComponentValue{TypeJson: typeJSON, Msgpack: payload}
	for _, marked := range marks {
		if len(marked.Marks) != 1 || !marked.Marks.Equal(cty.NewValueMarks(Sensitive)) {
			return nil, ErrInvalidValue
		}
		path, err := encodePath(marked.Path)
		if err != nil {
			return nil, err
		}
		result.SensitivePaths = append(result.SensitivePaths, path)
	}
	return result, nil
}

// Decode validates the type, payload, and sensitivity paths before accepting a
// value. Invalid paths must not silently drop sensitivity marks.
//
//nolint:lintroller // Pure SDK validation and decoding; the standalone SDK cannot import host perf tracking.
func Decode(encoded *pluginv1.ComponentValue) (cty.Value, error) {
	if encoded == nil {
		return cty.NilVal, ErrInvalidValue
	}
	typeOf, err := ctyjson.UnmarshalType(encoded.TypeJson)
	if err != nil {
		return cty.NilVal, ErrInvalidValue
	}
	value, err := msgpack.Unmarshal(encoded.Msgpack, typeOf)
	if err != nil {
		return cty.NilVal, ErrInvalidValue
	}
	marks := make([]cty.PathValueMarks, 0, len(encoded.SensitivePaths))
	for _, encodedPath := range encoded.SensitivePaths {
		path, err := decodePath(encodedPath)
		if err != nil {
			return cty.NilVal, err
		}
		if _, err := path.Apply(value); err != nil {
			return cty.NilVal, ErrInvalidValue
		}
		marks = append(marks, cty.PathValueMarks{Path: path, Marks: cty.NewValueMarks(Sensitive)})
	}
	return applyMarks(value, marks)
}

func applyMarks(value cty.Value, marks []cty.PathValueMarks) (cty.Value, error) {
	result := value.MarkWithPaths(marks)
	_, applied := result.UnmarkDeepWithPaths()
	for _, expected := range marks {
		found := false
		for _, actual := range applied {
			if expected.Equal(actual) {
				found = true
				break
			}
		}
		if !found {
			return cty.NilVal, ErrInvalidValue
		}
	}
	return result, nil
}

func encodePath(path cty.Path) (*pluginv1.ComponentValuePath, error) {
	result := &pluginv1.ComponentValuePath{}
	for _, step := range path {
		item := &pluginv1.ComponentValueStep{}
		switch step := step.(type) {
		case cty.GetAttrStep:
			item.Selector = &pluginv1.ComponentValueStep_Attribute{Attribute: step.Name}
		case cty.IndexStep:
			index, err := Encode(step.Key)
			if err != nil {
				return nil, err
			}
			item.Selector = &pluginv1.ComponentValueStep_Index{Index: index}
		default:
			return nil, ErrInvalidValue
		}
		result.Steps = append(result.Steps, item)
	}
	return result, nil
}

func decodePath(encoded *pluginv1.ComponentValuePath) (cty.Path, error) {
	if encoded == nil {
		return nil, ErrInvalidValue
	}
	path := cty.Path{}
	for _, step := range encoded.Steps {
		switch step := step.GetSelector().(type) {
		case *pluginv1.ComponentValueStep_Attribute:
			path = path.GetAttr(step.Attribute)
		case *pluginv1.ComponentValueStep_Index:
			index, err := decodeIndex(step.Index)
			if err != nil {
				return nil, err
			}
			path = path.Index(index)
		default:
			return nil, ErrInvalidValue
		}
	}
	return path, nil
}

func decodeIndex(encoded *pluginv1.ComponentValue) (cty.Value, error) {
	if encoded == nil || len(encoded.SensitivePaths) != 0 {
		return cty.NilVal, ErrInvalidValue
	}
	index, err := Decode(encoded)
	if err != nil || !index.IsKnown() || index.IsNull() || (!index.Type().Equals(cty.String) && !index.Type().Equals(cty.Number)) {
		return cty.NilVal, ErrInvalidValue
	}
	return index, nil
}
