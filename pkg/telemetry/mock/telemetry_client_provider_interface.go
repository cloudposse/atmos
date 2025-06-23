// Package mock_telemetry provides mock implementations for telemetry client testing
package mock_telemetry

import (
	// posthog_go provides the PostHog client library for analytics
	posthog_go "github.com/posthog/posthog-go"
)

// TelemetryClientProviderMock defines the interface for creating mock telemetry clients
// This interface is used for testing purposes to provide controlled telemetry behavior
type TelemetryClientProviderMock interface {
	// NewMockClient creates a new mock PostHog client instance
	// Parameters:
	//   - token: The authentication token for the PostHog client
	//   - config: Configuration settings for the PostHog client
	// Returns:
	//   - posthog_go.Client: The mock client instance
	//   - error: Any error that occurred during client creation
	NewMockClient(token string, config posthog_go.Config) (posthog_go.Client, error)
}
