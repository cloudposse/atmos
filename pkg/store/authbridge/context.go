package authbridge

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// resolvedContext exposes credentials already authenticated by the resolver.
// Store reads must not reauthenticate outside the invocation's success/failure cache.
type resolvedContext struct{ auth *schema.AuthContext }

func (r *resolvedContext) ResolveAWSAuthContext(_ context.Context, _ string) (*store.AWSAuthConfig, error) {
	defer perf.Track(nil, "authbridge.resolvedContext.ResolveAWSAuthContext")()

	if r.auth == nil || r.auth.AWS == nil {
		return nil, errUtils.ErrAuthenticationUnavailable
	}
	a := r.auth.AWS
	return &store.AWSAuthConfig{CredentialsFile: a.CredentialsFile, ConfigFile: a.ConfigFile, Profile: a.Profile, Region: a.Region, EndpointURL: a.EndpointURL}, nil
}

func (r *resolvedContext) ResolveAzureAuthContext(_ context.Context, _ string) (*store.AzureAuthConfig, error) {
	defer perf.Track(nil, "authbridge.resolvedContext.ResolveAzureAuthContext")()

	if r.auth == nil || r.auth.Azure == nil {
		return nil, errUtils.ErrAuthenticationUnavailable
	}
	a := r.auth.Azure
	return &store.AzureAuthConfig{CredentialsFile: a.CredentialsFile, SubscriptionID: a.SubscriptionID, TenantID: a.TenantID, UseOIDC: a.UseOIDC, ClientID: a.ClientID, TokenFilePath: a.TokenFilePath}, nil
}

func (r *resolvedContext) ResolveGCPAuthContext(_ context.Context, _ string) (*store.GCPAuthConfig, error) {
	defer perf.Track(nil, "authbridge.resolvedContext.ResolveGCPAuthContext")()

	if r.auth == nil || r.auth.GCP == nil {
		return nil, errUtils.ErrAuthenticationUnavailable
	}
	a := r.auth.GCP
	return &store.GCPAuthConfig{CredentialsFile: a.CredentialsFile, ProjectID: a.ProjectID, AccessToken: a.AccessToken, TokenExpiry: a.TokenExpiry}, nil
}

// NewResolvedContext exposes already-authenticated credentials without reauthenticating.
func NewResolvedContext(auth *schema.AuthContext) store.AuthContextResolver {
	return &resolvedContext{auth: auth}
}

// NewContextResolver adapts an invocation-scoped credential resolver for stores and secrets.
func NewContextResolver(resolve func(context.Context, string) (*schema.AuthContext, error)) store.AuthContextResolver {
	return &lazyContext{resolve: resolve}
}

type lazyContext struct {
	resolve func(context.Context, string) (*schema.AuthContext, error)
}

func (r *lazyContext) ResolveAWSAuthContext(ctx context.Context, identity string) (*store.AWSAuthConfig, error) {
	defer perf.Track(nil, "authbridge.lazyContext.ResolveAWSAuthContext")()
	auth, err := r.resolve(ctx, identity)
	if err != nil {
		return nil, err
	}
	return (&resolvedContext{auth: auth}).ResolveAWSAuthContext(ctx, identity)
}

func (r *lazyContext) ResolveAzureAuthContext(ctx context.Context, identity string) (*store.AzureAuthConfig, error) {
	defer perf.Track(nil, "authbridge.lazyContext.ResolveAzureAuthContext")()
	auth, err := r.resolve(ctx, identity)
	if err != nil {
		return nil, err
	}
	return (&resolvedContext{auth: auth}).ResolveAzureAuthContext(ctx, identity)
}

func (r *lazyContext) ResolveGCPAuthContext(ctx context.Context, identity string) (*store.GCPAuthConfig, error) {
	defer perf.Track(nil, "authbridge.lazyContext.ResolveGCPAuthContext")()
	auth, err := r.resolve(ctx, identity)
	if err != nil {
		return nil, err
	}
	return (&resolvedContext{auth: auth}).ResolveGCPAuthContext(ctx, identity)
}
