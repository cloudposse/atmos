package schema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrComponentConfigurationCollision prevents dynamic settings from overriding built-in fields.
// Declared here because the host errors package depends on schema through perf.
var ErrComponentConfigurationCollision = errors.New("component configuration key collision")

// ComponentConfiguration enables one separately installed configuration provider.
// The section name is the provider name unless provider overrides it.
type ComponentConfiguration struct {
	Enabled   bool                     `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Provider  string                   `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	Mode      string                   `yaml:"mode,omitempty" json:"mode,omitempty" mapstructure:"mode"`
	Discovery ComponentDiscoveryConfig `yaml:"discovery" json:"discovery" mapstructure:"discovery"`
	Options   map[string]any           `yaml:",inline" json:"-" mapstructure:",remain"`
}

// ComponentDiscoveryConfig bounds automatic discovery. Both switches default off.
type ComponentDiscoveryConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	IncludedPaths []string `yaml:"included_paths,omitempty" json:"included_paths,omitempty" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths,omitempty" json:"excluded_paths,omitempty" mapstructure:"excluded_paths"`
}

// ComponentConfigurationRuntime is an invocation-owned host service. It is not a wire contract.
type ComponentConfigurationRuntime interface {
	Discover(context.Context) (map[string]map[string]any, error)
	Prepare(context.Context, *ConfigAndStacksInfo) (*PreparedComponentConfiguration, error)
}

// ComponentConfigurationRevisionRuntime supplies isolated discovery for Git comparisons.
// Providers without this capability must fail affected selection rather than reuse HEAD.
type ComponentConfigurationRevisionRuntime interface {
	ForRevision(root string) (ComponentConfigurationRuntime, error)
}

// PreparedComponentConfiguration contains evaluated inputs and literal artifacts.
type PreparedComponentConfiguration struct {
	Engine      string
	Source      string
	Inputs      map[string]any
	BackendType string
	Backend     map[string]any
	Workspace   string
	Files       []ComponentConfigurationFile
}

// ComponentConfigurationFile must bypass template rendering.
type ComponentConfigurationFile struct {
	Path      string
	Content   []byte
	IfExists  string
	Sensitive bool
}

// ConfigurationOwned reports whether a configuration provider owns a component's settings.
// Discovery records ownership as a map; an explicit false or null leaves an ordinary component.
func ConfigurationOwned(settings map[string]any) bool {
	switch value := settings["configuration"].(type) {
	case nil:
		return false
	case bool:
		return value
	default:
		return true
	}
}

// HasComponentDiscovery reports an explicit opt-in without consulting plugins or the filesystem.
func (c *AtmosConfiguration) HasComponentDiscovery() bool {
	for _, provider := range c.Components.Terraform.ConfigurationProviders {
		if provider.Enabled && provider.Discovery.Enabled {
			return true
		}
	}
	return false
}

// MarshalJSON preserves dynamic provider sections beside built-in Terraform settings.
//
//nolint:gocritic // Value receiver preserves encoding when Terraform is held in an interface.
func (c Terraform) MarshalJSON() ([]byte, error) {
	type plain Terraform
	return marshalConfiguration(plain(c), c.ConfigurationProviders)
}

// MarshalJSON preserves provider-owned settings without adding a host-specific options wrapper.
//
//nolint:gocritic // Value receiver preserves encoding when Terraform is held in an interface.
func (c ComponentConfiguration) MarshalJSON() ([]byte, error) {
	type plain ComponentConfiguration
	return marshalConfiguration(plain(c), c.Options)
}

func marshalConfiguration[T any](base any, extras map[string]T) ([]byte, error) {
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	reserved := jsonFieldNames(reflect.TypeOf(base))
	for name, value := range extras {
		// Check declared names too: omitempty drops an empty built-in field from result.
		if _, exists := result[name]; exists || reserved[name] {
			return nil, fmt.Errorf("%w: configuration key %s", ErrComponentConfigurationCollision, name)
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		result[name] = data
	}
	return json.Marshal(result)
}

// jsonFieldNames returns the JSON names a struct type declares, including names
// that omitempty would leave out of an encoded value.
func jsonFieldNames(t reflect.Type) map[string]bool {
	names := map[string]bool{}
	if t.Kind() != reflect.Struct {
		return names
	}
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		switch {
		case name == "-":
			continue
		case name == "" && field.Anonymous:
			for embedded := range jsonFieldNames(field.Type) {
				names[embedded] = true
			}
		case name == "" && field.IsExported():
			names[field.Name] = true
		case name != "":
			names[name] = true
		}
	}
	return names
}
