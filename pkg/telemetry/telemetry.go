package telemetry

import (
	"fmt"
	"runtime"

	log "github.com/charmbracelet/log"
	"github.com/gruntwork-io/go-commons/version"
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

func (t *Telemetry) CaptureEvent(eventName string, properties map[string]interface{}) bool {
	if !t.isEnabled || t.token == "" {
		return false
	}

	client, err := t.clientProvider(t.token, posthog.Config{
		Endpoint: t.endpoint,
	})
	if err != nil {
		log.Warn(fmt.Sprintf("Could not create PostHog client: %s", err))
		return false
	}
	defer client.Close()

	propertiesMap := posthog.NewProperties().
		Set("error", false).
		Set("version", version.GetVersion()).
		Set("os", runtime.GOOS).
		Set("arch", runtime.GOARCH)

	for k, v := range properties {
		propertiesMap.Set(k, v)
	}

	// TODO: PostHog Enqueue always returns nil, but we still check errors
	// to handle them if posthog go lib will return them in the future
	err = client.Enqueue(posthog.Capture{
		DistinctId: t.distinctId,
		Event:      eventName,
		Properties: propertiesMap,
	})
	if err != nil {
		log.Debug(fmt.Sprintf("Could not enqueue event: %s", err))
		return false
	}

	return true
}

func (t *Telemetry) CaptureError(eventName string, properties map[string]interface{}) bool {
	if !t.isEnabled || t.token == "" {
		return false
	}

	client, err := posthog.NewWithConfig(t.token, posthog.Config{
		Endpoint: t.endpoint,
	})
	if err != nil {
		log.Warn(fmt.Sprintf("Could not create PostHog client: %s", err))
		return false
	}
	defer client.Close()

	propertiesMap := posthog.NewProperties().
		Set("error", true).
		Set("version", version.GetVersion()).
		Set("os", runtime.GOOS).
		Set("arch", runtime.GOARCH)

	for k, v := range properties {
		propertiesMap.Set(k, v)
	}

	err = client.Enqueue(posthog.Capture{
		DistinctId: t.distinctId,
		Event:      eventName,
		Properties: propertiesMap,
	})
	if err != nil {
		log.Debug(fmt.Sprintf("Could not enqueue event: %s", err))
		return false
	}

	return true
}
