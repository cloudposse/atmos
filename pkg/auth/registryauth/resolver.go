// Package registryauth binds OCI registry access to the existing Atmos auth lifecycle.
package registryauth

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RegistryCredentials supplies credentials scoped to one registry host.
// Auth identities can implement this without depending on an OCI client.
type RegistryCredentials interface {
	RegistryHost() string
	RegistryAuth() (username, password, identityToken, registryToken string)
}

// Manager is the authentication lifecycle needed by registry access.
type Manager interface {
	Authenticate(context.Context, string) (*types.WhoamiInfo, error)
	GetCachedCredentials(context.Context, string) (*types.WhoamiInfo, error)
	Logout(context.Context, string, bool) error
}

type cachedAuth struct {
	authenticator authn.Authenticator
	expires       time.Time
	session       [32]byte
}

// Resolver authenticates only exact configured hosts. Exchanged tokens are cached
// briefly against the live auth session; logout or rotation invalidates that cache.
type Resolver struct {
	config     *schema.AtmosConfiguration
	bindings   map[string]string
	mu         sync.Mutex
	cached     map[string]cachedAuth
	newManager func(*schema.AuthConfig, string) (Manager, error)
	exchange   func(context.Context, *integrations.IntegrationConfig, types.ICredentials) (*types.OCICredentials, error)
	prepare    Preparation
}

// New creates a resolver without loading credentials or starting authentication.
func New(config *schema.AtmosConfiguration, bindings map[string]string, options ...Option) *Resolver {
	resolver := &Resolver{config: config, bindings: maps.Clone(bindings), cached: make(map[string]cachedAuth), newManager: defaultManager, exchange: integrations.ExchangeRegistryCredentials}
	for _, option := range options {
		option(resolver)
	}
	return resolver
}

func defaultManager(config *schema.AuthConfig, path string) (Manager, error) {
	return auth.NewDefaultManager(config, path)
}

// Resolve authenticates explicitly bound hosts and leaves other hosts to the caller.
// A configured identity's failure is terminal, never an ambient-credential fallback.
func (r *Resolver) Resolve(ctx context.Context, resource authn.Resource) (authn.Authenticator, bool, error) {
	defer perf.Track(r.config, "registryauth.Resolver.Resolve")()

	host, err := normalizeHost(resource.RegistryStr())
	if err != nil {
		return nil, true, err
	}
	identity, found := r.bindings[host]
	if !found {
		return nil, false, nil
	}
	if err := authenticationCycle(ctx, identity); err != nil {
		return nil, true, err
	}
	if active, _ := ctx.Value(authenticationKey{}).(map[string]bool); len(active) > 0 {
		return nil, true, fmt.Errorf("%w: registry authentication cannot resolve another bound registry while authenticating", errUtils.ErrIdentityAuthFailed)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	credentials, err := r.authenticate(ctx, identity, host, false)
	if err != nil {
		delete(r.cached, host)
		return nil, true, err
	}
	return credentials, true, nil
}

// Authenticate logs in the selected identity and performs its registry exchange.
func Authenticate(ctx context.Context, config *schema.AtmosConfiguration, identity, host string, options ...Option) (authn.Authenticator, error) {
	defer perf.Track(config, "registryauth.Authenticate")()

	normalized, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}
	return New(config, nil, options...).authenticate(ctx, identity, normalized, true)
}

// Logout clears the selected identity through Atmos auth, including integration cleanup.
func Logout(ctx context.Context, config *schema.AtmosConfiguration, identity string, options ...Option) error {
	defer perf.Track(config, "registryauth.Logout")()

	selected, err := scopedConfig(&config.Auth, identity)
	if err != nil {
		return err
	}
	resolver := New(config, nil, options...)
	manager, cleanup, err := resolver.manager(ctx, selected)
	if err != nil {
		return err
	}
	defer cleanup()
	return manager.Logout(ctx, identity, true)
}

func (r *Resolver) authenticate(ctx context.Context, identity, host string, refresh bool) (authn.Authenticator, error) {
	ctx, err := guardAuthentication(ctx, identity)
	if err != nil {
		return nil, err
	}
	selected, err := scopedConfig(&r.config.Auth, identity)
	if err != nil {
		return nil, err
	}
	manager, cleanup, err := r.manager(ctx, selected)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	identityConfig := selected.Identities[identity]
	if err := validateDirectIdentity(&identityConfig, host); err != nil {
		return nil, err
	}
	whoami, err := r.authenticateIdentity(auth.ContextWithSkipIntegrations(ctx), manager, identity, refresh)
	if err != nil {
		return nil, err
	}
	return r.registryCredentials(ctx, selected, whoami, identity, host)
}

func (r *Resolver) authenticateIdentity(ctx context.Context, manager Manager, identity string, refresh bool) (*types.WhoamiInfo, error) {
	if refresh {
		return manager.Authenticate(ctx, identity)
	}
	cached, err := manager.GetCachedCredentials(ctx, identity)
	if err != nil || cached == nil {
		return manager.Authenticate(ctx, identity)
	}
	if cached.Credentials != nil {
		return cached, nil
	}
	if err := cached.Rehydrate(credentials.NewCredentialStoreWithConfig(&r.config.Auth)); err == nil && cached.Credentials != nil {
		return cached, nil
	}
	return manager.Authenticate(ctx, identity)
}
