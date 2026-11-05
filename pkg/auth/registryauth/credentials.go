package registryauth

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func matchingIntegration(config *schema.AuthConfig, host string) (string, error) {
	selected := ""
	for key, integration := range config.Integrations {
		if integrationHost(integration) != host {
			continue
		}
		if selected != "" {
			return "", fmt.Errorf("%w: multiple registry integrations target %s for this identity", errUtils.ErrIntegrationFailed, host)
		}
		selected = key
	}
	return selected, nil
}

func fromCredentials(credentials RegistryCredentials, host string) (authn.Authenticator, error) {
	if credentials.RegistryHost() != host {
		return nil, fmt.Errorf("%w: identity credentials do not authorize registry %s", errUtils.ErrIdentityAuthFailed, host)
	}
	username, password, identityToken, registryToken := credentials.RegistryAuth()
	config := authn.AuthConfig{Username: username, Password: password, IdentityToken: identityToken, RegistryToken: registryToken}
	return requireCredentials(&config)
}

func requireCredentials(config *authn.AuthConfig) (authn.Authenticator, error) {
	if config.Password == "" && config.IdentityToken == "" && config.RegistryToken == "" {
		return nil, errUtils.ErrIdentityCredentialsNone
	}
	return authn.FromConfig(*config), nil
}

func validateDirectIdentity(identity *schema.Identity, host string) error {
	if identity.Kind != "oci/user" {
		return nil
	}
	registry, _ := identity.Spec["registry"].(string)
	normalized, err := name.NewRegistry(strings.ToLower(registry), name.StrictValidation)
	if err != nil || normalized.Name() != host {
		return fmt.Errorf("%w: OCI identity does not authorize registry %s", errUtils.ErrIdentityAuthFailed, host)
	}
	return nil
}

func validateCredentialSelection(identity *schema.Identity, credentials RegistryCredentials) error {
	if identity.Kind != "oci/user" {
		return nil
	}
	expected, _ := identity.Spec["username"].(string)
	username, _, _, _ := credentials.RegistryAuth()
	if expected != "" && expected != username {
		return fmt.Errorf("%w: cached OCI username does not match the configured identity; log out and authenticate again", errUtils.ErrIdentityAuthFailed)
	}
	return nil
}
