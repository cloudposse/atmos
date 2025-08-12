package auth

import (
	"errors"

	l "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

var identityRegistry = map[string]func(provider string, identity string, config schema.AuthConfig) (LoginMethod, error){
	"aws/iam-identity-center": func(provider string, identity string, config schema.AuthConfig) (LoginMethod, error) {
		var data = &awsIamIdentityCenter{}
		b, err := yaml.Marshal(config.IdentityProviders[provider])
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(b, data)
		setDefaults(&data.Common, provider, config)
		data.Identity.Identity = identity
		return data, err
	},
	"aws/saml": func(provider string, identity string, config schema.AuthConfig) (LoginMethod, error) {
		var data = &awsSaml{}
		b, err := yaml.Marshal(config.IdentityProviders[provider])
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(b, data)
		setDefaults(&data.Common, provider, config)
		data.Identity.Identity = identity
		return data, err
	},
	//"oidc":                    func() LoginMethod { return &awsSaml{} },
}

func setDefaults(data *schema.IdentityProviderDefaultConfig, provider string, config schema.AuthConfig) {
	data.Provider = provider
	if data.Region == "" {
		data.Region = config.DefaultRegion
	}
}

func GetDefaultIdentity(config schema.AuthConfig) (string, error) {
	identityConfigs, err := GetIdentityConfigs(config)
	if err != nil {
		return "", err
	}

	var defaultIdentities []string
	for k, _ := range identityConfigs {
		if identityConfigs[k].Default && identityConfigs[k].Enabled {
			defaultIdentities = append(defaultIdentities, k)
		}
	}
	if len(defaultIdentities) == 1 {
		return defaultIdentities[0], nil
	} else if len(defaultIdentities) > 1 {
		l.Warn("multiple default identities found", "identities with default: true", defaultIdentities)
		return "", errors.New("multiple default identities found")
	}
	return "", errors.New("no default identity found")
}

func GetIdentityProviderConfigs(config schema.AuthConfig) (map[string]schema.IdentityProviderDefaultConfig, error) {
	identityConfigs := make(map[string]schema.IdentityProviderDefaultConfig)
	for k, _ := range config.IdentityProviders {
		rawBytes, err := yaml.Marshal(config.IdentityProviders[k])
		if err != nil {
			l.Errorf("failed to marshal identity %q: %w", k, err)
			return nil, err
		}

		identityConfig := &schema.IdentityProviderDefaultConfig{}
		if err := yaml.Unmarshal(rawBytes, identityConfig); err != nil {
			l.Errorf("failed to unmarshal identity %q: %w", k, err)
			return nil, err
		}
		identityConfigs[k] = *identityConfig
	}

	return identityConfigs, nil
}

func GetIdentityConfigs(config schema.AuthConfig) (map[string]schema.Identity, error) {
	identityConfigs := make(map[string]schema.Identity)
	for k, _ := range config.Identities {
		rawBytes, err := yaml.Marshal(config.Identities[k])
		if err != nil {
			l.Errorf("failed to marshal identity %q: %w", k, err)
			return nil, err
		}

		identityConfig := &schema.Identity{
			// Defaults to be overridden by unmarshalling
			Enabled: true,
		}
		if err := yaml.Unmarshal(rawBytes, identityConfig); err != nil {
			l.Errorf("failed to unmarshal identity %q: %w", k, err)
			return nil, err
		}
		identityConfigs[k] = *identityConfig
	}

	return identityConfigs, nil
}

func GetType(identityProviderName string, config schema.AuthConfig) (string, error) {
	identityConfigs, err := GetIdentityProviderConfigs(config)
	if err != nil {
		return "", err
	}
	return identityConfigs[identityProviderName].Type, nil
}

func GetIdp(identity string, config schema.AuthConfig) (string, error) {
	identityConfigs, err := GetIdentityConfigs(config)
	if err != nil {
		return "", err
	}
	return identityConfigs[identity].Idp, nil
}

func GetIdentityInstance(identity string, config schema.AuthConfig) (LoginMethod, error) {
	idpName, err := GetIdp(identity, config)
	typeVal, err := GetType(idpName, config)
	if err != nil {
		return nil, err
	}

	if providerFunc, ok := identityRegistry[typeVal]; ok {
		Lm, err := providerFunc(idpName, identity, config)
		if err != nil {
			return nil, err
		}

		b, err := yaml.Marshal(config.Identities[identity])
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(b, Lm)
		return Lm, nil

	}
	var supportedTypes []string
	for k, _ := range identityRegistry {
		supportedTypes = append(supportedTypes, k)
	}

	l.Error("unsupported identity type", "type", typeVal, "identity", identity, "supported_types", supportedTypes)
	return nil, errors.New("unsupported identity type")
}
