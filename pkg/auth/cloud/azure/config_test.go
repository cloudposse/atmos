package azure

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestBuildAzureCredentialFromCreds_Success(t *testing.T) {
	creds := &types.AzureCredentials{AccessToken: "test-token"}

	tokenCred, err := BuildAzureCredentialFromCreds(creds)
	require.NoError(t, err)
	require.NotNil(t, tokenCred)

	token, err := tokenCred.GetToken(t.Context(), policy.TokenRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, "test-token", token.Token)
}

func TestBuildAzureCredentialFromCreds_WrongType(t *testing.T) {
	_, err := BuildAzureCredentialFromCreds(&types.AWSCredentials{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestBuildARMClientOptions(t *testing.T) {
	tests := []struct {
		name     string
		cloudEnv *CloudEnvironment
		want     string
	}{
		{"nil defaults to public", nil, cloud.AzurePublic.ActiveDirectoryAuthorityHost},
		{"public", GetCloudEnvironment("public"), cloud.AzurePublic.ActiveDirectoryAuthorityHost},
		{"usgovernment", GetCloudEnvironment("usgovernment"), cloud.AzureGovernment.ActiveDirectoryAuthorityHost},
		{"china", GetCloudEnvironment("china"), cloud.AzureChina.ActiveDirectoryAuthorityHost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := BuildARMClientOptions(tt.cloudEnv)
			require.NotNil(t, opts)
			assert.Equal(t, tt.want, opts.Cloud.ActiveDirectoryAuthorityHost)
		})
	}
}
