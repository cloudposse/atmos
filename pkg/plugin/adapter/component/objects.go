package component

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// EncodeObject encodes host settings without passing through protobuf floating point values.
func EncodeObject(input any) (*pluginv1.ComponentValue, error) {
	defer perf.Track(nil, "componentadapter.EncodeObject")()

	raw, err := json.Marshal(input)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	typ, err := ctyjson.ImpliedType(raw)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	value, err := ctyjson.Unmarshal(raw, typ)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	payload, err := msgpack.Marshal(value, typ)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	typeJSON, err := ctyjson.MarshalType(typ)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	return &pluginv1.ComponentValue{TypeJson: typeJSON, Msgpack: payload}, nil
}

// DecodeObject adapts a fully evaluated object for Atmos's existing stack/varfile APIs.
// Unknown values must remain in the evaluator until preparation can resolve them.
// Sensitivity paths are validated and registered with masked I/O before values escape.
func DecodeObject(input *pluginv1.ComponentValue) (map[string]any, error) {
	defer perf.Track(nil, "componentadapter.DecodeObject")()

	if input == nil {
		return map[string]any{}, nil
	}
	value, err := decodePlain(input)
	if err != nil {
		return nil, err
	}
	if !value.IsWhollyKnown() {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	if value.IsNull() || (!value.Type().IsObjectType() && !value.Type().IsMapType()) {
		return nil, errUtils.ErrPluginProtocol
	}
	if err := registerSensitivePaths(value, input.SensitivePaths); err != nil {
		return nil, err
	}
	result, err := objectJSON(value)
	if err != nil {
		return nil, err
	}
	object, ok := result.(map[string]any)
	if !ok {
		return nil, errUtils.ErrPluginProtocol
	}
	return object, nil
}

// DecodeInputs adapts the evaluated variables of a component. It behaves like DecodeObject, but a value that
// is not yet known is a defect of the plugin that the user must be able to trace, so the error names the
// input and the path of the unknown value inside it. It never prints a value.
func DecodeInputs(input *pluginv1.ComponentValue) (map[string]any, error) {
	defer perf.Track(nil, "componentadapter.DecodeInputs")()

	if input == nil {
		return map[string]any{}, nil
	}
	value, err := decodePlain(input)
	if err != nil {
		return nil, err
	}
	if !value.IsWhollyKnown() && !value.IsNull() && (value.Type().IsObjectType() || value.Type().IsMapType()) {
		return nil, unknownInputError(value)
	}
	return DecodeObject(input)
}

// unknownInputError names the first input, in key order, that holds an unknown value.
func unknownInputError(object cty.Value) error {
	if !object.IsKnown() {
		return errUtils.Build(errUtils.ErrPluginProtocol).
			WithExplanation("The plugin returned the inputs of the component as an unknown value.").
			WithHint("Report this to the plugin author: inputs must be fully evaluated before they reach Atmos.").
			Err()
	}
	attributes := object.AsValueMap()
	for _, name := range slices.Sorted(maps.Keys(attributes)) {
		if attributes[name].IsWhollyKnown() {
			continue
		}
		path := name
		_ = cty.Walk(attributes[name], func(at cty.Path, current cty.Value) (bool, error) {
			if current.IsKnown() {
				return true, nil
			}
			path = name + formatPath(at)
			return false, nil
		})
		return errUtils.Build(errUtils.ErrPluginProtocol).
			WithExplanationf("The input `%s` is not known when the component is prepared, so Atmos cannot write it to the variable file.", path).
			WithContext("input", name).
			WithHint("Terraform computes unknown values during apply; an input must be known before Terraform starts. Replace it with a value that does not depend on resources that do not exist yet.").
			WithHint("If the input comes from a dependency output, give the dependency `mock_outputs` or apply the dependency first.").
			Err()
	}
	return errUtils.ErrPluginProtocol
}

// formatPath renders a cty path as .key and [index] steps.
func formatPath(path cty.Path) string {
	var text strings.Builder
	for _, step := range path {
		switch step := step.(type) {
		case cty.GetAttrStep:
			text.WriteString("." + step.Name)
		case cty.IndexStep:
			if step.Key.Type() == cty.String {
				text.WriteString("[" + strconv.Quote(step.Key.AsString()) + "]")
			} else {
				text.WriteString("[" + step.Key.AsBigFloat().String() + "]")
			}
		}
	}
	return text.String()
}

// DecodeValue adapts one fully evaluated value of any type, such as an entry of a value map.
// A null value decodes to nil.
func DecodeValue(input *pluginv1.ComponentValue) (any, error) {
	defer perf.Track(nil, "componentadapter.DecodeValue")()

	value, err := decodePlain(input)
	if err != nil {
		return nil, err
	}
	if !value.IsWhollyKnown() {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	if value.IsNull() {
		return nil, nil
	}
	if err := registerSensitivePaths(value, input.SensitivePaths); err != nil {
		return nil, err
	}
	return objectJSON(value)
}

// DecodeString reads a fully evaluated string value. Sensitivity paths are registered with masked I/O
// before the text escapes, so a secret carried this way never reaches output unmasked.
func DecodeString(input *pluginv1.ComponentValue) (string, error) {
	defer perf.Track(nil, "componentadapter.DecodeString")()

	if input == nil {
		return "", nil
	}
	value, err := decodePlain(input)
	if err != nil {
		return "", err
	}
	if !value.IsWhollyKnown() {
		return "", errUtils.ErrPluginUnsupportedExtension
	}
	if value.IsNull() || value.Type() != cty.String {
		return "", errUtils.ErrPluginProtocol
	}
	if err := registerSensitivePaths(value, input.SensitivePaths); err != nil {
		return "", err
	}
	return value.AsString(), nil
}

func decodePlain(input *pluginv1.ComponentValue) (cty.Value, error) {
	if input == nil {
		return cty.NilVal, errUtils.ErrPluginProtocol
	}
	typ, err := ctyjson.UnmarshalType(input.TypeJson)
	if err != nil {
		return cty.NilVal, errUtils.ErrPluginProtocol
	}
	value, err := msgpack.Unmarshal(input.Msgpack, typ)
	if err != nil {
		return cty.NilVal, errUtils.ErrPluginProtocol
	}
	return value, nil
}

func sensitiveValue(value cty.Value, encoded *pluginv1.ComponentValuePath) (cty.Value, error) {
	if encoded == nil {
		return cty.NilVal, errUtils.ErrPluginProtocol
	}
	path := cty.Path{}
	for _, step := range encoded.Steps {
		switch selector := step.GetSelector().(type) {
		case *pluginv1.ComponentValueStep_Attribute:
			path = path.GetAttr(selector.Attribute)
		case *pluginv1.ComponentValueStep_Index:
			index, err := sensitiveIndex(selector.Index)
			if err != nil {
				return cty.NilVal, err
			}
			path = path.Index(index)
		default:
			return cty.NilVal, errUtils.ErrPluginProtocol
		}
	}
	result, err := path.Apply(value)
	if err != nil {
		return cty.NilVal, errUtils.ErrPluginProtocol
	}
	return result, nil
}

func objectJSON(value cty.Value) (any, error) {
	raw, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var result any
	if err := decoder.Decode(&result); err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	return result, nil
}

func registerSensitive(value any) {
	switch value := value.(type) {
	case map[string]any:
		for _, child := range value {
			registerSensitive(child)
		}
	case []any:
		for _, child := range value {
			registerSensitive(child)
		}
	default:
		atmosio.RegisterSecretValue(value)
	}
}

func registerSensitivePaths(value cty.Value, paths []*pluginv1.ComponentValuePath) error {
	for _, path := range paths {
		selected, err := sensitiveValue(value, path)
		if err != nil {
			return err
		}
		plain, err := objectJSON(selected)
		if err != nil {
			return err
		}
		registerSensitive(plain)
	}
	return nil
}

func sensitiveIndex(encoded *pluginv1.ComponentValue) (cty.Value, error) {
	index, err := decodePlain(encoded)
	if err != nil || !index.IsKnown() || index.IsNull() || len(encoded.SensitivePaths) != 0 {
		return cty.NilVal, errUtils.ErrPluginProtocol
	}
	if index.Type() != cty.String && index.Type() != cty.Number {
		return cty.NilVal, errUtils.ErrPluginProtocol
	}
	return index, nil
}
