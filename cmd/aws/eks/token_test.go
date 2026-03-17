package eks

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTokenCmd_Help(t *testing.T) {
	assert.Equal(t, "token", tokenCmd.Use)
	assert.Equal(t, "Generate an EKS bearer token for kubectl", tokenCmd.Short)
	assert.Contains(t, tokenCmd.Long, "ExecCredential")
}

func TestTokenCmd_HasFlags(t *testing.T) {
	// Verify --cluster-name flag exists.
	clusterFlag := tokenCmd.Flags().Lookup("cluster-name")
	require.NotNil(t, clusterFlag)
	assert.Equal(t, "", clusterFlag.DefValue)

	// Verify --region flag exists.
	regionFlag := tokenCmd.Flags().Lookup("region")
	require.NotNil(t, regionFlag)
	assert.Equal(t, "", regionFlag.DefValue)

	// Verify --identity flag exists.
	identityFlag := tokenCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "", identityFlag.DefValue)
	assert.Equal(t, "i", identityFlag.Shorthand)
}

func TestTokenCmd_NoArgs(t *testing.T) {
	// Command accepts no positional args.
	assert.Nil(t, tokenCmd.Args(tokenCmd, []string{}))
	assert.NotNil(t, tokenCmd.Args(tokenCmd, []string{"extra"}))
}

func TestTokenCmd_ParentIsEksCmd(t *testing.T) {
	assert.NotNil(t, tokenCmd.Parent())
	if tokenCmd.Parent() != nil {
		assert.Equal(t, "eks", tokenCmd.Parent().Name())
	}
}

func TestTokenCmd_SilencesUsage(t *testing.T) {
	// Usage should be silenced since kubectl calls this automatically.
	assert.True(t, tokenCmd.SilenceUsage)
}

func TestTokenCmd_LongDescription(t *testing.T) {
	assert.Contains(t, tokenCmd.Long, "kubectl exec credential plugin")
	assert.Contains(t, tokenCmd.Long, "--cluster-name")
	assert.Contains(t, tokenCmd.Long, "--region")
	assert.Contains(t, tokenCmd.Long, "--identity")
	assert.Contains(t, tokenCmd.Long, "Examples:")
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

func TestResolveIdentity_Flag(t *testing.T) {
	// Create a temporary command to test identity resolution.
	cmd := tokenCmd
	// Reset flag for test.
	err := cmd.Flags().Set("identity", "test-identity")
	require.NoError(t, err)

	result := resolveIdentity(cmd)
	assert.Equal(t, "test-identity", result)

	// Reset.
	t.Cleanup(func() {
		_ = cmd.Flags().Set("identity", "")
	})
}

func TestResolveIdentity_EnvVar(t *testing.T) {
	t.Setenv("ATMOS_IDENTITY", "env-identity")

	// Create a fresh command to avoid flag state from other tests.
	cmd := tokenCmd
	_ = cmd.Flags().Set("identity", "")

	result := resolveIdentity(cmd)
	assert.Equal(t, "env-identity", result)
}

func TestResolveIdentity_Empty(t *testing.T) {
	t.Setenv("ATMOS_IDENTITY", "")

	cmd := tokenCmd
	_ = cmd.Flags().Set("identity", "")

	result := resolveIdentity(cmd)
	assert.Equal(t, "", result)
}
