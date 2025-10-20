package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeviceFlowClient(t *testing.T) {
	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")

	assert.NotNil(t, client)
	realClient, ok := client.(*realDeviceFlowClient)
	require.True(t, ok)
	assert.Equal(t, "test-client-id", realClient.clientID)
	assert.Equal(t, []string{"repo"}, realClient.scopes)
	assert.Equal(t, "test-service", realClient.keychainSvc)
	assert.NotNil(t, realClient.httpClient)
	assert.NotNil(t, realClient.keychainStore)
}

func TestRealDeviceFlowClient_StartDeviceFlow(t *testing.T) {
	// Create mock HTTP server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		response := DeviceFlowResponse{
			DeviceCode:      "device-code-123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	// Override deviceCodeURL to point to test server.
	originalURL := deviceCodeURL
	deviceCodeURL = server.URL
	defer func() { deviceCodeURL = originalURL }()

	// Start device flow.
	resp, err := realClient.StartDeviceFlow(context.Background())

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "device-code-123", resp.DeviceCode)
	assert.Equal(t, "ABCD-1234", resp.UserCode)
	assert.Equal(t, "https://github.com/login/device", resp.VerificationURI)
	assert.Equal(t, 5, resp.Interval)
}

func TestRealDeviceFlowClient_PollForToken_Success(t *testing.T) {
	callCount := 0

	// Create mock HTTP server that returns authorization_pending first, then success.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount == 1 {
			// First call: pending.
			response := map[string]string{
				"error":             "authorization_pending",
				"error_description": "User hasn't authorized yet",
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		} else {
			// Second call: success.
			response := map[string]string{
				"access_token": "ghs_test_token",
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	// Override accessTokenURL to point to test server.
	originalURL := accessTokenURL
	accessTokenURL = server.URL
	defer func() { accessTokenURL = originalURL }()

	// Override polling interval for faster tests.
	token, err := realClient.PollForToken(context.Background(), "device-code-123")

	require.NoError(t, err)
	assert.Equal(t, "ghs_test_token", token)
	assert.GreaterOrEqual(t, callCount, 2)
}

func TestRealDeviceFlowClient_PollForToken_Expired(t *testing.T) {
	// Create mock HTTP server that returns expired_token.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"error":             "expired_token",
			"error_description": "Device code expired",
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	originalURL := accessTokenURL
	accessTokenURL = server.URL
	defer func() { accessTokenURL = originalURL }()

	token, err := realClient.PollForToken(context.Background(), "device-code-123")

	require.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "device code expired")
}

func TestRealDeviceFlowClient_PollForToken_Denied(t *testing.T) {
	// Create mock HTTP server that returns access_denied.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"error":             "access_denied",
			"error_description": "User denied authorization",
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	originalURL := accessTokenURL
	accessTokenURL = server.URL
	defer func() { accessTokenURL = originalURL }()

	token, err := realClient.PollForToken(context.Background(), "device-code-123")

	require.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "user denied authorization")
}

func TestRealDeviceFlowClient_PollForToken_ContextCancelled(t *testing.T) {
	// Create a context that's already cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")

	token, err := client.PollForToken(ctx, "device-code-123")

	require.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestRealDeviceFlowClient_GetCachedToken(t *testing.T) {
	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	// Use mock keychain store.
	mockStore := &mockKeychainStore{
		tokens: map[string]string{
			"test-service:github-token": "cached-token-123",
		},
	}
	realClient.keychainStore = mockStore

	token, err := realClient.GetCachedToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "cached-token-123", token)
}

func TestRealDeviceFlowClient_GetCachedToken_NotFound(t *testing.T) {
	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	mockStore := &mockKeychainStore{
		tokens: map[string]string{},
	}
	realClient.keychainStore = mockStore

	token, err := realClient.GetCachedToken(context.Background())

	require.NoError(t, err) // GetCachedToken returns empty string, not error.
	assert.Empty(t, token)
}

func TestRealDeviceFlowClient_StoreToken(t *testing.T) {
	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	mockStore := &mockKeychainStore{
		tokens: map[string]string{},
	}
	realClient.keychainStore = mockStore

	err := realClient.StoreToken(context.Background(), "new-token-456")

	require.NoError(t, err)
	assert.Equal(t, "new-token-456", mockStore.tokens["test-service:github-token"])
}

func TestRealDeviceFlowClient_DeleteToken(t *testing.T) {
	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	mockStore := &mockKeychainStore{
		tokens: map[string]string{
			"test-service:github-token": "token-to-delete",
		},
	}
	realClient.keychainStore = mockStore

	err := realClient.DeleteToken(context.Background())

	require.NoError(t, err)
	_, exists := mockStore.tokens["test-service:github-token"]
	assert.False(t, exists)
}

// mockKeychainStore is a simple in-memory keychain for testing.
type mockKeychainStore struct {
	tokens map[string]string
}

func (m *mockKeychainStore) Get(service string, account string) (string, error) {
	key := service + ":" + account
	token, exists := m.tokens[key]
	if !exists {
		return "", assert.AnError
	}
	return token, nil
}

func (m *mockKeychainStore) Set(service string, account string, token string) error {
	key := service + ":" + account
	m.tokens[key] = token
	return nil
}

func (m *mockKeychainStore) Delete(service string, account string) error {
	key := service + ":" + account
	delete(m.tokens, key)
	return nil
}

func TestRealDeviceFlowClient_StartDeviceFlow_DefaultInterval(t *testing.T) {
	// Test that default interval is set when not provided by GitHub.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DeviceFlowResponse{
			DeviceCode:      "device-code-123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			// No interval provided.
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewDeviceFlowClient("test-client-id", []string{"repo"}, "test-service")
	realClient := client.(*realDeviceFlowClient)

	originalURL := deviceCodeURL
	deviceCodeURL = server.URL
	defer func() { deviceCodeURL = originalURL }()

	resp, err := realClient.StartDeviceFlow(context.Background())

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, defaultInterval, resp.Interval, "Should use default interval when not provided")
}
