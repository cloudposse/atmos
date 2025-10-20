package telemetry

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"testing"

	mock_telemetry "github.com/cloudposse/atmos/pkg/telemetry/mock"
	"github.com/cloudposse/atmos/pkg/version"
	"go.uber.org/mock/gomock"
	"github.com/posthog/posthog-go"
	"github.com/stretchr/testify/assert"
)

// TestPosthogIntegrationToken is a test token for PostHog integration tests.
const (
	TestPosthogIntegrationToken = "phc_7s7MrHWxPR2if1DHHDrKBRgx7SvlaoSM59fIiQueexS"
)

// TestTelemetryConstructor tests the telemetry constructor to ensure it properly initializes
// all fields with the provided parameters.
func TestTelemetryConstructor(t *testing.T) {
	// Generate random test data to avoid conflicts between test runs.
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	// Set up gomock controller for managing mock expectations.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Set up mock expectations - these should not be called during constructor test.
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Create telemetry instance.
	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	// Verify all fields are properly set.
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)
}

// TestTelemetryCaptureMethod tests the Capture method when telemetry is enabled
// and all operations succeed.
func TestTelemetryCaptureMethod(t *testing.T) {
	// Generate random test data.
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	// Set up gomock controller.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Set up mock expectations for successful capture.
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(1)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Create telemetry instance.
	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	// Verify constructor worked correctly.
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	// Test the Capture method.
	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.True(t, captured)
}

// TestTelemetryDisabledCaptureMethod tests the Capture method when telemetry is disabled.
// - should return false without making any client calls.
func TestTelemetryDisabledCaptureMethod(t *testing.T) {
	// Generate random test data.
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := false

	// Set up gomock controller.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Set up mock expectations - these should not be called when disabled.
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Create telemetry instance.
	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	// Verify constructor worked correctly.
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	// Test the Capture method - should return false when disabled.
	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

// TestTelemetryEmptyTokenCaptureMethod tests the Capture method when token is empty.
// - should return false without making any client calls.
func TestTelemetryEmptyTokenCaptureMethod(t *testing.T) {
	// Use empty token.
	token := ""
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	// Set up gomock controller.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Set up mock expectations - these should not be called with empty token.
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Return(mockClient, nil).Times(0)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Create telemetry instance.
	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	// Verify constructor worked correctly.
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	// Test the Capture method - should return false with empty token.
	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

// TestTelemetryProviderErrorCaptureMethod tests the Capture method when the client provider
// returns an error during client creation. This ensures that telemetry gracefully handles
// provider errors and returns false without attempting to capture events.
func TestTelemetryProviderErrorCaptureMethod(t *testing.T) {
	// Generate unique test data to avoid conflicts.
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client provider to be called once and return an error.
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Return(mockClient, errors.New("provider error")).Times(1)

	// These methods should not be called when provider returns an error.
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("error", false).
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(nil).Times(0)
	mockClient.EXPECT().Close().Return(nil).Times(0)

	// Create telemetry instance with mock client provider.
	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	// Verify telemetry instance was created correctly.
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	// Test Capture method - should return false when provider returns error.
	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

// TestTelemetryEnqueueErrorCaptureMethod tests the Capture method when the client's
// Enqueue method returns an error. This ensures that telemetry properly handles
// enqueue errors and returns false, while still attempting to close the client.
func TestTelemetryEnqueueErrorCaptureMethod(t *testing.T) {
	// Generate unique test data to avoid conflicts.
	token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
	endpoint := fmt.Sprintf("https://us.i.posthog.com/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client provider to succeed in creating client.
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Return(mockClient, nil).Times(1)

	// Expect Enqueue to be called once and return an error.
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).Return(errors.New("enqueue error")).Times(1)
	// Expect Close to be called once even when Enqueue fails.
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// Create telemetry instance with mock client provider.
	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	// Verify telemetry instance was created correctly.
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	// Test Capture method - should return false when Enqueue returns error.
	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
	assert.False(t, captured)
}

// TestTelemetryPosthogIntegrationCaptureMethod tests the Capture method with a real
// PostHog client integration. This test verifies that telemetry works correctly
// with actual PostHog API calls using a valid token and endpoint.
func TestTelemetryPosthogIntegrationCaptureMethod(t *testing.T) {
	// Use test PostHog integration token and valid endpoint.
	token := TestPosthogIntegrationToken
	endpoint := "https://us.i.posthog.com/"
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	// Variable to hold the real PostHog client for integration testing.
	var realPosthogClient posthog.Client

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client.
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client provider to create a real PostHog client during the call.
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Do(func(token string, config *posthog.Config) {
		var err error
		realPosthogClient, err = posthog.NewWithConfig(token, *config)
		if err != nil {
			t.Fatalf("Failed to create real PostHog client: %v", err)
		}
	}).Return(mockClient, nil).Times(1)

	// Expect Enqueue to delegate to the real PostHog client.
	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet-1",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).DoAndReturn(func(capture posthog.Capture) error {
		return realPosthogClient.Enqueue(capture)
	}).Times(1)
	// Expect Close to delegate to the real PostHog client.
	mockClient.EXPECT().Close().Do(func() {
		realPosthogClient.Close()
	}).Return(nil).Times(1)

	// Create telemetry instance with mock client provider.
	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	// Verify telemetry instance was created correctly.
	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	// Test Capture method with real PostHog integration.
	captured := telemetry.Capture("test-snippet-1", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)

	// Verify real PostHog client was created and capture was successful.
	assert.NotNil(t, realPosthogClient)
	assert.True(t, captured)
}

// TestTelemetryPosthogIntegrationWrongEndpointCaptureMethod tests the Capture method
// with a real PostHog client but using an incorrect endpoint. This test verifies
// that telemetry handles invalid endpoints gracefully and still returns true
// (since PostHog Enqueue currently always returns nil).
func TestTelemetryPosthogIntegrationWrongEndpointCaptureMethod(t *testing.T) {
	// Use test PostHog integration token but with incorrect endpoint.
	token := TestPosthogIntegrationToken
	endpoint := fmt.Sprintf("https://us.i.posthog.com/wrong/%d", rand.IntN(10000))
	distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))
	enabled := true

	// Variable to hold the real PostHog client for integration testing.
	var realPosthogClient posthog.Client

	// Set up gomock controller for mocking.
	ctrl := gomock.NewController(t)

	// Create mock client provider and client
	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
	mockClient := mock_telemetry.NewMockClient(ctrl)

	// Expect client provider to create a real PostHog client during the call
	mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Do(func(token string, config *posthog.Config) {
		var err error
		realPosthogClient, err = posthog.NewWithConfig(token, *config)
		if err != nil {
			t.Fatalf("Failed to create real PostHog client: %v", err)
		}
	}).Return(mockClient, nil).Times(1)

	mockClient.EXPECT().Enqueue(posthog.Capture{
		DistinctId: distinctId,
		Event:      "test-snippet",
		Properties: posthog.NewProperties().
			Set("plan", "Enterprise").
			Set("friends", 42),
	}).DoAndReturn(func(capture posthog.Capture) error {
		return realPosthogClient.Enqueue(capture)
	}).Times(1)
	mockClient.EXPECT().Close().Do(func() {
		realPosthogClient.Close()
	}).Return(nil).Times(1)

	telemetry := NewTelemetry(enabled, token, Options{Endpoint: endpoint, DistinctID: distinctId, Logging: false}, mockClientProvider.NewMockClient)

	assert.Equal(t, telemetry.isEnabled, enabled)
	assert.Equal(t, telemetry.token, token)
	assert.Equal(t, telemetry.endpoint, endpoint)
	assert.Equal(t, telemetry.distinctId, distinctId)
	assert.NotNil(t, telemetry.clientProvider)

	captured := telemetry.Capture("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)

	assert.NotNil(t, realPosthogClient)
	// TODO: PostHog Enqueue always returns nil, but we still check errors
	// to handle them if posthog go lib will return them in the future.
	assert.True(t, captured)
}

// TestTelemetryLogging tests telemetry with different logging configurations
// using a table-driven approach to avoid duplication.
func TestTelemetryLogging(t *testing.T) {
	tests := []struct {
		name           string
		logging        bool
		expectedLogger string // "PosthogLogger" or "SilentLogger"
	}{
		{
			name:           "LoggingEnabled",
			logging:        true,
			expectedLogger: "PosthogLogger",
		},
		{
			name:           "LoggingDisabled",
			logging:        false,
			expectedLogger: "SilentLogger",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock client provider and client.
			mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)
			mockClient := mock_telemetry.NewMockClient(ctrl)

			// Define test input values.
			enabled := true
			token := fmt.Sprintf("phc_test_token_%d", rand.IntN(10000))
			endpoint := "https://test.posthog.com"
			distinctId := fmt.Sprintf("test-user-%d", rand.IntN(10000))

			// Set up expectations for successful flow.
			mockClientProvider.EXPECT().NewMockClient(token, gomock.Any()).Return(mockClient, nil).Times(1)
			mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(1)
			mockClient.EXPECT().Close().Return(nil).Times(1)

			// Create telemetry instance with the test case's logging configuration.
			telemetry := NewTelemetry(enabled, token, Options{
				Endpoint:   endpoint,
				DistinctID: distinctId,
				Logging:    tc.logging,
			}, mockClientProvider.NewMockClient)

			// Verify telemetry instance was created correctly.
			assert.Equal(t, enabled, telemetry.isEnabled)
			assert.Equal(t, token, telemetry.token)
			assert.Equal(t, endpoint, telemetry.endpoint)
			assert.Equal(t, distinctId, telemetry.distinctId)
			assert.Equal(t, tc.logging, telemetry.logging)

			// Act: Capture an event.
			success := telemetry.Capture("test_event", map[string]interface{}{"key": "value"})

			// Assert: Verify event was captured successfully.
			assert.True(t, success, "Expected capture to succeed for %s", tc.name)
		})
	}
}
