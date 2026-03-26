package ambient

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ambientProviderName   = "ambient"
	logKeyIdentityAmbient = "identity"
)

// ambientIdentity implements a cloud-agnostic passthrough identity.
// It does nothing: no authentication, no credential clearing, no IMDS disabling.
// The environment is passed through unchanged to subprocesses.
type ambientIdentity struct {
	name   string
	config *schema.Identity
}

// NewAmbientIdentity creates a new cloud-agnostic ambient identity.
func NewAmbientIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "ambient.NewAmbientIdentity")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidAuthConfig, name)
	}
	if config.Kind != "ambient" {
		return nil, fmt.Errorf("%w: invalid identity kind for ambient: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	return &ambientIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *ambientIdentity) Kind() string {
	return "ambient"
}

// SetRealm sets the credential isolation realm for this identity.
// Ambient identities do not use realm-based isolation.
func (i *ambientIdentity) SetRealm(_ string) {
	// No-op: ambient identities don't store credentials.
}

// GetProviderName returns the provider name for this identity.
// Ambient identities are standalone and always return "ambient".
func (i *ambientIdentity) GetProviderName() (string, error) {
	return ambientProviderName, nil
}

// Authenticate is a no-op for ambient identities.
// Ambient identities do not resolve or manage credentials.
func (i *ambientIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "ambient.ambientIdentity.Authenticate")()

	log.Debug("Ambient identity authentication is a no-op", logKeyIdentityAmbient, i.name)
	return nil, nil
}

// Validate validates the identity configuration.
// Ambient identities have no required fields.
func (i *ambientIdentity) Validate() error {
	return nil
}

// Environment returns environment variables for this identity.
// Ambient identities do not set any environment variables.
func (i *ambientIdentity) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

// Paths returns credential files/directories used by this identity.
// Ambient identities do not use any credential files.
func (i *ambientIdentity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment returns the environment unchanged.
// This is the critical difference from other identity kinds: no credential clearing,
// no IMDS disabling, no file path overrides. Pure passthrough.
func (i *ambientIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "ambient.ambientIdentity.PrepareEnvironment")()

	// Create a copy to avoid mutating the input.
	result := make(map[string]string, len(environ))
	for k, v := range environ {
		result[k] = v
	}

	log.Debug("Ambient identity passing environment through unchanged", logKeyIdentityAmbient, i.name)
	return result, nil
}

// PostAuthenticate is a no-op for ambient identities.
func (i *ambientIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}

// Logout is a no-op for ambient identities.
func (i *ambientIdentity) Logout(_ context.Context) error {
	return nil
}

// CredentialsExist always returns true for ambient identities.
// Ambient credentials are assumed to always be available in the environment.
func (i *ambientIdentity) CredentialsExist() (bool, error) {
	return true, nil
}

// LoadCredentials returns nil for ambient identities.
// Credentials are resolved by the cloud SDK at runtime, not by Atmos.
func (i *ambientIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}

// IsStandaloneAmbientChain checks if the authentication chain represents a standalone ambient identity.
func IsStandaloneAmbientChain(chain []string, identities map[string]schema.Identity) bool {
	if len(chain) != 1 {
		return false
	}

	identityName := chain[0]
	if identity, exists := identities[identityName]; exists {
		return identity.Kind == "ambient"
	}

	return false
}

// AuthenticateStandaloneAmbient handles authentication for standalone ambient identities.
func AuthenticateStandaloneAmbient(ctx context.Context, identityName string, identities map[string]types.Identity) (types.ICredentials, error) {
	defer perf.Track(nil, "ambient.AuthenticateStandaloneAmbient")()

	log.Debug("Authenticating ambient identity directly", logKeyIdentityAmbient, identityName)

	identity, exists := identities[identityName]
	if !exists {
		return nil, fmt.Errorf("%w: ambient identity %q not found", errUtils.ErrInvalidAuthConfig, identityName)
	}

	// Ambient identities return nil credentials — they don't manage credentials.
	credentials, err := identity.Authenticate(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: ambient identity %q authentication failed: %w", errUtils.ErrAuthenticationFailed, identityName, err)
	}

	log.Debug("Ambient identity authenticated successfully", logKeyIdentityAmbient, identityName)
	return credentials, nil
}
