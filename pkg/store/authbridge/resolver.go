// Package authbridge provides an implementation of store.AuthContextResolver that bridges
// the store package with the auth system. This package exists as a sub-package of store
// to avoid circular dependencies: pkg/config imports pkg/store, pkg/auth imports pkg/config,
// but pkg/store never imports authbridge, so authbridge can safely import pkg/auth.
package authbridge

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

const logIdentityKey = "identity"

// Resolver implements store.AuthContextResolver by delegating to an AuthManager.
type Resolver struct {
	authManager types.AuthManager
	// stackInfo is retained for callers and diagnostics. Store identities are resolved through
	// AuthenticateIsolated, so the resolver never reads or writes it.
	stackInfo *schema.ConfigAndStacksInfo
}

// Verify that Resolver implements store.AuthContextResolver.
var (
	_ store.AuthContextResolver            = (*Resolver)(nil)
	_ store.EnvironmentAuthContextResolver = (*Resolver)(nil)
)

// NewResolver creates a new Resolver that uses the provided AuthManager to resolve
// identity names to cloud-specific auth contexts.
func NewResolver(authManager types.AuthManager, stackInfo *schema.ConfigAndStacksInfo) *Resolver {
	return &Resolver{
		authManager: authManager,
		stackInfo:   stackInfo,
	}
}

// authenticateIsolated authenticates the named identity on a manager separate from the
// component's shared manager and returns the resulting auth context.
//
// Isolated authentication returns a context owned by a separate manager, so the component's
// authentication chain and credentials are never changed by a store read. Authenticating on the
// shared manager would overwrite its chain and auth context, and later nested !terraform.state
// reads would inherit the store identity (issue #3544).
func (r *Resolver) authenticateIsolated(ctx context.Context, identityName string) (*schema.AuthContext, error) {
	authCtx, err := r.authManager.AuthenticateIsolated(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate identity %q for store: %w", identityName, err)
	}

	return authCtx, nil
}

// ResolveAWSAuthContext authenticates the named identity and returns AWS credentials.
func (r *Resolver) ResolveAWSAuthContext(ctx context.Context, identityName string) (*store.AWSAuthConfig, error) {
	defer perf.Track(nil, "authbridge.ResolveAWSAuthContext")()

	log.Debug("Resolving AWS auth context for store identity", logIdentityKey, identityName)

	authCtx, err := r.authenticateIsolated(ctx, identityName)
	if err != nil {
		return nil, err
	}
	if authCtx == nil || authCtx.AWS == nil {
		return nil, fmt.Errorf("%w: AWS auth context not available after authenticating identity %q", store.ErrAuthContextNotAvailable, identityName)
	}

	return (&resolvedContext{auth: authCtx}).ResolveAWSAuthContext(ctx, identityName)
}

// ResolveAzureAuthContext authenticates the named identity and returns Azure credentials.
func (r *Resolver) ResolveAzureAuthContext(ctx context.Context, identityName string) (*store.AzureAuthConfig, error) {
	defer perf.Track(nil, "authbridge.ResolveAzureAuthContext")()

	log.Debug("Resolving Azure auth context for store identity", logIdentityKey, identityName)

	authCtx, err := r.authenticateIsolated(ctx, identityName)
	if err != nil {
		return nil, err
	}
	if authCtx == nil || authCtx.Azure == nil {
		return nil, fmt.Errorf("%w: Azure auth context not available after authenticating identity %q", store.ErrAuthContextNotAvailable, identityName)
	}

	return (&resolvedContext{auth: authCtx}).ResolveAzureAuthContext(ctx, identityName)
}

// ResolveGCPAuthContext authenticates the named identity and returns GCP credentials.
func (r *Resolver) ResolveGCPAuthContext(ctx context.Context, identityName string) (*store.GCPAuthConfig, error) {
	defer perf.Track(nil, "authbridge.ResolveGCPAuthContext")()

	log.Debug("Resolving GCP auth context for store identity", logIdentityKey, identityName)

	authCtx, err := r.authenticateIsolated(ctx, identityName)
	if err != nil {
		return nil, err
	}
	if authCtx == nil || authCtx.GCP == nil {
		return nil, fmt.Errorf("%w: GCP auth context not available after authenticating identity %q", store.ErrAuthContextNotAvailable, identityName)
	}

	return (&resolvedContext{auth: authCtx}).ResolveGCPAuthContext(ctx, identityName)
}

// ResolveEnvironmentAuthContext authenticates the named identity and returns native environment credentials.
func (r *Resolver) ResolveEnvironmentAuthContext(ctx context.Context, identityName string) (*store.EnvironmentAuthConfig, error) {
	defer perf.Track(nil, "authbridge.ResolveEnvironmentAuthContext")()

	log.Debug("Resolving environment auth context for store identity", logIdentityKey, identityName)

	authCtx, err := r.authenticateIsolated(ctx, identityName)
	if err != nil {
		return nil, err
	}
	if authCtx == nil || len(authCtx.Environment) == 0 {
		return nil, fmt.Errorf("%w: environment auth context not available after authenticating identity %q", store.ErrAuthContextNotAvailable, identityName)
	}

	return (&resolvedContext{auth: authCtx}).ResolveEnvironmentAuthContext(ctx, identityName)
}
