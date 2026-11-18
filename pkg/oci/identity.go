package oci

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	orasauth "oras.land/oras-go/v2/registry/remote/auth"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
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
	if matched {
		// An authenticator that resolves to an empty credential would otherwise send requests
		// unauthenticated, so it is checked whenever it is asked to authorize.
		return nonEmptyCredential{Authenticator: auth}, true, nil
	}
	return auth, matched, nil
}

// nonEmptyCredential makes a matched identity fail closed: an authorization that carries no
// credential is an error, never an anonymous request. The check runs on every authorization, so a
// token that refreshes into an empty credential is rejected too.
type nonEmptyCredential struct {
	authn.Authenticator
}

func (n nonEmptyCredential) Authorization() (*authn.AuthConfig, error) {
	defer perf.Track(nil, "oci.nonEmptyCredential.Authorization")()

	config, err := n.Authenticator.Authorization()
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: the bound identity resolved to no credential", errUtils.ErrOCIRegistry)
	}
	credential, err := credentialFrom(authn.FromConfig(*config))
	if err != nil {
		return nil, err
	}
	if credential == orasauth.EmptyCredential {
		return nil, fmt.Errorf("%w: the bound identity resolved to an empty credential", errUtils.ErrOCIRegistry)
	}
	if !canSend(credential) {
		return nil, fmt.Errorf("%w: the bound identity resolved to an incomplete credential: a token, or both a username and a password, is required", errUtils.ErrOCIRegistry)
	}
	return config, nil
}

// canSend reports whether the registry client can turn the credential into an Authorization header: a
// token, or a username together with a password. A username or password on its own is rejected
// instead of letting the request go out unauthenticated.
func canSend(credential orasauth.Credential) bool {
	return credential.AccessToken != "" || credential.RefreshToken != "" || (credential.Username != "" && credential.Password != "")
}
