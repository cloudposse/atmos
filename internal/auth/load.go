package auth

import (
	"errors"
	l "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

// TODO decide if we want to use this map of functions or a switch statement.
var identityRegistry = map[string]func() LoginMethod{
	"aws/iam-identity-center": func() LoginMethod { return &awsIamIdentityCenter{} },
	//"oidc":                    func() LoginMethod { return &awsSaml{} },
	"aws/saml": func() LoginMethod { return &awsSaml{} },
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

func GetIdentityConfigs(config schema.AuthConfig) (map[string]schema.IdentityDefaultConfig, error) {
	identityConfigs := make(map[string]schema.IdentityDefaultConfig)
	for k, _ := range config.Identities {
		rawBytes, err := yaml.Marshal(config.Identities[k])
		if err != nil {
			l.Errorf("failed to marshal identity %q: %w", k, err)
			return nil, err
		}

		identityConfig := &schema.IdentityDefaultConfig{}
		if err := yaml.Unmarshal(rawBytes, identityConfig); err != nil {
			l.Errorf("failed to unmarshal identity %q: %w", k, err)
			return nil, err
		}
		identityConfigs[k] = *identityConfig
	}

	return identityConfigs, nil
}

func GetType(identity string, config schema.AuthConfig) (string, error) {
	identityConfigs, err := GetIdentityConfigs(config)
	if err != nil {
		return "", err
	}
	return identityConfigs[identity].Type, nil
	//for k, _ := range config.Identities {
	//	if k == identity {
	//		rawBytes, err := yaml.Marshal(config.Identities[k])
	//		if err != nil {
	//			l.Errorf("failed to marshal identity %q: %w", k, err)
	//			return "", err
	//		}
	//
	//		identityConfig := &schema.IdentityDefaultConfig{}
	//		if err := yaml.Unmarshal(rawBytes, identityConfig); err != nil {
	//			l.Errorf("failed to unmarshal identity %q: %w", k, err)
	//			return "", err
	//		}
	//		return identityConfig.Type, nil
	//	}
	//}
	//return "", fmt.Errorf("identity %q not found", identity)
}

func GetIdentityInstance(identity string, config schema.AuthConfig) (LoginMethod, error) {
	typeVal, err := GetType(identity, config)
	if err != nil {
		return nil, err
	}

	// TODO see above decision
	switch typeVal {
	case "aws/iam-identity-center":
		var data = &awsIamIdentityCenter{}
		b, err := yaml.Marshal(config.Identities[identity])
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(b, data)
		data.Alias = identity
		if data.Region == "" {
			data.Region = config.DefaultRegion
		}
		return data, err
	case "aws/saml":
		var data = &awsSaml{}
		b, err := yaml.Marshal(config.Identities[identity])
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(b, data)
		data.Alias = identity
		return data, err
	}

	l.Error("unsupported identity type", "type", typeVal, "identity", identity)
	return nil, errors.New("unsupported identity type")
}
