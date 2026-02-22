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

// Resolver implements store.AuthContextResolver by delegating to an AuthManager.
type Resolver struct {
	authManager types.AuthManager
	stackInfo   *schema.ConfigAndStacksInfo
}

// Verify that Resolver implements store.AuthContextResolver.
var _ store.AuthContextResolver = (*Resolver)(nil)

// NewResolver creates a new Resolver that uses the provided AuthManager to resolve
// identity names to cloud-specific auth contexts.
func NewResolver(authManager types.AuthManager, stackInfo *schema.ConfigAndStacksInfo) *Resolver {
	return &Resolver{
		authManager: authManager,
		stackInfo:   stackInfo,
	}
}

// ResolveAWSAuthContext authenticates the named identity and returns AWS credentials.
func (r *Resolver) ResolveAWSAuthContext(ctx context.Context, identityName string) (*store.AWSAuthConfig, error) {
	defer perf.Track(nil, "authbridge.ResolveAWSAuthContext")()

	log.Debug("Resolving AWS auth context for store identity", "identity", identityName)

	_, err := r.authManager.Authenticate(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate identity %q for store: %w", identityName, err)
	}

	// After authentication, the auth context is populated in stackInfo.
	if r.stackInfo == nil || r.stackInfo.AuthContext == nil || r.stackInfo.AuthContext.AWS == nil {
		return nil, fmt.Errorf("%w: AWS auth context not available after authenticating identity %q", store.ErrAuthContextNotAvailable, identityName)
	}

	aws := r.stackInfo.AuthContext.AWS

	return &store.AWSAuthConfig{
		CredentialsFile: aws.CredentialsFile,
		ConfigFile:      aws.ConfigFile,
		Profile:         aws.Profile,
		Region:          aws.Region,
	}, nil
}

// ResolveAzureAuthContext authenticates the named identity and returns Azure credentials.
func (r *Resolver) ResolveAzureAuthContext(ctx context.Context, identityName string) (*store.AzureAuthConfig, error) {
	defer perf.Track(nil, "authbridge.ResolveAzureAuthContext")()

	log.Debug("Resolving Azure auth context for store identity", "identity", identityName)

	_, err := r.authManager.Authenticate(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate identity %q for store: %w", identityName, err)
	}

	if r.stackInfo == nil || r.stackInfo.AuthContext == nil || r.stackInfo.AuthContext.Azure == nil {
		return nil, fmt.Errorf("%w: Azure auth context not available after authenticating identity %q", store.ErrAuthContextNotAvailable, identityName)
	}

	azure := r.stackInfo.AuthContext.Azure

	return &store.AzureAuthConfig{
		TenantID: azure.TenantID,
		UseOIDC:  azure.UseOIDC,
		ClientID: azure.ClientID,
	}, nil
}

// ResolveGCPAuthContext authenticates the named identity and returns GCP credentials.
func (r *Resolver) ResolveGCPAuthContext(ctx context.Context, identityName string) (*store.GCPAuthConfig, error) {
	defer perf.Track(nil, "authbridge.ResolveGCPAuthContext")()

	log.Debug("Resolving GCP auth context for store identity", "identity", identityName)

	_, err := r.authManager.Authenticate(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate identity %q for store: %w", identityName, err)
	}

	if r.stackInfo == nil || r.stackInfo.AuthContext == nil || r.stackInfo.AuthContext.GCP == nil {
		return nil, fmt.Errorf("%w: GCP auth context not available after authenticating identity %q", store.ErrAuthContextNotAvailable, identityName)
	}

	gcpCtx := r.stackInfo.AuthContext.GCP

	return &store.GCPAuthConfig{
		CredentialsFile: gcpCtx.CredentialsFile,
	}, nil
}
