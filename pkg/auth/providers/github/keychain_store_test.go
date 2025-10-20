package github

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestOSKeychainStore_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping keychain integration test in short mode")
	}

	store := newOSKeychainStore()
	service := "atmos-test-service"
	account := "test-account"
	token := "test-token-value"

	// Clean up any existing test data.
	_ = keyring.Delete(service, account)
	defer keyring.Delete(service, account)

	// Test Set.
	err := store.Set(service, account, token)
	if err != nil {
		// Keychain may not be available in CI environments.
		t.Skipf("Skipping keychain test on %s: keychain not available: %v", runtime.GOOS, err)
	}
	require.NoError(t, err)

	// Test Get.
	retrieved, err := store.Get(service, account)
	require.NoError(t, err)
	assert.Equal(t, token, retrieved)

	// Test Delete.
	err = store.Delete(service, account)
	require.NoError(t, err)

	// Verify deletion.
	_, err = store.Get(service, account)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token not found")
}

func TestOSKeychainStore_GetNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping keychain integration test in short mode")
	}

	store := newOSKeychainStore()
	service := "atmos-nonexistent-service"
	account := "nonexistent-account"

	// Clean up.
	_ = keyring.Delete(service, account)

	_, err := store.Get(service, account)
	if err == nil {
		t.Skip("Keychain not available in test environment")
	}

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token not found")
}

func TestOSKeychainStore_DeleteNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping keychain integration test in short mode")
	}

	store := newOSKeychainStore()
	service := "atmos-nonexistent-service"
	account := "nonexistent-account"

	// Clean up.
	_ = keyring.Delete(service, account)

	err := store.Delete(service, account)
	if err == nil {
		t.Skip("Keychain not available in test environment")
	}

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token not found")
}
