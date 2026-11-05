package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAWSAmbientPostAuthenticateContext(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("ambient", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)
	for _, region := range []string{"eu-west-1", ""} {
		t.Run("region="+region, func(t *testing.T) {
			previous := &schema.AWSAuthContext{CredentialsFile: "stale-credentials", ConfigFile: "stale-config", Profile: "managed", EndpointURL: "http://stale"}
			azure := &schema.AzureAuthContext{Profile: "azure"}
			auth := &schema.AuthContext{AWS: previous, Azure: azure}
			require.NoError(t, identity.PostAuthenticate(t.Context(), &types.PostAuthenticateParams{
				AuthContext: auth, Credentials: &types.AWSCredentials{Region: region},
			}))
			assert.Equal(t, &schema.AWSAuthContext{Region: region}, auth.AWS)
			assert.Same(t, azure, auth.Azure)
			assert.Equal(t, "managed", previous.Profile)
		})
	}
}

func TestAWSAmbientPostAuthenticateWithoutContextOrCredentials(t *testing.T) {
	identity, err := NewAWSAmbientIdentity("ambient", &schema.Identity{Kind: "aws/ambient"})
	require.NoError(t, err)
	for _, params := range []*types.PostAuthenticateParams{
		nil,
		{},
		{Credentials: &types.AWSCredentials{}},
		{AuthContext: &schema.AuthContext{}},
		{AuthContext: &schema.AuthContext{}, Credentials: (*types.AWSCredentials)(nil)},
	} {
		require.NoError(t, identity.PostAuthenticate(t.Context(), params))
		if params != nil && params.AuthContext != nil {
			assert.Nil(t, params.AuthContext.AWS)
		}
	}
}
