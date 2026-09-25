package authbridge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestContextAdaptersPreserveProviderCredentials(t *testing.T) {
	expiry := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	auth := &schema.AuthContext{
		AWS:   &schema.AWSAuthContext{CredentialsFile: "aws-credentials", ConfigFile: "aws-config", Profile: "selected", Region: "us-east-1", EndpointURL: "http://localhost:4566"},
		Azure: &schema.AzureAuthContext{CredentialsFile: "azure-credentials", SubscriptionID: "subscription", TenantID: "tenant", UseOIDC: true, ClientID: "client", TokenFilePath: "token-file"},
		GCP:   &schema.GCPAuthContext{CredentialsFile: "gcp-credentials", ProjectID: "project", AccessToken: "test-token", TokenExpiry: expiry},
	}
	for _, lazy := range []bool{false, true} {
		t.Run(map[bool]string{false: "resolved", true: "lazy"}[lazy], func(t *testing.T) {
			calls := 0
			resolver := NewResolvedContext(auth)
			if lazy {
				resolver = NewContextResolver(func(ctx context.Context, identity string) (*schema.AuthContext, error) {
					require.Equal(t, t.Context(), ctx)
					require.Equal(t, "selected", identity)
					calls++
					return auth, nil
				})
			}
			require.Zero(t, calls, "construction must not resolve credentials")
			aws, err := resolver.ResolveAWSAuthContext(t.Context(), "selected")
			require.NoError(t, err)
			require.Equal(t, &store.AWSAuthConfig{CredentialsFile: "aws-credentials", ConfigFile: "aws-config", Profile: "selected", Region: "us-east-1", EndpointURL: "http://localhost:4566"}, aws)
			azure, err := resolver.ResolveAzureAuthContext(t.Context(), "selected")
			require.NoError(t, err)
			require.Equal(t, &store.AzureAuthConfig{CredentialsFile: "azure-credentials", SubscriptionID: "subscription", TenantID: "tenant", UseOIDC: true, ClientID: "client", TokenFilePath: "token-file"}, azure)
			gcp, err := resolver.ResolveGCPAuthContext(t.Context(), "selected")
			require.NoError(t, err)
			require.Equal(t, &store.GCPAuthConfig{CredentialsFile: "gcp-credentials", ProjectID: "project", AccessToken: "test-token", TokenExpiry: expiry}, gcp)
			if lazy {
				require.Equal(t, 3, calls)
			}
		})
	}
}

func TestContextAdaptersRejectMissingCredentials(t *testing.T) {
	for _, auth := range []*schema.AuthContext{nil, {}} {
		resolver := NewResolvedContext(auth)
		aws, err := resolver.ResolveAWSAuthContext(t.Context(), "selected")
		require.Nil(t, aws)
		require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
		azure, err := resolver.ResolveAzureAuthContext(t.Context(), "selected")
		require.Nil(t, azure)
		require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
		gcp, err := resolver.ResolveGCPAuthContext(t.Context(), "selected")
		require.Nil(t, gcp)
		require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
	}
}

func TestLazyContextPreservesResolutionErrors(t *testing.T) {
	resolver := NewContextResolver(func(context.Context, string) (*schema.AuthContext, error) {
		return nil, errUtils.ErrInvalidAuthConfig
	})
	aws, err := resolver.ResolveAWSAuthContext(t.Context(), "selected")
	require.Nil(t, aws)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	azure, err := resolver.ResolveAzureAuthContext(t.Context(), "selected")
	require.Nil(t, azure)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	gcp, err := resolver.ResolveGCPAuthContext(t.Context(), "selected")
	require.Nil(t, gcp)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}
