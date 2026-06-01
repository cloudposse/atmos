// Package atmospro implements the `atmos/pro` passthrough identity.
//
// The atmos/pro provider already yields usable credentials (an Atmos Pro session JWT),
// so this identity is a thin passthrough: it forwards the provider's credentials so that
// linked integrations (e.g., github/sts) receive the session JWT. It exists so callers
// have an identity to authenticate/run as; integrations may instead bind directly to the
// provider via `via.provider`.
package atmospro

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// IdentityKind is the identity kind for the Atmos Pro identity.
const IdentityKind = "atmos/pro"

const logKeyIdentity = "identity"

// proIdentity implements a passthrough identity over the atmos/pro provider.
type proIdentity struct {
	name   string
	config *schema.Identity
	realm  string
}

// NewIdentity creates a new atmos/pro passthrough identity. It requires via.provider.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "atmospro.NewIdentity")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidAuthConfig, name)
	}
	if config.Kind != IdentityKind {
		return nil, fmt.Errorf("%w: invalid identity kind for atmos/pro: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}
	if config.Via == nil || config.Via.Provider == "" {
		return nil, fmt.Errorf("%w: atmos/pro identity %q must define via.provider", errUtils.ErrInvalidIdentityConfig, name)
	}

	return &proIdentity{name: name, config: config}, nil
}

// Kind returns the identity kind.
func (i *proIdentity) Kind() string { return IdentityKind }

// SetRealm sets the credential isolation realm for this identity.
func (i *proIdentity) SetRealm(realm string) { i.realm = realm }

// GetProviderName returns the provider name from via.provider.
func (i *proIdentity) GetProviderName() (string, error) {
	if i.config.Via == nil || i.config.Via.Provider == "" {
		return "", fmt.Errorf("%w: atmos/pro identity %q has no via.provider", errUtils.ErrInvalidIdentityConfig, i.name)
	}
	return i.config.Via.Provider, nil
}

// Authenticate passes the provider's credentials (the Atmos Pro session JWT) through unchanged.
func (i *proIdentity) Authenticate(_ context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "atmospro.proIdentity.Authenticate")()

	log.Debug("Atmos Pro identity passing provider credentials through", logKeyIdentity, i.name)
	return baseCreds, nil
}

// Validate validates the identity configuration.
func (i *proIdentity) Validate() error {
	if i.config.Via == nil || i.config.Via.Provider == "" {
		return fmt.Errorf("%w: atmos/pro identity %q must define via.provider", errUtils.ErrInvalidIdentityConfig, i.name)
	}
	return nil
}

// Environment returns environment variables for this identity (none; integrations contribute git env).
func (i *proIdentity) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

// Paths returns credential files/directories used by this identity (none).
func (i *proIdentity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment returns the environment unchanged (a copy to avoid mutation).
func (i *proIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "atmospro.proIdentity.PrepareEnvironment")()

	result := make(map[string]string, len(environ))
	for k, v := range environ {
		result[k] = v
	}
	return result, nil
}

// PostAuthenticate is a no-op for the atmos/pro identity.
func (i *proIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}

// Logout is a no-op for the atmos/pro identity (session JWT lives in the keyring).
func (i *proIdentity) Logout(_ context.Context) error { return nil }

// CredentialsExist reports whether identity-managed credentials exist (none; managed by keyring).
func (i *proIdentity) CredentialsExist() (bool, error) { return false, nil }

// LoadCredentials returns nil (no identity-managed credential storage).
func (i *proIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}
