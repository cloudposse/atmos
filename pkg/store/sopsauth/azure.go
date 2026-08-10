package sopsauth

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/getsops/sops/v3/azkv"

	"github.com/cloudposse/atmos/pkg/perf"
)

// errEmptyAzureAuthContext indicates the resolver returned a nil Azure auth context without an error.
var errEmptyAzureAuthContext = errors.New("resolved empty Azure auth context")

// AzureKV resolves the identity's Azure credentials and wraps them as a getsops Key Vault token
// credential. It mirrors the Azure Key Vault store: the default Azure credential chain is used with
// a tenant hint from the resolved identity.
func (b *resolverBuilder) AzureKV(ctx context.Context, identity string) (AzureApplier, error) {
	defer perf.Track(nil, "sopsauth.AzureKV")()

	authContext, err := b.resolver.ResolveAzureAuthContext(ctx, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Azure auth context for SOPS identity %q: %w", identity, err)
	}
	if authContext == nil {
		return nil, fmt.Errorf("%w for SOPS identity %q", errEmptyAzureAuthContext, identity)
	}

	options := &azidentity.DefaultAzureCredentialOptions{}
	if authContext.TenantID != "" {
		options.TenantID = authContext.TenantID
	}
	cred, err := azidentity.NewDefaultAzureCredential(options)
	if err != nil {
		return nil, fmt.Errorf("failed to build Azure credential for SOPS identity %q: %w", identity, err)
	}
	return azkv.NewTokenCredential(cred), nil
}
