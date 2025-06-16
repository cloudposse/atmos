package telemetry

import (
	"testing"

	"github.com/posthog/posthog-go"
)

func TestTelemetry(t *testing.T) {
	telemetry := NewTelemetry(true, DefaultTelemetryToken, "test-user-2")

	telemetry.CaptureEvent("test-snippet", posthog.NewProperties().
		Set("plan", "Enterprise").
		Set("friends", 42),
	)
}
