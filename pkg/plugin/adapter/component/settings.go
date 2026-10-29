package component

import (
	"bytes"
	"encoding/json"

	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func (p *Provider) validateConfiguration(value *pluginv1.ComponentValue) error {
	settings := map[string]any{}
	if value != nil {
		var err error
		settings, err = settingsObject(value)
		if err != nil {
			return err
		}
	}
	return p.ValidateSettings(settings)
}

// settingsObject decodes a known provider-settings object only for local schema
// validation. The original typed envelope and sensitivity paths travel unchanged
// to the plugin; resolved component inputs are not converted to JSON here.
func settingsObject(encoded *pluginv1.ComponentValue) (map[string]any, error) {
	valueType, err := ctyjson.UnmarshalType(encoded.TypeJson)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	value, err := msgpack.Unmarshal(encoded.Msgpack, valueType)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	data, err := ctyjson.Marshal(value, valueType)
	if err != nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var settings map[string]any
	if err := decoder.Decode(&settings); err != nil || settings == nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	return settings, nil
}
