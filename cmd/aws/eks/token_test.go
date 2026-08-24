package eks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// initTestIO initializes the IO context for tests that call data.Write().
func initTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to create IO context: %v", err)
	}
	data.InitWriter(ioCtx)
	t.Cleanup(func() { data.Reset() })
}

// newTestTokenCmd creates a fresh cobra.Command with token flags for testing.
// This avoids shared state from the global tokenCmd across tests.
func newTestTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "token",
		RunE: executeTokenCommand,
	}
	cmd.Flags().String("cluster-name", "", "EKS cluster name")
	cmd.Flags().String("region", "", "AWS region")
	cmd.Flags().StringP("identity", "i", "", "Atmos identity")
	return cmd
}

// mockAuthConfig returns a minimal AtmosConfiguration for testing.
func mockAuthConfig() schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"dev-admin": {Kind: "aws/user"},
			},
		},
	}
}

// mockAWSCreds returns test AWS credentials.
func mockAWSCreds() *types.AWSCredentials {
	return &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDH...",
		Region:          "us-east-2",
	}
}

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

// --- executeTokenCommand tests using DI overrides ---

func TestExecuteTokenCommand_MissingClusterName(t *testing.T) {
	origInitConfig := initCliConfigFn
	t.Cleanup(func() { initCliConfigFn = origInitConfig })
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("region", "us-east-2")
	// cluster-name not set.

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
	assert.Contains(t, err.Error(), "--cluster-name is required")
}

func TestExecuteTokenCommand_MissingRegion(t *testing.T) {
	origInitConfig := initCliConfigFn
	t.Cleanup(func() { initCliConfigFn = origInitConfig })
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	// region not set.

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
	assert.Contains(t, err.Error(), "--region is required")
}

func TestExecuteTokenCommand_ConfigInitFailure(t *testing.T) {
	origInitConfig := initCliConfigFn
	t.Cleanup(func() { initCliConfigFn = origInitConfig })
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, fmt.Errorf("config load failed")
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("region", "us-east-2")

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestExecuteTokenCommand_AuthFailure(t *testing.T) {
	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
	})

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return nil, fmt.Errorf("authentication failed")
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("region", "us-east-2")
	_ = cmd.Flags().Set("identity", "dev-admin")

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestExecuteTokenCommand_TokenGenerationFailure(t *testing.T) {
	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	origGetToken := getEKSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
		getEKSTokenFn = origGetToken
	})

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return mockAWSCreds(), nil
	}
	getEKSTokenFn = func(_ context.Context, _ types.ICredentials, _, _ string) (string, time.Time, error) {
		return "", time.Time{}, fmt.Errorf("STS presign failed")
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("region", "us-east-2")
	_ = cmd.Flags().Set("identity", "dev-admin")

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
	assert.Contains(t, err.Error(), "STS presign failed")
}

func TestExecuteTokenCommand_Success(t *testing.T) {
	initTestIO(t)

	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	origGetToken := getEKSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
		getEKSTokenFn = origGetToken
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_SESSION_TOKEN")
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("AWS_DEFAULT_REGION")
	})

	expectedToken := "k8s-aws-v1.test-token-data"
	expectedExpiry := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return mockAWSCreds(), nil
	}
	getEKSTokenFn = func(_ context.Context, _ types.ICredentials, clusterName, region string) (string, time.Time, error) {
		assert.Equal(t, "my-cluster", clusterName)
		assert.Equal(t, "us-east-2", region)
		return expectedToken, expectedExpiry, nil
	}

	// Capture stdout by redirecting data.Write output.
	// The function calls data.Write which writes to stdout.
	// We verify the ExecCredential structure via the mocked token instead.
	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("region", "us-east-2")
	_ = cmd.Flags().Set("identity", "dev-admin")

	// Redirect stdout to capture output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := executeTokenCommand(cmd, []string{})

	// Restore stdout and read output.
	w.Close()
	os.Stdout = oldStdout

	require.NoError(t, err)

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Parse the JSON output.
	var cred execCredential
	require.NoError(t, json.Unmarshal([]byte(output), &cred))
	assert.Equal(t, execCredentialAPIVersion, cred.APIVersion)
	assert.Equal(t, "ExecCredential", cred.Kind)
	assert.Equal(t, expectedToken, cred.Status.Token)
	assert.Equal(t, "2026-03-17T12:00:00Z", cred.Status.ExpirationTimestamp)

	// Verify AWS creds were exported to env.
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", os.Getenv("AWS_ACCESS_KEY_ID"))
}

func TestExecuteTokenCommand_IdentityFromEnvVar(t *testing.T) {
	initTestIO(t)

	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	origGetToken := getEKSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
		getEKSTokenFn = origGetToken
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_SESSION_TOKEN")
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("AWS_DEFAULT_REGION")
	})

	t.Setenv("ATMOS_IDENTITY", "env-identity")

	var capturedIdentity string
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, identityName string) (types.ICredentials, error) {
		capturedIdentity = identityName
		return mockAWSCreds(), nil
	}
	getEKSTokenFn = func(_ context.Context, _ types.ICredentials, _, _ string) (string, time.Time, error) {
		return "token", time.Now().Add(15 * time.Minute), nil
	}

	// Redirect stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("region", "us-east-2")
	// identity flag NOT set — should fall back to ATMOS_IDENTITY env var.

	err := executeTokenCommand(cmd, []string{})
	w.Close()
	os.Stdout = oldStdout

	require.NoError(t, err)
	assert.Equal(t, "env-identity", capturedIdentity)
}

// --- authenticateForToken tests ---

func TestAuthenticateForToken_NoIdentityNoDefault(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{},
	}

	ctx := context.Background()
	_, err := authenticateForToken(ctx, authConfig, "", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
	assert.Contains(t, err.Error(), "no identity specified and no default identity found")
}

func TestAuthenticateForToken_NilAuthConfig(t *testing.T) {
	ctx := context.Background()
	_, err := authenticateForToken(ctx, nil, "", "test-identity")
	require.Error(t, err)
	// NewAuthManager should fail with nil config.
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
}
