package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPostAuthenticate_AWSRegionFallsBackToDefault verifies AWS_DEFAULT_REGION is
// used when AWS_REGION is absent from the resolved profile.
func TestPostAuthenticate_AWSRegionFallsBackToDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	id := newAWSIdentity(t)
	id.SetRealm("test-realm")
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{
		"AWS_ENDPOINT_URL":   "http://localhost:34566",
		"AWS_DEFAULT_REGION": "eu-central-1",
	}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  ac,
		StackInfo:    &schema.ConfigAndStacksInfo{Stack: "dev"},
		ProviderName: "local-aws",
		IdentityName: "local-aws",
	}))
	require.NotNil(t, ac.AWS)
	assert.Equal(t, "eu-central-1", ac.AWS.Region, "falls back to AWS_DEFAULT_REGION")
}

// TestPostAuthenticate_AWSResolverErrorPropagates verifies a resolver failure
// surfaces from setAWSAuthContext rather than being swallowed.
func TestPostAuthenticate_AWSResolverErrorPropagates(t *testing.T) {
	id := newAWSIdentity(t)
	id.SetStack("dev")
	id.SetEmulatorResolver(&fakeResolver{err: errUtils.ErrEmulatorNotRunning})

	ac := &schema.AuthContext{}
	err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac, ProviderName: "local-aws", IdentityName: "local-aws",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
	assert.Nil(t, ac.AWS, "auth context left unpopulated on resolver error")
}
