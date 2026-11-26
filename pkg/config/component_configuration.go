package config

import (
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// terraformKnownKeys returns the mapstructure names of the built-in `components.terraform` settings,
// derived from the schema struct so a new setting is known without a separate list.
func terraformKnownKeys() map[string]bool {
	typ := reflect.TypeOf(schema.Terraform{})
	known := make(map[string]bool)
	for i := range typ.NumField() {
		name := strings.Split(typ.Field(i).Tag.Get("mapstructure"), ",")[0]
		if name != "" && name != "-" {
			known[name] = true
		}
	}
	return known
}

// Preserve the loader's established handling of unknown Terraform keys.
// Only mappings with an explicit enabled or provider marker claim a provider section.
// The other unknown keys are dropped here and recorded by recordUnknownTerraformSections.
func terraformConfigurationDecodeHook() mapstructure.DecodeHookFuncType {
	typ := reflect.TypeOf(schema.Terraform{})
	known := terraformKnownKeys()
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

// recordUnknownTerraformSections records the keys under `components.terraform` that the decode hook
// drops: keys that are neither a built-in setting nor a mapping with an enabled or provider marker.
// Validation reports them later, once the plugin packages that might claim them are known.
func recordUnknownTerraformSections(v *viper.Viper, config *schema.AtmosConfiguration) {
	config.Components.Terraform.UnknownSections = nil
	fields, ok := v.Get("components.terraform").(map[string]any)
	if !ok {
		return
	}
	known := terraformKnownKeys()
	var unknown []string
	for key, item := range fields {
		if !known[key] && !claimedConfigurationProvider(item) {
			unknown = append(unknown, key)
		}
	}
	slices.Sort(unknown)
	config.Components.Terraform.UnknownSections = unknown
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
