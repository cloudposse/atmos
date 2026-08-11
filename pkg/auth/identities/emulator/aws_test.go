package emulator

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPostAuthenticate_AWSPopulatesAuthContext verifies an aws/emulator identity
// writes a static-credentials file and fills AuthContext.AWS with the live
// emulator endpoint — the contract in-process store/secret SDK clients rely on.
func TestPostAuthenticate_AWSPopulatesAuthContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	id := newAWSIdentity(t)
	id.SetRealm("test-realm")
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{
		"AWS_ENDPOINT_URL":      "http://localhost:34566",
		"AWS_REGION":            "us-west-2",
		"AWS_ACCESS_KEY_ID":     "test",
		"AWS_SECRET_ACCESS_KEY": "test",
	}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  ac,
		StackInfo:    &schema.ConfigAndStacksInfo{Stack: "dev"},
		ProviderName: "local-aws",
		IdentityName: "local-aws",
	}))

	require.NotNil(t, ac.AWS)
	assert.Equal(t, "http://localhost:34566", ac.AWS.EndpointURL)
	assert.Equal(t, "us-west-2", ac.AWS.Region)
	assert.Equal(t, "local-aws", ac.AWS.Profile)
	require.NotEmpty(t, ac.AWS.CredentialsFile)

	body, err := os.ReadFile(ac.AWS.CredentialsFile)
	require.NoError(t, err)
	assert.Contains(t, string(body), "test", "static dummy credentials written to the shared credentials file")
}

// TestPostAuthenticate_ResolvesWithoutStack verifies that project-scoped emulator
// identities resolve their configured emulator even when no stack context exists.
func TestPostAuthenticate_ResolvesWithoutStack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	id := newAWSIdentity(t)
	resolver := &fakeResolver{env: map[string]string{"AWS_ENDPOINT_URL": "http://localhost:1", "AWS_REGION": "us-east-1"}}
	id.SetEmulatorResolver(resolver)

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac, ProviderName: "local-aws", IdentityName: "local-aws",
	}))
	require.NotNil(t, ac.AWS)
	assert.Equal(t, "local/aws", resolver.gotName)
}

func TestPostAuthenticate_UsesProjectScopedIdentityWithoutStack(t *testing.T) {
	id := newAWSIdentity(t)
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{"AWS_ENDPOINT_URL": "x"}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac, ProviderName: "local-aws", IdentityName: "local-aws",
	}))
	assert.NotNil(t, ac.AWS)
}

// TestPostAuthenticate_SkipsForKubernetesAndNilResolver verifies non-AWS targets and
// a missing resolver are no-ops (kubernetes is consumed via KUBECONFIG instead).
func TestPostAuthenticate_SkipsForKubernetesAndNilResolver(t *testing.T) {
	k8s, err := New("local-k8s", &schema.Identity{Kind: types.IdentityKindKubernetesEmulator, Emulator: "k3s"})
	require.NoError(t, err)
	k8s.SetEmulatorResolver(&fakeResolver{})

	ac := &schema.AuthContext{}
	require.NoError(t, k8s.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac, StackInfo: &schema.ConfigAndStacksInfo{Stack: "dev"},
	}))
	assert.Nil(t, ac.AWS, "kubernetes emulator contributes no AWS auth context")

	// Nil resolver must not panic and must leave the context unpopulated.
	id := newAWSIdentity(t)
	ac2 := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac2, StackInfo: &schema.ConfigAndStacksInfo{Stack: "dev"},
	}))
	assert.Nil(t, ac2.AWS)
}
