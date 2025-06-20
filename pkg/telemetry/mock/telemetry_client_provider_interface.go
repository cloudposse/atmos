package mock_telemetry

import (
	posthog_go "github.com/posthog/posthog-go"
)

type TelemetryClientProviderMock interface {
	NewMockClient(token string, config posthog_go.Config) (posthog_go.Client, error)
}
