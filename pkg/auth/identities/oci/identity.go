// Package oci implements host-scoped registry credentials under Atmos authentication.
package oci

import (
	"context"
	"fmt"
	"maps"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials/prompt"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Identity owns an OCI username and password/PAT scoped to one registry host.
type Identity struct {
	name, registry, username, passwordEnv, realm string
	store                                        types.CredentialStore
	prompt                                       types.CredentialPromptFunc
}

// NewIdentity creates a standalone oci/user identity with an explicit credential source.
func NewIdentity(identityName string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "ociidentity.NewIdentity")()

	if config == nil || config.Kind != types.IdentityKindOCIUser || config.Via != nil {
		return nil, errUtils.ErrInvalidIdentityConfig
	}
	identity := &Identity{name: identityName, prompt: prompt.Prompt}
	if err := identity.configure(config.Spec); err != nil {
		return nil, err
	}
	return identity, identity.Validate()
}

func (i *Identity) configure(spec map[string]any) error {
	fields := map[string]*string{"registry": &i.registry, "username": &i.username, "password_env": &i.passwordEnv}
	for key, value := range spec {
		target, ok := fields[key]
		if !ok {
			return fmt.Errorf("%w: unsupported oci/user spec field %q", errUtils.ErrInvalidIdentityConfig, key)
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: oci/user spec.%s must be a string", errUtils.ErrInvalidIdentityConfig, key)
		}
		*target = text
	}
	i.registry = strings.ToLower(i.registry)
	return nil
}

// Validate checks the explicit host and secret source without reading credentials.
func (i *Identity) Validate() error {
	if i.registry == "" || strings.ContainsAny(i.registry, "/@?# \t\r\n") || strings.TrimSpace(i.username) == "" {
		return errUtils.Build(errUtils.ErrInvalidIdentityConfig).WithExplanation("An `oci/user` identity requires `spec.registry` (host, optionally with a port) and `spec.username`.").Err()
	}
	registry, err := name.NewRegistry(i.registry, name.StrictValidation)
	if err != nil {
		return fmt.Errorf("%w: invalid oci/user registry host", errUtils.ErrInvalidIdentityConfig)
	}
	i.registry = registry.Name()
	if i.passwordEnv != "" && !environmentName.MatchString(i.passwordEnv) {
		return fmt.Errorf("%w: spec.password_env must name an environment variable", errUtils.ErrInvalidIdentityConfig)
	}
	return nil
}

// Kind returns the registered identity kind.
func (i *Identity) Kind() string { return types.IdentityKindOCIUser }

// GetProviderName returns this standalone identity's chain root.
func (i *Identity) GetProviderName() (string, error) { return i.name, nil }

// SetRealm binds keyring cleanup to the manager's credential realm.
func (i *Identity) SetRealm(realm string) { i.realm = realm }

// SetCredentialStore receives the auth manager's configured keyring.
func (i *Identity) SetCredentialStore(store types.CredentialStore) { i.store = store }

// IsStandalone identifies an identity requiring no upstream provider.
func (i *Identity) IsStandalone() bool { return true }

// Authenticate delegates to standalone credential collection.
func (i *Identity) Authenticate(ctx context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return i.AuthenticateStandalone(ctx)
}

// AuthenticateStandalone collects only the configured secret or an interactive password/PAT.
func (i *Identity) AuthenticateStandalone(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "ociidentity.AuthenticateStandalone")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	password, err := i.password()
	if err != nil {
		return nil, err
	}
	return &types.OCICredentials{Registry: i.registry, Username: i.username, Password: password}, nil
}

func (i *Identity) password() (string, error) {
	if i.passwordEnv != "" {
		//nolint:forbidigo // The exact environment variable is explicitly selected by this identity's configuration.
		password := os.Getenv(i.passwordEnv)
		if password == "" {
			return "", errUtils.Build(errUtils.ErrAuthenticationFailed).WithExplanationf("The password environment variable `%s` for identity `%s` is empty or unset.", i.passwordEnv, i.name).WithHint("Set the configured environment variable before running `atmos auth login`.").Err()
		}
		return password, nil
	}
	values, err := i.prompt(types.CredentialPromptSpec{IdentityName: i.name, CloudType: "OCI registry", Fields: []types.CredentialField{
		{Name: "registry", Title: "Registry", Default: i.registry, Required: true},
		{Name: "username", Title: "Username", Default: i.username, Required: true},
		{Name: "password", Title: "Password or personal access token", Required: true, Secret: true},
	}})
	if err != nil {
		return "", errUtils.Build(err).WithHint("Run `atmos auth login` in an interactive terminal, or configure `spec.password_env` for unattended authentication.").Err()
	}
	if values["password"] == "" {
		return "", errUtils.ErrMissingInput
	}
	return values["password"], nil
}

// Logout removes this identity's registry credentials; other identities are untouched.
func (i *Identity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "ociidentity.Logout")()

	if err := ctx.Err(); err != nil {
		return err
	}
	if i.store == nil {
		return nil
	}
	return i.store.Delete(i.name, i.realm)
}

// Environment leaves credentials in the keyring instead of exposing them as shell variables.
func (i *Identity) Environment() (map[string]string, error) { return map[string]string{}, nil }

// Paths returns no files because credentials use the configured keyring.
func (i *Identity) Paths() ([]types.Path, error) { return []types.Path{}, nil }

// PrepareEnvironment preserves the caller's environment without adding registry secrets.
func (i *Identity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return maps.Clone(environ), nil
}

// PostAuthenticate leaves persistence to the shared auth manager.
func (i *Identity) PostAuthenticate(context.Context, *types.PostAuthenticateParams) error { return nil }

// CredentialsExist reports no identity-managed files; the manager checks its keyring separately.
func (i *Identity) CredentialsExist() (bool, error) { return false, nil }

// LoadCredentials reports no secondary credential storage.
func (i *Identity) LoadCredentials(context.Context) (types.ICredentials, error) { return nil, nil }
