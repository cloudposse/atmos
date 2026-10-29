package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestECRExchangeReturnsMintedCredentialsWithoutSharedDockerState(t *testing.T) {
	originalToken, originalDocker := ecrGetAuthToken, ecrDockerConfigFactory
	t.Cleanup(func() { ecrGetAuthToken, ecrDockerConfigFactory = originalToken, originalDocker })
	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		t.Fatal("exchange must not access shared Docker credentials")
		return nil, nil
	}
	directory := t.TempDir()
	t.Setenv("DOCKER_CONFIG", directory)
	before := []byte(`{"auths":{"123456789012.dkr.ecr.us-east-1.amazonaws.com":{"auth":"other-user-token"}}}`)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "config.json"), before, 0o600))
	input := &types.AWSCredentials{AccessKeyID: "selected-identity"}
	expiry := time.Now().Add(time.Hour)
	ecrGetAuthToken = func(ctx context.Context, actual types.ICredentials, account, region string) (*awsCloud.ECRAuthResult, error) {
		require.Same(t, input, actual)
		require.Equal(t, "123456789012", account)
		require.Equal(t, "us-east-1", region)
		return &awsCloud.ECRAuthResult{Username: "AWS", Password: "minted-selected-token", ExpiresAt: expiry}, nil
	}
	integration := &ECRIntegration{registry: &schema.Registry{AccountID: "123456789012", Region: "us-east-1"}}
	result, err := integration.ExchangeCredentials(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, &types.OCICredentials{Registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com", Username: "AWS", Password: "minted-selected-token", Expiration: &expiry}, result)
	after, err := os.ReadFile(filepath.Join(directory, "config.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	ecrGetAuthToken = func(context.Context, types.ICredentials, string, string) (*awsCloud.ECRAuthResult, error) {
		return &awsCloud.ECRAuthResult{Username: "AWS", Password: "without-expiry"}, nil
	}
	result, err = integration.ExchangeCredentials(t.Context(), input)
	require.NoError(t, err)
	require.Nil(t, result.Expiration)
	ecrGetAuthToken = func(context.Context, types.ICredentials, string, string) (*awsCloud.ECRAuthResult, error) {
		return nil, nil
	}
	_, err = integration.ExchangeCredentials(t.Context(), input)
	require.ErrorIs(t, err, errUtils.ErrIdentityCredentialsNone)
}

func TestECRPublicExchangeReturnsMintedCredentials(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("DOCKER_CONFIG", directory)
	expiry := time.Now().Add(time.Hour)
	input := &types.AWSCredentials{AccessKeyID: "selected-identity"}
	integration := &ECRPublicIntegration{getAuthToken: func(ctx context.Context, actual types.ICredentials) (*awsCloud.ECRPublicAuthResult, error) {
		require.Same(t, input, actual)
		return &awsCloud.ECRPublicAuthResult{Username: "AWS", Password: "minted-public-token", ExpiresAt: expiry}, nil
	}}
	result, err := integration.ExchangeCredentials(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, &types.OCICredentials{Registry: "public.ecr.aws", Username: "AWS", Password: "minted-public-token", Expiration: &expiry}, result)
	_, err = os.Stat(filepath.Join(directory, "config.json"))
	require.True(t, os.IsNotExist(err), "an exchange must not publish shared Docker credentials")
	integration.getAuthToken = func(context.Context, types.ICredentials) (*awsCloud.ECRPublicAuthResult, error) {
		return &awsCloud.ECRPublicAuthResult{Username: "AWS", Password: "unknown-expiry"}, nil
	}
	result, err = integration.ExchangeCredentials(t.Context(), input)
	require.NoError(t, err)
	require.Nil(t, result.Expiration)
	integration.getAuthToken = func(context.Context, types.ICredentials) (*awsCloud.ECRPublicAuthResult, error) { return nil, nil }
	_, err = integration.ExchangeCredentials(t.Context(), input)
	require.ErrorIs(t, err, errUtils.ErrIdentityCredentialsNone)
}
