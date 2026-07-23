package aks

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
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
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
	cmd.Flags().String("cluster-name", "", "AKS cluster name")
	cmd.Flags().String("resource-group", "", "Azure resource group")
	cmd.Flags().String("subscription-id", "", "Azure subscription ID")
	cmd.Flags().String("server-id", "", "AKS AAD server application ID")
	cmd.Flags().StringP("identity", "i", "", "Atmos identity")
	return cmd
}

// mockAuthConfig returns a minimal AtmosConfiguration for testing.
func mockAuthConfig() schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"azure-dev": {Kind: "azure/subscription"},
			},
		},
	}
}

// mockAzureCreds returns test Azure credentials.
func mockAzureCreds() *types.AzureCredentials {
	return &types.AzureCredentials{
		AccessToken:    "aad-access-token",
		TenantID:       "tenant-123",
		SubscriptionID: "sub-456",
	}
}

func TestTokenCmd_Help(t *testing.T) {
	assert.Equal(t, "token", tokenCmd.Use)
	assert.Equal(t, "Generate an AKS bearer token for kubectl", tokenCmd.Short)
	assert.Contains(t, tokenCmd.Long, "ExecCredential")
}

func TestTokenCmd_HasFlags(t *testing.T) {
	clusterFlag := tokenCmd.Flags().Lookup("cluster-name")
	require.NotNil(t, clusterFlag)
	assert.Equal(t, "", clusterFlag.DefValue)

	rgFlag := tokenCmd.Flags().Lookup("resource-group")
	require.NotNil(t, rgFlag)
	assert.Equal(t, "", rgFlag.DefValue)

	subFlag := tokenCmd.Flags().Lookup("subscription-id")
	require.NotNil(t, subFlag)

	identityFlag := tokenCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "", identityFlag.DefValue)
	assert.Equal(t, "i", identityFlag.Shorthand)

	assert.NotNil(t, tokenCmd.Flags().Lookup("server-id"))
}

func TestTokenCmd_NoArgs(t *testing.T) {
	assert.Nil(t, tokenCmd.Args(tokenCmd, []string{}))
	assert.NotNil(t, tokenCmd.Args(tokenCmd, []string{"extra"}))
}

func TestTokenCmd_ParentIsAksCmd(t *testing.T) {
	assert.NotNil(t, tokenCmd.Parent())
	if tokenCmd.Parent() != nil {
		assert.Equal(t, "aks", tokenCmd.Parent().Name())
	}
}

func TestTokenCmd_SilencesUsage(t *testing.T) {
	assert.True(t, tokenCmd.SilenceUsage)
}

func TestTokenCmd_LongDescription(t *testing.T) {
	assert.Contains(t, tokenCmd.Long, "kubectl exec credential plugin")
	assert.Contains(t, tokenCmd.Long, "--cluster-name")
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
			"azure-dev": {Kind: "azure/subscription"},
		},
	}
	result := resolveDefaultIdentity(authConfig)
	assert.Equal(t, "azure-dev", result)
}

func TestResolveDefaultIdentity_MultipleIdentities(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"azure-dev":  {Kind: "azure/subscription"},
			"azure-prod": {Kind: "azure/subscription"},
		},
	}
	result := resolveDefaultIdentity(authConfig)
	assert.Equal(t, "", result)
}

func TestExecCredentialAPIVersion(t *testing.T) {
	assert.Equal(t, "client.authentication.k8s.io/v1beta1", execCredentialAPIVersion)
}

func TestAKSTokenErrors(t *testing.T) {
	assert.NotNil(t, errUtils.ErrAKSTokenGeneration)
}

func TestExecCredentialStruct(t *testing.T) {
	cred := execCredential{
		APIVersion: execCredentialAPIVersion,
		Kind:       "ExecCredential",
		Status: execCredentialStatus{
			ExpirationTimestamp: "2026-03-16T12:00:00Z",
			Token:               "aad-jwt-test-token",
		},
	}

	assert.Equal(t, "client.authentication.k8s.io/v1beta1", cred.APIVersion)
	assert.Equal(t, "ExecCredential", cred.Kind)
	assert.Equal(t, "aad-jwt-test-token", cred.Status.Token)
}

func TestResolveIdentity_Flag(t *testing.T) {
	cmd := tokenCmd
	err := cmd.Flags().Set("identity", "test-identity")
	require.NoError(t, err)

	result := resolveIdentity(cmd)
	assert.Equal(t, "test-identity", result)

	t.Cleanup(func() {
		_ = cmd.Flags().Set("identity", "")
	})
}

func TestResolveIdentity_EnvVar(t *testing.T) {
	t.Setenv("ATMOS_IDENTITY", "env-identity")

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
	_ = cmd.Flags().Set("resource-group", "my-rg")
	// cluster-name not set.

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
	assert.Contains(t, err.Error(), "--cluster-name is required")
}

func TestExecuteTokenCommand_MissingResourceGroup(t *testing.T) {
	origInitConfig := initCliConfigFn
	t.Cleanup(func() { initCliConfigFn = origInitConfig })
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	// resource-group not set.

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
	assert.Contains(t, err.Error(), "--resource-group is required")
}

func TestExecuteTokenCommand_ConfigInitFailure(t *testing.T) {
	origInitConfig := initCliConfigFn
	t.Cleanup(func() { initCliConfigFn = origInitConfig })
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, fmt.Errorf("config load failed")
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("resource-group", "my-rg")

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
	_ = cmd.Flags().Set("resource-group", "my-rg")
	_ = cmd.Flags().Set("identity", "azure-dev")

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestExecuteTokenCommand_TokenGenerationFailure(t *testing.T) {
	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	origGetToken := getAKSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
		getAKSTokenFn = origGetToken
	})

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(ctx context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		assert.Equal(t, "custom-server-id/.default", azureCloud.AKSServerScopeFromContext(ctx))
		return mockAzureCreds(), nil
	}
	getAKSTokenFn = func(_ types.ICredentials) (string, time.Time, error) {
		return "", time.Time{}, fmt.Errorf("no AKS-scoped token available")
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("resource-group", "my-rg")
	_ = cmd.Flags().Set("identity", "azure-dev")
	_ = cmd.Flags().Set("server-id", "custom-server-id")

	err := executeTokenCommand(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
	assert.Contains(t, err.Error(), "no AKS-scoped token available")
}

func TestExecuteTokenCommand_Success(t *testing.T) {
	initTestIO(t)

	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	origGetToken := getAKSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
		getAKSTokenFn = origGetToken
	})

	expectedToken := "aad-jwt-test-token"
	expectedExpiry := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return mockAzureCreds(), nil
	}
	getAKSTokenFn = func(_ types.ICredentials) (string, time.Time, error) {
		return expectedToken, expectedExpiry, nil
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("resource-group", "my-rg")
	_ = cmd.Flags().Set("identity", "azure-dev")

	// Redirect stdout to capture output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := executeTokenCommand(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	require.NoError(t, err)

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var cred execCredential
	require.NoError(t, json.Unmarshal([]byte(output), &cred))
	assert.Equal(t, execCredentialAPIVersion, cred.APIVersion)
	assert.Equal(t, "ExecCredential", cred.Kind)
	assert.Equal(t, expectedToken, cred.Status.Token)
	assert.Equal(t, "2026-03-17T12:00:00Z", cred.Status.ExpirationTimestamp)
}

func TestExecuteTokenCommand_Success_NoExpiration(t *testing.T) {
	initTestIO(t)

	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	origGetToken := getAKSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
		getAKSTokenFn = origGetToken
	})

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, _ string) (types.ICredentials, error) {
		return mockAzureCreds(), nil
	}
	getAKSTokenFn = func(_ types.ICredentials) (string, time.Time, error) {
		return "aad-jwt-test-token", time.Time{}, nil
	}

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("resource-group", "my-rg")
	_ = cmd.Flags().Set("identity", "azure-dev")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := executeTokenCommand(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	require.NoError(t, err)

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var cred execCredential
	require.NoError(t, json.Unmarshal([]byte(output), &cred))
	assert.Equal(t, "", cred.Status.ExpirationTimestamp)
}

func TestExecuteTokenCommand_IdentityFromEnvVar(t *testing.T) {
	initTestIO(t)

	origInitConfig := initCliConfigFn
	origAuth := authenticateForTokenFn
	origGetToken := getAKSTokenFn
	t.Cleanup(func() {
		initCliConfigFn = origInitConfig
		authenticateForTokenFn = origAuth
		getAKSTokenFn = origGetToken
	})

	t.Setenv("ATMOS_IDENTITY", "env-identity")

	var capturedIdentity string
	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return mockAuthConfig(), nil
	}
	authenticateForTokenFn = func(_ context.Context, _ *schema.AuthConfig, _, identityName string) (types.ICredentials, error) {
		capturedIdentity = identityName
		return mockAzureCreds(), nil
	}
	getAKSTokenFn = func(_ types.ICredentials) (string, time.Time, error) {
		return "token", time.Now().Add(15 * time.Minute), nil
	}

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newTestTokenCmd()
	_ = cmd.Flags().Set("cluster-name", "my-cluster")
	_ = cmd.Flags().Set("resource-group", "my-rg")
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
	assert.ErrorIs(t, err, errUtils.ErrAKSTokenGeneration)
	assert.Contains(t, err.Error(), "no identity specified and no default identity found")
}

func TestAuthenticateForToken_NilAuthConfig(t *testing.T) {
	ctx := context.Background()
	_, err := authenticateForToken(ctx, nil, "", "test-identity")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
}
