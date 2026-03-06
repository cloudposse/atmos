package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEKSTokenCmd_Help(t *testing.T) {
	assert.Equal(t, "eks-token", authEKSTokenCmd.Use)
	assert.Equal(t, "Generate an EKS bearer token for kubectl", authEKSTokenCmd.Short)
	assert.Contains(t, authEKSTokenCmd.Long, "ExecCredential")
}

func TestAuthEKSTokenCmd_HasFlags(t *testing.T) {
	_ = NewTestKit(t)

	// Verify --cluster-name flag exists.
	clusterFlag := authEKSTokenCmd.Flags().Lookup("cluster-name")
	require.NotNil(t, clusterFlag)
	assert.Equal(t, "", clusterFlag.DefValue)

	// Verify --region flag exists.
	regionFlag := authEKSTokenCmd.Flags().Lookup("region")
	require.NotNil(t, regionFlag)
	assert.Equal(t, "", regionFlag.DefValue)
}

func TestAuthEKSTokenCmd_NoArgs(t *testing.T) {
	// Command accepts no positional args.
	assert.Nil(t, authEKSTokenCmd.Args(authEKSTokenCmd, []string{}))
	assert.NotNil(t, authEKSTokenCmd.Args(authEKSTokenCmd, []string{"extra"}))
}

func TestAuthEKSTokenCmd_ParentIsAuthCmd(t *testing.T) {
	assert.NotNil(t, authEKSTokenCmd.Parent())
	if authEKSTokenCmd.Parent() != nil {
		assert.Equal(t, "auth", authEKSTokenCmd.Parent().Name())
	}
}

func TestAuthEKSTokenCmd_SilencesUsage(t *testing.T) {
	// Usage should be silenced since kubectl calls this automatically.
	assert.True(t, authEKSTokenCmd.SilenceUsage)
}

func TestAuthEKSTokenCmd_LongDescription(t *testing.T) {
	assert.Contains(t, authEKSTokenCmd.Long, "kubectl exec credential plugin")
	assert.Contains(t, authEKSTokenCmd.Long, "--cluster-name")
	assert.Contains(t, authEKSTokenCmd.Long, "--region")
	assert.Contains(t, authEKSTokenCmd.Long, "--identity")
	assert.Contains(t, authEKSTokenCmd.Long, "Examples:")
}

func TestResolveDefaultIdentity_NilConfig(t *testing.T) {
	result := resolveDefaultIdentity(nil)
	assert.Equal(t, "", result)
}

func TestResolveDefaultIdentity_EmptyIdentities(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{},
	}
	result := resolveDefaultIdentity(authConfig)
	assert.Equal(t, "", result)
}

func TestResolveDefaultIdentity_SingleIdentity(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"dev-admin": {Kind: "aws/user"},
		},
	}
	result := resolveDefaultIdentity(authConfig)
	assert.Equal(t, "dev-admin", result)
}

func TestResolveDefaultIdentity_MultipleIdentities(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"dev-admin":     {Kind: "aws/user"},
			"staging-admin": {Kind: "aws/user"},
		},
	}
	result := resolveDefaultIdentity(authConfig)
	// Multiple identities returns empty (can't auto-select).
	assert.Equal(t, "", result)
}

func TestExecCredentialAPIVersion(t *testing.T) {
	assert.Equal(t, "client.authentication.k8s.io/v1beta1", execCredentialAPIVersion)
}

func TestEKSTokenErrors(t *testing.T) {
	// Verify error constants exist and are usable.
	assert.NotNil(t, errUtils.ErrEKSTokenGeneration)
}
