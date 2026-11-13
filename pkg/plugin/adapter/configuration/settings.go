package configuration

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/provideroptions"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultIncludedPaths is the discovery scope when `discovery.enabled` is true and
// `discovery.included_paths` is not set: every unit below the project root. Terragrunt itself
// discovers recursively from the working directory, and failing here would make the natural
// first step (`atmos config set ...discovery.enabled true`) lock the project out of every command.
var defaultIncludedPaths = []string{"**"}

// settingsValidator is implemented by providers that check their own settings schema locally.
type settingsValidator interface {
	ValidateSettings(map[string]any) error
}

// settingsKey returns the configuration key of a provider setting, for example
// `components.terraform.terragrunt.discovery.included_paths`.
func settingsKey(section string, parts ...string) string {
	return strings.Join(append([]string{"components", "terraform", section}, parts...), ".")
}

// invalidSetting names the offending key and how to repair it. The repair is always possible with
// `atmos config set`, which never loads or validates plugin settings.
func invalidSetting(key, problem, hint string) error {
	return errUtils.Build(errUtils.ErrPluginInvalidConfig).
		WithExplanationf("%s: `%s`.", problem, key).
		WithContext("config_key", key).
		WithHint(hint).
		Err()
}

// validateSettings checks one enabled provider section without starting a plugin.
func validateSettings(section string, settings *schema.ComponentConfiguration) error {
	if settings.Mode != "" && settings.Mode != provideroptions.ModeNative && settings.Mode != provideroptions.ModeExecutable {
		return invalidSetting(settingsKey(section, "mode"), "Unsupported mode", "The supported modes are `native` and `executable`. Remove `mode`, or fix it with `atmos config set "+settingsKey(section, "mode")+" native`.")
	}
	for _, field := range []struct {
		name     string
		patterns []string
	}{{"included_paths", settings.Discovery.IncludedPaths}, {"excluded_paths", settings.Discovery.ExcludedPaths}} {
		for _, pattern := range field.patterns {
			if !filepath.IsLocal(pattern) {
				key := settingsKey(section, "discovery", field.name)
				return invalidSetting(key, fmt.Sprintf("The pattern %q must be relative to the project root and stay inside it", pattern), "Use a pattern such as `live/**`; absolute paths and `..` are not allowed.")
			}
		}
	}
	return nil
}

// validateProviderOptions asks the provider to check keys the host does not own, such as a
// misspelled `discoverey`, so they fail loudly instead of silently doing nothing.
func validateProviderOptions(section string, provider Provider, request *pluginv1.DiscoverComponentsRequest) error {
	validator, ok := provider.(settingsValidator)
	if !ok {
		return nil
	}
	values, err := adapter.DecodeObject(request.ProviderConfiguration)
	if err != nil {
		return err
	}
	if err := validator.ValidateSettings(values); err != nil {
		return errUtils.Build(err).WithContext("config_section", settingsKey(section)).Err()
	}
	return nil
}

// operationMode is the mode a section runs in; an unset mode is native.
func operationMode(settings *schema.ComponentConfiguration) string {
	if settings.Mode == "" {
		return provideroptions.ModeNative
	}
	return settings.Mode
}

// operationOptions encodes the options the user set explicitly. Only those are sent, so the provider
// can tell an absent option from an explicit false, zero, or empty one. Sensitive values are marked so
// every consumer masks them.
func operationOptions(options map[string]schema.ComponentProviderOption) (map[string]*pluginv1.ComponentValue, error) {
	if len(options) == 0 {
		return nil, nil
	}
	result := make(map[string]*pluginv1.ComponentValue, len(options))
	for name, option := range options {
		value, err := adapter.EncodeObject(option.Value)
		if err != nil {
			return nil, err
		}
		if option.Sensitive {
			value.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
		}
		result[name] = value
	}
	return result, nil
}

// providerOperation returns the mode and a copy of the options of the section that enabled a provider.
func (s *Session) providerOperation(provider string) (string, map[string]*pluginv1.ComponentValue) {
	for _, request := range s.importProviders {
		if request.Provider != provider || request.Operation == nil {
			continue
		}
		options := make(map[string]*pluginv1.ComponentValue, len(request.Operation.Options))
		for name, value := range request.Operation.Options {
			options[name] = proto.Clone(value).(*pluginv1.ComponentValue)
		}
		return request.Operation.Mode, options
	}
	return provideroptions.ModeNative, nil
}

// sortedSections lists the configured provider sections in stable order.
func (s *Session) sortedSections() []string {
	sections := make([]string, 0, len(s.importProviders))
	for section := range s.importProviders {
		sections = append(sections, section)
	}
	slices.Sort(sections)
	return sections
}

// validateOptions checks the provider-owned keys of every enabled section that sets any.
func (s *Session) validateOptions(registry map[string]Provider) error {
	for _, section := range s.sortedSections() {
		request := s.importProviders[section]
		if len(s.options[section]) == 0 {
			continue
		}
		provider := registry[request.Provider]
		if provider == nil {
			continue
		}
		if err := validateProviderOptions(section, provider, request); err != nil {
			return err
		}
	}
	return nil
}

// hasProviderOptions reports whether any enabled section sets keys the host does not own.
func (s *Session) hasProviderOptions() bool {
	for _, options := range s.options {
		if len(options) > 0 {
			return true
		}
	}
	return false
}

func (s *Session) providerSettings(name string) *pluginv1.ComponentValue {
	var settings *pluginv1.ComponentValue
	for _, request := range s.importProviders {
		if request.Provider == name {
			settings = request.ProviderConfiguration
		}
	}
	return settings
}
