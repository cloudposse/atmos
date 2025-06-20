package telemetry

import (
	log "github.com/charmbracelet/log"
	"github.com/posthog/posthog-go"
)

// TelemetryClientProvider is a function type that creates a PostHog client.
// given a token and configuration. This allows for dependency injection
// and easier testing by providing mock implementations.
type TelemetryClientProvider func(string, posthog.Config) (posthog.Client, error)

// PosthogClientProvider is the default implementation of TelemetryClientProvider
// that creates a real PostHog client using the provided token and configuration.
func PosthogClientProvider(token string, config posthog.Config) (posthog.Client, error) {
	return posthog.NewWithConfig(token, config)
}

// Telemetry represents a telemetry system that can capture events and send them
// to a PostHog analytics service. It provides a configurable way to enable/disable
// telemetry and customize the client provider for testing purposes.
type Telemetry struct {
	isEnabled      bool                    // Whether telemetry is enabled
	token          string                  // PostHog API token for authentication
	endpoint       string                  // PostHog endpoint URL
	distinctId     string                  // Unique identifier for the user/instance
	clientProvider TelemetryClientProvider // Function to create PostHog client
}

// NewTelemetry creates a new Telemetry instance with the specified configuration.
// The clientProvider parameter allows for dependency injection, making it easier
// to test the telemetry system with mock clients.
func NewTelemetry(isEnabled bool, token string, endpoint string, distinctId string, clientProvider TelemetryClientProvider) *Telemetry {
	return &Telemetry{
		isEnabled:      isEnabled,
		token:          token,
		endpoint:       endpoint,
		distinctId:     distinctId,
		clientProvider: clientProvider,
	}
}

// Capture sends a telemetry event to PostHog with the given event name and properties.
// Returns true if the event was successfully captured, false otherwise.
// The method handles various failure scenarios gracefully:
// - Telemetry disabled or missing token
// - Client creation failures
// - Event enqueue failures
func (t *Telemetry) Capture(eventName string, properties map[string]interface{}) bool {
	// Early return if telemetry is disabled or token is missing
	if !t.isEnabled || t.token == "" {
		log.Debug("Telemetry is disabled, skipping capture")
		return false
	}

	// Create PostHog client using the provided client provider
	client, err := t.clientProvider(t.token, posthog.Config{
		Endpoint: t.endpoint,
	})
	if err != nil {
		log.Error("Could not create PostHog client", "error", err)
		return false
	}
	defer client.Close()

	// TODO: PostHog Enqueue always returns nil, but we still check errors
	// to handle them if posthog go lib will return them in the future
	err = client.Enqueue(posthog.Capture{
		DistinctId: t.distinctId,
		Event:      eventName,
		Properties: properties,
	})
	if err != nil {
		log.Error("Could not enqueue event", "error", err)
		return false
	}
	log.Debug("Telemetry event captured")
	return true
}
