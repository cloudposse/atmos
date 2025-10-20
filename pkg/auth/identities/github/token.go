package github

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// tokenIdentity implements GitHub token identity.
// This identity represents GitHub access tokens, which can be obtained from multiple providers:
// - github/user provider (OAuth Device Flow)
// - github/app provider (GitHub App installation token)
// - github/oidc provider (GitHub Actions OIDC)
type tokenIdentity struct {
	name   string
	config *schema.Identity
}

// NewTokenIdentity creates a new GitHub token identity.
func NewTokenIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "github.NewTokenIdentity")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidAuthConfig, name)
	}
	if config.Kind != "github/token" {
		return nil, fmt.Errorf("%w: invalid identity kind for GitHub token: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	log.Debug("Creating GitHub token identity", "identity", name)

	return &tokenIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *tokenIdentity) Kind() string {
	return "github/token"
}

// GetProviderName returns the provider name for this identity.
func (i *tokenIdentity) GetProviderName() (string, error) {
	if i.config.Via == nil {
		return "", fmt.Errorf("%w: identity %q has no provider configured (via field is nil)", errUtils.ErrInvalidAuthConfig, i.name)
	}
	if i.config.Via.Provider == "" {
		return "", fmt.Errorf("%w: identity %q has empty provider name", errUtils.ErrInvalidAuthConfig, i.name)
	}
	return i.config.Via.Provider, nil
}

// Authenticate authenticates with the GitHub provider.
func (i *tokenIdentity) Authenticate(ctx context.Context, providerCreds types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "github.tokenIdentity.Authenticate")()

	// For GitHub token identity, the provider handles all the authentication.
	// The provider credentials are already the final GitHub token.

	if providerCreds == nil {
		return nil, fmt.Errorf("%w: provider credentials are nil for identity %q", errUtils.ErrAuthenticationFailed, i.name)
	}

	// Verify we got GitHub credentials (can be from any GitHub provider).
	switch creds := providerCreds.(type) {
	case *types.GitHubUserCredentials:
		log.Debug("GitHub token identity authenticated via user provider", "identity", i.name, "provider", creds.Provider)
		return creds, nil
	case *types.GitHubAppCredentials:
		log.Debug("GitHub token identity authenticated via app provider", "identity", i.name, "provider", creds.Provider)
		return creds, nil
	case *types.OIDCCredentials:
		log.Debug("GitHub token identity authenticated via OIDC provider", "identity", i.name, "provider", creds.Provider)
		return creds, nil
	default:
		return nil, fmt.Errorf("%w: expected GitHub credentials, got %T", errUtils.ErrAuthenticationFailed, providerCreds)
	}
}

// Validate validates the identity configuration.
func (i *tokenIdentity) Validate() error {
	// Verify provider is configured.
	if _, err := i.GetProviderName(); err != nil {
		return err
	}
	return nil
}

// Environment returns environment variables for this identity.
func (i *tokenIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Add environment variables from identity config.
	for _, envVar := range i.config.Env {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// PostAuthenticate performs any post-authentication setup.
func (i *tokenIdentity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	// No post-authentication setup needed for GitHub token identity.
	// Environment variables are set by the auth manager using BuildWhoamiInfo.
	return nil
}
