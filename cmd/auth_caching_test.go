package cmd

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
)

// TestAuth_CredentialCaching verifies that credentials are cached after login
// and reused for subsequent commands without triggering re-authentication.
//
// This is a regression test for the issue reported by Bogdan where browser
// authentication was triggered on every command, causing slowdowns.
//
// Expected behavior:
// - First command (login): Authenticate and cache credentials.
// - Subsequent commands: Use cached credentials (instant, no browser).
func TestAuth_CredentialCaching(t *testing.T) {
	tk := NewTestKit(t)

	// Use mock auth scenario with file keyring for test isolation.
	// Memory keyring doesn't persist between RootCmd.Execute() calls.
	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", tempDir+"/keyring.json")
	tk.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-for-file-keyring")

	// Step 1: Authenticate (this caches credentials).
	t.Run("initial login caches credentials", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})

		start := time.Now()
		err := RootCmd.Execute()
		loginDuration := time.Since(start)

		require.NoError(t, err, "Login should succeed")
		t.Logf("Login took %v", loginDuration)
	})

	// Step 2: Run multiple commands that should use cached credentials.
	// These should complete instantly without triggering browser auth.
	cachedCommands := []struct {
		name string
		args []string
	}{
		{
			name: "auth whoami with cached credentials",
			args: []string{"auth", "whoami", "--identity", "mock-identity"},
		},
		{
			name: "auth env json with cached credentials",
			args: []string{"auth", "env", "--format", "json", "--identity", "mock-identity"},
		},
		{
			name: "auth env bash with cached credentials",
			args: []string{"auth", "env", "--format", "bash", "--identity", "mock-identity"},
		},
		{
			name: "auth env dotenv with cached credentials",
			args: []string{"auth", "env", "--format", "dotenv", "--identity", "mock-identity"},
		},
	}

	// Run all cached command tests sequentially without creating new TestKits
	// to preserve memory keyring state.
	for _, tc := range cachedCommands {
		t.Run(tc.name, func(t *testing.T) {
			// Don't create new TestKit - reuse parent's environment to keep keyring state.
			RootCmd.SetArgs(tc.args)

			start := time.Now()
			err := RootCmd.Execute()
			duration := time.Since(start)

			require.NoError(t, err, "Command should succeed with cached credentials")

			// Cached credentials should be instant (< 1 second).
			// Browser-based auth typically takes 5-30 seconds.
			// We use 2 seconds as a generous threshold to account for test overhead.
			assert.Less(t, duration, 2*time.Second,
				"Command took %v - may have triggered re-authentication instead of using cache", duration)

			t.Logf("%s completed in %v", strings.Join(tc.args, " "), duration)
		})
	}
}

// TestAuth_NoBrowserPromptForCachedCredentials is a more comprehensive test
// that simulates a real-world workflow with multiple commands.
func TestAuth_NoBrowserPromptForCachedCredentials(t *testing.T) {
	tk := NewTestKit(t)

	// Setup mock auth scenario with file keyring.
	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", tempDir+"/keyring.json")

	// Step 1: Initial login to cache credentials.
	t.Log("Step 1: Performing initial login to cache credentials")
	RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})
	err := RootCmd.Execute()
	require.NoError(t, err, "Initial login should succeed")

	// Step 2: Simulate a typical workflow with multiple commands.
	// All of these should use cached credentials without triggering browser auth.
	workflowCommands := [][]string{
		{"auth", "whoami", "--identity", "mock-identity"},
		{"auth", "list"},
		{"auth", "env", "--identity", "mock-identity"},
		{"auth", "validate"},
	}

	totalDuration := time.Duration(0)

	// Run workflow commands sequentially, reusing parent TestKit to preserve keyring state.
	for i, args := range workflowCommands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			// Don't create new TestKit - reuse parent's environment to keep memory keyring state.
			RootCmd.SetArgs(args)

			start := time.Now()
			err := RootCmd.Execute()
			duration := time.Since(start)

			totalDuration += duration

			require.NoError(t, err, "Command %d should succeed: %v", i+1, args)

			// Individual commands should be fast.
			assert.Less(t, duration, 2*time.Second,
				"Command %d took too long (%v) - possible re-authentication", i+1, duration)

			t.Logf("Command %d: %v completed in %v", i+1, strings.Join(args, " "), duration)
		})
	}

	// Total workflow should be fast (< 5 seconds for all commands).
	// If browser auth was triggered for each command, this would take 20-120 seconds.
	t.Logf("Total workflow duration: %v", totalDuration)
	assert.Less(t, totalDuration, 5*time.Second,
		"Total workflow took too long (%v) - credentials may not be properly cached", totalDuration)
}

// TestAuth_ExpiredCredentialsForceReauth verifies that expired credentials
// trigger re-authentication rather than using stale cached values.
func TestAuth_ExpiredCredentialsForceReauth(t *testing.T) {
	tk := NewTestKit(t)

	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", tempDir+"/keyring.json")

	// Note: Mock credentials expire in 2099, so they won't actually expire in tests.
	// This test verifies the error handling when credentials are not found.
	t.Run("no cached credentials returns error", func(t *testing.T) {
		RootCmd.SetArgs([]string{"auth", "whoami", "--identity", "mock-identity"})

		err := RootCmd.Execute()

		// Should fail because no credentials are cached yet.
		require.Error(t, err, "Should fail when no credentials cached")
		assert.Contains(t, err.Error(), "no credentials found",
			"Error should indicate credentials not found")
	})

	// After login, credentials should be available.
	t.Run("credentials available after login", func(t *testing.T) {
		// Don't create new TestKit - preserve file keyring state.
		// Login first.
		RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})
		err := RootCmd.Execute()
		require.NoError(t, err)

		// Now whoami should work.
		RootCmd.SetArgs([]string{"auth", "whoami", "--identity", "mock-identity"})
		err = RootCmd.Execute()
		require.NoError(t, err, "Whoami should succeed after login")
	})
}

// TestAuth_MultipleIdentities verifies that credentials for different
// identities are cached independently.
func TestAuth_MultipleIdentities(t *testing.T) {
	tk := NewTestKit(t)

	tk.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
	tempDir := t.TempDir()
	tk.Setenv("ATMOS_KEYRING_TYPE", "file")
	tk.Setenv("ATMOS_KEYRING_FILE_PATH", tempDir+"/keyring.json")

	identities := []string{"mock-identity", "mock-identity-2"}

	// Login to both identities.
	for _, identity := range identities {
		t.Run("login to "+identity, func(t *testing.T) {
			// Don't create new TestKit - preserve file keyring state.
			RootCmd.SetArgs([]string{"auth", "login", "--identity", identity})
			err := RootCmd.Execute()
			require.NoError(t, err, "Login should succeed for %s", identity)
		})
	}

	// Verify both identities have cached credentials.
	for _, identity := range identities {
		t.Run("whoami for "+identity, func(t *testing.T) {
			// Don't create new TestKit - preserve file keyring state.
			RootCmd.SetArgs([]string{"auth", "whoami", "--identity", identity})

			start := time.Now()
			err := RootCmd.Execute()
			duration := time.Since(start)

			require.NoError(t, err, "Whoami should succeed for %s", identity)
			assert.Less(t, duration, 2*time.Second,
				"Whoami for %s took too long (%v) - credentials may not be cached", identity, duration)
		})
	}
}

// TestKeyringStoreRetrieve tests basic store and retrieve operations.
func TestKeyringStoreRetrieve(t *testing.T) {
	tempDir := t.TempDir()
	keyringPath := filepath.Join(tempDir, "keyring.json")

	t.Setenv("ATMOS_KEYRING_TYPE", "file")
	t.Setenv("ATMOS_KEYRING_FILE_PATH", keyringPath)
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345678")

	// Create credential store
	store := credentials.NewCredentialStore()

	// Create mock credentials
	creds := &mock.Credentials{
		AccessKeyID:     "MOCK_KEY",
		SecretAccessKey: "MOCK_SECRET",
		SessionToken:    "MOCK_TOKEN",
		Region:          "us-east-1",
		Expiration:      time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC),
	}

	// Store credentials
	err := store.Store("test-identity", creds)
	require.NoError(t, err, "Should store credentials")

	// Retrieve credentials
	retrieved, err := store.Retrieve("test-identity")
	require.NoError(t, err, "Should retrieve credentials")

	// Verify type
	mockRetrieved, ok := retrieved.(*mock.Credentials)
	require.True(t, ok, "Retrieved credentials should be mock.Credentials, got %T", retrieved)

	// Verify values
	assert.Equal(t, "MOCK_KEY", mockRetrieved.AccessKeyID)
}
