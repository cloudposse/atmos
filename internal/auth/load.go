package auth

import (
	"errors"

	l "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

func NewIdentity() schema.Identity {
	return schema.Identity{
		Enabled: true,
	}
}

var identityRegistry = map[string]func(provider string, identity string, config schema.AuthConfig) (LoginMethod, error){
	"aws/iam-identity-center": NewAwsIamIdentityCenterFactory,
	"aws/saml":                NewAwsSamlFactory,
	"":                        NewAwsAssumeRoleFactory, // Empty - used for AssumeRole - no ProviderName
	"aws/user":                NewAwsUserFactory,
	"aws/oidc":               NewAwsOidcFactory,
}

func setDefaults(data *schema.ProviderDefaultConfig, provider string, config schema.AuthConfig) {
	if data.Region == "" {
		data.Region = config.DefaultRegion
	}
	if data.Profile == "" {
		data.Profile = config.DefaultProfile
	}
}

func GetDefaultIdentity(configuration map[string]any) (string, error) {
	identityConfigs := GetEnabledIdentities(configuration)
	var defaultIdentities []string
	for k := range identityConfigs {
		if identityConfigs[k].Default && identityConfigs[k].Enabled {
			defaultIdentities = append(defaultIdentities, k)
		}
	}
	l.Debug("Fount default identities", "defaultIdentities", defaultIdentities)
	if len(defaultIdentities) == 1 {
		return defaultIdentities[0], nil
	} else if len(defaultIdentities) > 1 {
		l.Warn("multiple default identities found", "defaultIdentities", defaultIdentities)
		return "", errors.New("multiple default identities found")
	}
	return "", errors.New("no default identity found")
}

func GetProviderConfigs(config schema.AuthConfig) (map[string]schema.ProviderDefaultConfig, error) {
	identityConfigs := make(map[string]schema.ProviderDefaultConfig)
	for k := range config.Providers {
		rawBytes, err := yaml.Marshal(config.Providers[k])
		if err != nil {
			l.Errorf("failed to marshal identity %q: %v", k, err)
			return nil, err
		}

		identityConfig := &schema.ProviderDefaultConfig{}
		if err := yaml.Unmarshal(rawBytes, identityConfig); err != nil {
			l.Errorf("failed to unmarshal identity %q: %v", k, err)
			return nil, err
		}
		identityConfigs[k] = *identityConfig
	}

	return identityConfigs, nil
}

func GetAllIdentityConfigs(identityMap map[string]any) (map[string]schema.Identity, error) {
	identityConfigs := make(map[string]schema.Identity)
	for k, v := range identityMap {
		rawBytes, err := yaml.Marshal(v)
		if err != nil {
			l.Errorf("failed to marshal identity %q: %v", k, err)
			return nil, err
		}

		identityConfig := &schema.Identity{
			// Defaults to be overridden by unmarshalling
			Enabled: true,
		}
		if err := yaml.Unmarshal(rawBytes, identityConfig); err != nil {
			l.Errorf("failed to unmarshal identity %q: %v", k, err)
			return nil, err
		}
		identityConfigs[k] = *identityConfig
	}

	return identityConfigs, nil
}

func GetEnabledIdentities(identityMap map[string]any) map[string]schema.Identity {
	identityConfigs, err := GetEnabledIdentitiesE(identityMap)
	if err != nil {
		l.Errorf("failed to get enabled identities: %v", err)
		return nil
	}
	return identityConfigs
}

func GetEnabledIdentitiesE(identityMap map[string]any) (map[string]schema.Identity, error) {
	identityConfigs, err := GetAllIdentityConfigs(identityMap)
	if err != nil {
		return nil, err
	}
	filteredIdentities := make(map[string]schema.Identity)
	for k, v := range identityConfigs {
		// TODO move this to a validate method
		if v.Enabled && (v.Provider != "" || v.RoleArn != "") {
			filteredIdentities[k] = v
		}
	}
	return filteredIdentities, nil
}

func GetType(identityProviderName string, config schema.AuthConfig) (string, error) {
	identityConfigs, err := GetProviderConfigs(config)
	if err != nil {
		return "", err
	}
	return identityConfigs[identityProviderName].Type, nil
}

func GetIdp(identity string, config schema.AuthConfig) (string, error) {
	identityConfigs, err := GetAllIdentityConfigs(config.Identities)
	if err != nil {
		return "", err
	}
	return identityConfigs[identity].Provider, nil
}

// GetIdentityInstance retrieves an identity instance based on the specified identity and configuration.
// If info is provided, component-level identities will be considered and can override global ones.
func GetIdentityInstance(identity string, config schema.AuthConfig, info *schema.ConfigAndStacksInfo) (LoginMethod, error) {
	// Merge component identities with global identities if info is provided
	identityMap := config.Identities
	if info != nil && info.ComponentIdentitiesSection != nil {
		// Use component identities if they exist, otherwise fall back to global identities
		componentIdentities := info.ComponentIdentitiesSection
		if len(componentIdentities) > 0 {
			// Only override the specific identity if it exists in component identities
			if componentIdentity, exists := componentIdentities[identity]; exists {
				// Clone the global identities map
				mergedIdentities := make(map[string]any)
				for k, v := range config.Identities {
					mergedIdentities[k] = v
				}

				// Override with component identity
				mergedIdentities[identity] = componentIdentity
				identityMap = mergedIdentities
			}
		}
	}

	// Create a temporary config with the merged identities
	mergedConfig := schema.AuthConfig{
		Identities:     identityMap,
		Providers:      config.Providers,
		DefaultRegion:  config.DefaultRegion,
		DefaultProfile: config.DefaultProfile,
	}

	idpName, err := GetIdp(identity, mergedConfig)
	typeVal, err := GetType(idpName, mergedConfig)
	l.Info("Using Identity Instance", "identity", identity, "provider", idpName, "type", typeVal)

	if err != nil {
		return nil, err
	}

	if providerFunc, ok := identityRegistry[typeVal]; ok {
		// providerFunc is a function based on the provider type from identityRegistry.
		// This essentially returns a LoginMethod of the correct type
		Lm, err := providerFunc(idpName, identity, mergedConfig)
		if err != nil {
			return nil, err
		}

		b, err := yaml.Marshal(mergedConfig.Identities[identity])
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(b, Lm)
		return Lm, nil
	}

	var supportedTypes []string
	for k := range identityRegistry {
		supportedTypes = append(supportedTypes, k)
	}

	l.Error("unsupported identity type", "type", typeVal, "identity", identity, "supported_types", supportedTypes)
	return nil, errors.New("unsupported identity type")
}
