package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
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

func TestExportAWSCredsToEnv_Success(t *testing.T) {
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDH...",
		Region:          "us-east-2",
	}

	// Set AWS_PROFILE to verify it gets cleared.
	t.Setenv("AWS_PROFILE", "some-profile")

	err := exportAWSCredsToEnv(creds)
	require.NoError(t, err)

	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	assert.Equal(t, "FwoGZXIvYXdzEBYaDH...", os.Getenv("AWS_SESSION_TOKEN"))
	assert.Equal(t, "us-east-2", os.Getenv("AWS_REGION"))
	assert.Equal(t, "us-east-2", os.Getenv("AWS_DEFAULT_REGION"))
	assert.Equal(t, "", os.Getenv("AWS_PROFILE"))

	// Clean up (t.Setenv handles AWS_PROFILE, manually clean others).
	t.Cleanup(func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_SESSION_TOKEN")
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("AWS_DEFAULT_REGION")
	})
}

func TestExportAWSCredsToEnv_PartialCreds(t *testing.T) {
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG",
		// No session token, no region.
	}

	err := exportAWSCredsToEnv(creds)
	require.NoError(t, err)

	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG", os.Getenv("AWS_SECRET_ACCESS_KEY"))

	t.Cleanup(func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	})
}

func TestExportAWSCredsToEnv_NonAWSCreds(t *testing.T) {
	// Passing a non-AWS credential type should return error.
	err := exportAWSCredsToEnv(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
}

func TestExecCredentialStruct(t *testing.T) {
	cred := execCredential{
		APIVersion: execCredentialAPIVersion,
		Kind:       "ExecCredential",
		Status: execCredentialStatus{
			ExpirationTimestamp: "2026-03-16T12:00:00Z",
			Token:               "k8s-aws-v1.test-token",
		},
	}

	assert.Equal(t, "client.authentication.k8s.io/v1beta1", cred.APIVersion)
	assert.Equal(t, "ExecCredential", cred.Kind)
	assert.Equal(t, "k8s-aws-v1.test-token", cred.Status.Token)
}
