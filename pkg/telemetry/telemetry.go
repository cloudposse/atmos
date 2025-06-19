package telemetry

import (
	"fmt"
	"runtime"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/posthog/posthog-go"
)

type TelemetryClientProvider func(string, posthog.Config) (posthog.Client, error)

func PosthogClientProvider(token string, config posthog.Config) (posthog.Client, error) {
	return posthog.NewWithConfig(token, config)
}

type Telemetry struct {
	isEnabled      bool
	token          string
	endpoint       string
	distinctId     string
	clientProvider func(string, posthog.Config) (posthog.Client, error)
}

func NewTelemetry(isEnabled bool, token string, endpoint string, distinctId string, clientProvider TelemetryClientProvider) *Telemetry {
	return &Telemetry{
		isEnabled:      isEnabled,
		token:          token,
		endpoint:       endpoint,
		distinctId:     distinctId,
		clientProvider: clientProvider,
	}
}

func (t *Telemetry) Capture(eventName string, properties map[string]interface{}) bool {
	if !t.isEnabled || t.token == "" {
		log.Debug("Telemetry is disabled, skipping capture")
		return false
	}

	client, err := t.clientProvider(t.token, posthog.Config{
		Endpoint: t.endpoint,
	})
	if err != nil {
		log.Error(fmt.Sprintf("Could not create PostHog client: %s", err))
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
		log.Error(fmt.Sprintf("Could not enqueue event: %s", err))
		return false
	}
	log.Debug("Telemetry event captured")
	return true
}

func (t *Telemetry) defaultProperties() posthog.Properties {
	return posthog.NewProperties().
		Set("version", version.Version).
		Set("os", runtime.GOOS).
		Set("arch", runtime.GOARCH)
}

func (t *Telemetry) CaptureEvent(eventName string, properties map[string]interface{}) bool {
	propertiesMap := t.defaultProperties().Set("error", false)

	for k, v := range properties {
		propertiesMap.Set(k, v)
	}

	return t.Capture(eventName, propertiesMap)
}

func (t *Telemetry) CaptureError(eventName string, properties map[string]interface{}) bool {
	propertiesMap := t.defaultProperties().Set("error", true)

	for k, v := range properties {
		propertiesMap.Set(k, v)
	}

	return t.Capture(eventName, propertiesMap)
}
