package registryauth

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	awscloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	azurecloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PluginBindings resolves configured registry aliases into exact registry host bindings.
// Two aliases for a host must agree on the identity; credentials never follow catalog links.

func PluginBindings(config *schema.AtmosConfiguration) (map[string]string, error) {
	bindings := make(map[string]string)
	for alias, registry := range config.Plugins.Registries {
		if registry.Identity == "" {
			continue
		}
		if _, exists := config.Auth.Identities[registry.Identity]; !exists {
			return nil, fmt.Errorf("%w: plugin registry %s references identity %s", errUtils.ErrIdentityNotFound, alias, registry.Identity)
		}
		host, err := pluginconfig.RegistryHost(&registry)
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", alias, err)
		}
		if previous, exists := bindings[host]; exists && previous != registry.Identity {
			return nil, fmt.Errorf("%w: registry host %s has conflicting identities", errUtils.ErrPluginInvalidConfig, host)
		}
		bindings[host] = registry.Identity
	}
	return bindings, nil
}

func scopedConfig(config *schema.AuthConfig, identity string) (*schema.AuthConfig, error) {
	scoped := *config
	scoped.Identities = make(map[string]schema.Identity)
	scoped.Providers = make(map[string]schema.Provider)
	scoped.Integrations = make(map[string]schema.Integration)
	for current := identity; current != ""; {
		if _, seen := scoped.Identities[current]; seen {
			return nil, fmt.Errorf("%w: registry identity chain contains a cycle at %s", errUtils.ErrIdentityAuthFailed, current)
		}
		selected, found := config.Identities[current]
		if !found {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrIdentityNotFound, current)
		}
		scoped.Identities[current] = selected
		parent, provider := identityParent(&selected)
		current = parent
		if provider != "" {
			item, found := config.Providers[provider]
			if !found {
				return nil, fmt.Errorf("%w: %s", errUtils.ErrProviderNotFound, provider)
			}
			scoped.Providers[provider] = item
		}
	}
	for key, integration := range config.Integrations {
		if integration.Via != nil && integration.Via.Identity == identity {
			scoped.Integrations[key] = integration
		}
	}
	return &scoped, nil
}

func integrationHost(integration schema.Integration) string {
	if integration.Kind == "aws/ecr-public" {
		return "public.ecr.aws"
	}
	if integration.Spec == nil || integration.Spec.Registry == nil {
		return ""
	}
	registry := integration.Spec.Registry
	switch integration.Kind {
	case "aws/ecr":
		return awscloud.BuildRegistryURL(registry.AccountID, registry.Region)
	case "azure/acr":
		return azurecloud.BuildRegistryURL(registry.Name)
	default:
		return ""
	}
}

func identityParent(identity *schema.Identity) (parent, provider string) {
	provider = identity.Provider
	if identity.Via != nil {
		parent = identity.Via.Identity
		if identity.Via.Provider != "" {
			provider = identity.Via.Provider
		}
	}
	return parent, provider
}

// Identity resolves an exact normalized host to its configured Atmos identity.
func Identity(config *schema.AtmosConfiguration, host string) (string, error) {
	bindings, err := PluginBindings(config)
	if err != nil {
		return "", err
	}
	normalized, err := normalizeHost(host)
	if err != nil {
		return "", err
	}
	return bindings[normalized], nil
}

func normalizeHost(host string) (string, error) {
	registry, err := name.NewRegistry(strings.ToLower(host), name.StrictValidation)
	if err != nil {
		return "", fmt.Errorf("%w: invalid registry host", errUtils.ErrInvalidAuthConfig)
	}
	return registry.Name(), nil
}
