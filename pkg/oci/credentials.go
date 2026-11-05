package oci

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ResolveAuthentication keeps Docker/helper credentials ahead of GHCR settings.
// Resolver errors fail closed; mutation callers must never fall back to anonymous.
func ResolveAuthentication(config *schema.AtmosConfiguration, chain authn.Keychain, resource authn.Resource) (authn.Authenticator, string, error) {
	defer perf.Track(config, "oci.ResolveAuthentication")()

	if chain != nil {
		auth, err := chain.Resolve(resource)
		if err != nil {
			return nil, "", errors.Join(errUtils.ErrOCIRegistry, err)
		}
		if auth != nil && auth != authn.Anonymous {
			return auth, "registry keychain", nil
		}
	}
	if strings.EqualFold(resource.RegistryStr(), "ghcr.io") {
		if auth, source := GHCRAuth(config); auth != nil {
			return auth, source, nil
		}
	}
	return authn.Anonymous, "anonymous", nil
}

// CredentialKeychain stores registry credentials in the shared Atmos keyring.
// Registry names, not package aliases, determine credential scope.
type CredentialKeychain struct {
	Store    keyring.Keyring
	Open     func() (keyring.Keyring, error)
	Optional bool
}

func (k CredentialKeychain) backend() (keyring.Keyring, error) {
	if k.Store != nil {
		return k.Store, nil
	}
	if k.Open != nil {
		return k.Open()
	}
	return nil, errUtils.ErrOCIRegistry
}

// Resolve implements the standard OCI keychain interface.
func (k CredentialKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	defer perf.Track(nil, "oci.CredentialKeychain.Resolve")()

	store, err := k.backend()
	if err != nil {
		if k.Optional && errors.Is(err, keyring.ErrUnavailable) {
			return authn.Anonymous, nil
		}
		return nil, err
	}
	raw, err := store.Get(registryCredentialKey(resource.RegistryStr()))
	if errors.Is(err, keyring.ErrNotFound) {
		return authn.Anonymous, nil
	}
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	var config authn.AuthConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return authn.FromConfig(config), nil
}

// Login records explicit credentials for a registry without changing source configuration.
func (k CredentialKeychain) Login(registry string, config *authn.AuthConfig) error {
	defer perf.Track(nil, "oci.CredentialKeychain.Login")()

	ref, err := name.NewRegistry(registry, name.StrictValidation)
	if err != nil || config == nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	if config.Password == "" && config.IdentityToken == "" && config.RegistryToken == "" {
		return errUtils.ErrOCIRegistry
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	store, err := k.backend()
	if err != nil {
		return err
	}
	if err := store.Set(registryCredentialKey(ref.Name()), string(raw)); err != nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return nil
}

// Logout removes only Atmos's stored credentials for the selected registry.
func (k CredentialKeychain) Logout(registry string) error {
	defer perf.Track(nil, "oci.CredentialKeychain.Logout")()

	ref, err := name.NewRegistry(registry, name.StrictValidation)
	if err != nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	store, err := k.backend()
	if err != nil {
		return err
	}
	return store.Delete(registryCredentialKey(ref.Name()))
}

func registryCredentialKey(registry string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(registry)))
	return "oci/registry/" + hex.EncodeToString(digest[:])
}
