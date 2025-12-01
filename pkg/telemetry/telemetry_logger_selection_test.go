package telemetry

import (
	"testing"

	mock_telemetry "github.com/cloudposse/atmos/pkg/telemetry/mock"
	"github.com/posthog/posthog-go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestTelemetryLoggerSelectionBasedOnFlag tests that the correct logger is selected
// based on the logging flag during client creation.
func TestTelemetryLoggerSelectionBasedOnFlag(t *testing.T) {
	tests := []struct {
		name           string
		logging        bool
		expectedLogger string
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create a custom mock provider to inspect the config
			var capturedConfig *posthog.Config
			mockProvider := func(token string, config *posthog.Config) (posthog.Client, error) {
				capturedConfig = config
				mockClient := mock_telemetry.NewMockClient(ctrl)
				mockClient.EXPECT().Enqueue(gomock.Any()).Return(nil).Times(1)
				mockClient.EXPECT().Close().Return(nil).Times(1)
				return mockClient, nil
			}

			// Create telemetry with specified logging setting
			telemetry := NewTelemetry(true, "test-token", Options{
				Endpoint:   "https://test.com",
				DistinctID: "test-id",
				Logging:    tt.logging,
			}, mockProvider)
			success := telemetry.Capture("test", map[string]interface{}{})

			assert.True(t, success)
			assert.NotNil(t, capturedConfig)
			assert.NotNil(t, capturedConfig.Logger)

			// Verify the correct logger type is used
			switch tt.expectedLogger {
			case "PosthogLogger":
				_, isPosthogLogger := capturedConfig.Logger.(*PosthogLogger)
				assert.True(t, isPosthogLogger, "Should use PosthogLogger when logging is enabled")
			case "SilentLogger":
				_, isSilentLogger := capturedConfig.Logger.(*SilentLogger)
				assert.True(t, isSilentLogger, "Should use SilentLogger when logging is disabled")
			}
		})
	}
}

// TestTelemetryConstructorWithLogging tests that the telemetry constructor correctly
// handles the logging parameter.
func TestTelemetryConstructorWithLogging(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClientProvider := mock_telemetry.NewMockTelemetryClientProviderMock(ctrl)

	// Test with logging enabled
	t.Run("LoggingEnabled", func(t *testing.T) {
		telemetry := NewTelemetry(true, "token", Options{Endpoint: "endpoint", DistinctID: "id", Logging: true}, mockClientProvider.NewMockClient)
		assert.True(t, telemetry.logging)
		assert.True(t, telemetry.isEnabled)
	})

	// Test with logging disabled
	t.Run("LoggingDisabled", func(t *testing.T) {
		telemetry := NewTelemetry(true, "token", Options{Endpoint: "endpoint", DistinctID: "id", Logging: false}, mockClientProvider.NewMockClient)
		assert.False(t, telemetry.logging)
		assert.True(t, telemetry.isEnabled)
	})

	// Test that logging flag is independent of enabled flag
	t.Run("TelemetryDisabledLoggingEnabled", func(t *testing.T) {
		telemetry := NewTelemetry(false, "token", Options{Endpoint: "endpoint", DistinctID: "id", Logging: true}, mockClientProvider.NewMockClient)
		assert.True(t, telemetry.logging)
		assert.False(t, telemetry.isEnabled)
	})
}
