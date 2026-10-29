package component

import (
	"bytes"
	"encoding/json"

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
