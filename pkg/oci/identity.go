package oci

import (
	"context"
	"errors"

	"github.com/google/go-containerregistry/pkg/authn"

	errUtils "github.com/cloudposse/atmos/errors"
)

// AuthenticationResolver resolves explicitly bound identities before ambient credentials.
// The matched result distinguishes an unbound host from a failed configured identity.
type AuthenticationResolver interface {
	Resolve(context.Context, authn.Resource) (authn.Authenticator, bool, error)
}

// WithAuthenticationResolver binds a context-aware identity resolver to this client.
// Configured identities never fall through to Docker helpers or environment credentials.
//
//nolint:lintroller // Functional option only.
func WithAuthenticationResolver(resolver AuthenticationResolver) ClientOption {
	return func(client *Client) { client.identity = resolver }
}

func (c *Client) authentication(ctx context.Context, resource authn.Resource) (authn.Authenticator, error) {
	bound, matched, err := c.boundAuthentication(ctx, resource)
	if err != nil || matched {
		return bound, err
	}
	auth, _, err := ResolveAuthentication(c.config, c.keychain, resource)
	if err != nil {
		return nil, err
	}
	if auth == authn.Anonymous && c.fallback != nil {
		auth, err = c.fallback.Resolve(resource)
		if err != nil {
			return nil, errors.Join(errUtils.ErrOCIRegistry, err)
		}
	}
	return auth, nil
}

func (c *Client) boundAuthentication(ctx context.Context, resource authn.Resource) (authn.Authenticator, bool, error) {
	if c.identity == nil {
		return nil, false, nil
	}
	auth, matched, err := c.identity.Resolve(ctx, resource)
	if err != nil {
		return nil, true, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	if matched && (auth == nil || auth == authn.Anonymous) {
		return nil, true, errUtils.ErrOCIRegistry
	}
	return auth, matched, nil
}
