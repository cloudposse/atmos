package config

import (
	"maps"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Preserve the loader's established handling of unknown Terraform keys.
// Only mappings with an explicit enabled or provider marker claim a provider section.
func terraformConfigurationDecodeHook() mapstructure.DecodeHookFuncType {
	typ := reflect.TypeOf(schema.Terraform{})
	known := make(map[string]bool)
	for i := range typ.NumField() {
		name := strings.Split(typ.Field(i).Tag.Get("mapstructure"), ",")[0]
		known[name] = true
	}
	return func(_ reflect.Type, to reflect.Type, value any) (any, error) {
		if to != typ {
			return value, nil
		}
		fields, ok := value.(map[string]any)
		if !ok {
			return value, nil
		}
		result := maps.Clone(fields)
		for key, item := range result {
			if known[key] {
				continue
			}
			if !claimedConfigurationProvider(item) {
				delete(result, key)
			}
		}
		return result, nil
	}
}

func claimedConfigurationProvider(value any) bool {
	fields, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, enabled := fields["enabled"]
	_, provider := fields["provider"]
	return enabled || provider
}
