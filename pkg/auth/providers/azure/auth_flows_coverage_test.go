package azure

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubAz puts a fake `az` executable on PATH that prints the given JSON.
// The output is written to a data file so no shell quoting is needed, making
// the same approach work for both the POSIX script and the Windows batch file
// (exec.LookPath resolves az.bat via PATHEXT).
func stubAz(t *testing.T, stdout string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	outFile := filepath.Join(dir, "az_output.txt")
	require.NoError(t, os.WriteFile(outFile, []byte(stdout+"\n"), 0o644))

	if runtime.GOOS == "windows" {
		script := fmt.Sprintf("@echo off\r\ntype \"%s\"\r\nexit /b %d\r\n", outFile, exitCode)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "az.bat"), []byte(script), 0o755))
	} else {
		script := fmt.Sprintf("#!/bin/sh\ncat \"%s\"\nexit %d\n", outFile, exitCode)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "az"), []byte(script), 0o755))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newTestCLIProvider(t *testing.T, subscriptionID string) *cliProvider {
	t.Helper()
	spec := map[string]interface{}{"tenant_id": "tenant-123"}
	if subscriptionID != "" {
		spec["subscription_id"] = subscriptionID
	}
	p, err := NewCLIProvider("azure-cli", &schema.Provider{Kind: "azure/cli", Spec: spec})
	require.NoError(t, err)
	return p
}

func TestCLIProvider_Authenticate_BuildsCredentialsFromAzOutput(t *testing.T) {
	expires := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	stubAz(t, fmt.Sprintf(`{"accessToken":"az-token","expiresOn":"%s","tenant":"tenant-123","subscription":"sub-from-az","tokenType":"Bearer"}`, expires), 0)

	p := newTestCLIProvider(t, "")

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)

	azureCreds, ok := creds.(*authTypes.AzureCredentials)
	require.True(t, ok)
	assert.Equal(t, "az-token", azureCreds.AccessToken)
	assert.Equal(t, "tenant-123", azureCreds.TenantID)
	assert.Equal(t, "sub-from-az", azureCreds.SubscriptionID, "subscription falls back to the az response when not configured")
	assert.Equal(t, authTypes.AzureAuthMethodCLI, azureCreds.AuthMethod,
		"CLI-minted credentials must be marked so the az cache write-back is skipped")
}

func TestCLIProvider_Authenticate_NotLoggedIn(t *testing.T) {
	stubAz(t, `Please run 'az login' to setup account.`, 1)

	p := newTestCLIProvider(t, "sub-123")

	_, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "az login")
}

func testJWT(t *testing.T, payload string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"RS256"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func newTestDeviceCodeProvider(t *testing.T) *deviceCodeProvider {
	t.Helper()
	p, err := NewDeviceCodeProvider("azure-device-code", &schema.Provider{
		Kind: "azure/device-code",
		Spec: map[string]interface{}{"tenant_id": "tenant-123"},
	})
	require.NoError(t, err)
	return p
}

func TestDeviceCodeProvider_CreateCredentials_AllTokens(t *testing.T) {
	p := newTestDeviceCodeProvider(t)
	now := time.Now().UTC()

	creds, err := p.createCredentials(&tokenAcquisitionResult{
		accessToken:       "mgmt-token",
		expiresOn:         now.Add(1 * time.Hour),
		graphToken:        "graph-token",
		graphExpiresOn:    now.Add(2 * time.Hour),
		keyVaultToken:     "kv-token",
		keyVaultExpiresOn: now.Add(3 * time.Hour),
		homeAccountID:     "home-oid.home-tenant",
	})
	require.NoError(t, err)

	azureCreds, ok := creds.(*authTypes.AzureCredentials)
	require.True(t, ok)
	assert.Equal(t, "mgmt-token", azureCreds.AccessToken)
	assert.Equal(t, "graph-token", azureCreds.GraphAPIToken)
	assert.Equal(t, "kv-token", azureCreds.KeyVaultToken)
	assert.Equal(t, "home-oid.home-tenant", azureCreds.HomeAccountID,
		"MSAL home account ID must survive into credentials for correct az cache interop")
	assert.Equal(t, authTypes.AzureAuthMethodDeviceCode, azureCreds.AuthMethod)
}

func TestDeviceCodeProvider_CreateCredentials_ManagementOnly(t *testing.T) {
	p := newTestDeviceCodeProvider(t)

	creds, err := p.createCredentials(&tokenAcquisitionResult{
		accessToken: "mgmt-token",
		expiresOn:   time.Now().UTC().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	azureCreds, ok := creds.(*authTypes.AzureCredentials)
	require.True(t, ok)
	assert.Empty(t, azureCreds.GraphAPIToken)
	assert.Empty(t, azureCreds.KeyVaultToken)
	assert.Empty(t, azureCreds.HomeAccountID)
}

func newOfflineMSALClient(t *testing.T) public.Client {
	t.Helper()
	client, err := public.New(
		defaultAzureClientID,
		public.WithAuthority("https://login.microsoftonline.com/tenant-123"),
		public.WithInstanceDiscovery(false),
	)
	require.NoError(t, err)
	return client
}

func TestDeviceCodeProvider_TrySilentTokenAcquisition(t *testing.T) {
	p := newTestDeviceCodeProvider(t)
	client := newOfflineMSALClient(t)

	tests := []struct {
		name     string
		accounts []public.Account
	}{
		{
			name:     "no cached accounts",
			accounts: nil,
		},
		{
			name: "no account for tenant",
			accounts: []public.Account{{
				HomeAccountID:     "oid.other-tenant",
				Realm:             "other-tenant",
				PreferredUsername: "user@example.com",
			}},
		},
		{
			// The account matches the tenant but is not in the MSAL cache, so
			// the silent call fails locally and the flow falls through to
			// device code.
			name: "matching account but silent acquisition fails",
			accounts: []public.Account{{
				HomeAccountID:     "home-oid.home-tenant",
				Realm:             "tenant-123",
				PreferredUsername: "user@example.com",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.trySilentTokenAcquisition(context.Background(), &client, tt.accounts)
			assert.Empty(t, result.accessToken)
			assert.Empty(t, result.homeAccountID, "home account id must not be set when silent acquisition does not succeed")
		})
	}
}

func TestDeviceCodeProvider_AcquireAdditionalTokens_SilentFailures(t *testing.T) {
	p := newTestDeviceCodeProvider(t)
	client := newOfflineMSALClient(t)

	accounts := []public.Account{{
		HomeAccountID:     "home-oid.home-tenant",
		Realm:             "tenant-123",
		PreferredUsername: "user@example.com",
	}}
	result := tokenAcquisitionResult{accessToken: "mgmt-token"}

	// Graph and KeyVault silent acquisitions fail locally (account not in
	// cache); the result must keep the management token and stay usable.
	p.acquireAdditionalTokens(context.Background(), &client, accounts, &result)
	assert.Equal(t, "mgmt-token", result.accessToken)
	assert.Empty(t, result.graphToken)
	assert.Empty(t, result.keyVaultToken)
}

func TestDeviceCodeProvider_Authenticate_Headless(t *testing.T) {
	// Sandbox HOME so the MSAL cache is created under a temp dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	p := newTestDeviceCodeProvider(t)

	// Under `go test` stderr is not a TTY, so after the (empty-cache) silent
	// attempt the device code flow refuses to start.
	_, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "interactive terminal")
}

func TestDeviceCodeProvider_FindAccountForTenant_CapturesHomeAccountID(t *testing.T) {
	p := newTestDeviceCodeProvider(t)

	accounts := []public.Account{
		{HomeAccountID: "oid.other", Realm: "other-tenant"},
		{HomeAccountID: "home-oid.home-tenant", Realm: "tenant-123"},
	}
	account, err := p.findAccountForTenant(accounts)
	require.NoError(t, err)
	assert.Equal(t, "home-oid.home-tenant", account.HomeAccountID)
}

func TestDeviceCodeProvider_CaptureHomeAccountID(t *testing.T) {
	p := newTestDeviceCodeProvider(t)

	t.Run("matching account captured", func(t *testing.T) {
		result := tokenAcquisitionResult{}
		p.captureHomeAccountID([]public.Account{
			{HomeAccountID: "home-oid.home-tenant", Realm: "tenant-123"},
		}, &result)
		assert.Equal(t, "home-oid.home-tenant", result.homeAccountID)
	})

	t.Run("no matching account leaves result empty", func(t *testing.T) {
		result := tokenAcquisitionResult{}
		p.captureHomeAccountID([]public.Account{
			{HomeAccountID: "oid.other", Realm: "other-tenant"},
		}, &result)
		assert.Empty(t, result.homeAccountID)
	})
}
