package telemetry

import (
	"fmt"
	"runtime"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/google/uuid"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/posthog/posthog-go"
	"github.com/spf13/cobra"
)

const (
	DefaultTelemetryToken    = "phc_7s7MrHWxPR2if1DHHDrKBRgx7SvlaoSM59fIiQueexS"
	DefaultTelemetryEndpoint = "https://us.i.posthog.com"
	DefaultEventName         = "command"
)

type Telemetry struct {
	isEnabled  bool
	token      string
	endpoint   string
	distinctId string
}

func NewTelemetry(isEnabled bool, token string, endpoint string, distinctId string) *Telemetry {
	return &Telemetry{
		isEnabled:  isEnabled,
		token:      token,
		endpoint:   endpoint,
		distinctId: distinctId,
	}
}

func (t *Telemetry) CaptureEvent(eventName string, properties map[string]interface{}) {
	if !t.isEnabled {
		return
	}

	client, err := posthog.NewWithConfig(t.token, posthog.Config{
		Endpoint: t.endpoint,
	})
	if err != nil {
		log.Warn(fmt.Sprintf("Could not create PostHog client: %s", err))
		return
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

	err = client.Enqueue(posthog.Capture{
		DistinctId: t.distinctId,
		Event:      eventName,
		Properties: propertiesMap,
	})
	if err != nil {
		log.Debug(fmt.Sprintf("Could not enqueue event: %s", err))
	}
}

func (t *Telemetry) CaptureError(eventName string, properties map[string]interface{}) {
	if !t.isEnabled {
		return
	}

	client, err := posthog.NewWithConfig(t.token, posthog.Config{
		Endpoint: t.endpoint,
	})
	if err != nil {
		log.Warn(fmt.Sprintf("Could not create PostHog client: %s", err))
		return
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
	}
}

// InitializeTelemetry initializes a new Telemetry client.
func InitializeTelemetry() *Telemetry {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return nil
	}

	// Load the cache
	cacheCfg, err := cfg.LoadCache()
	if err != nil {
		log.Warn(fmt.Sprintf("Could not load cache: %s", err))
		return nil
	}

	if cacheCfg.AtmosInstanceId == "" {
		cacheCfg.AtmosInstanceId = uuid.New().String()
	}
	if saveErr := cfg.SaveCache(cacheCfg); saveErr != nil {
		log.Warn(fmt.Sprintf("Unable to save cache: %s", saveErr))
	}
	token := DefaultTelemetryToken

	if atmosConfig.Settings.Telemetry.Token != "" {
		token = atmosConfig.Settings.Telemetry.Token
	}

	endpoint := DefaultTelemetryEndpoint

	if atmosConfig.Settings.Telemetry.Endpoint != "" {
		endpoint = atmosConfig.Settings.Telemetry.Endpoint
	}

	return NewTelemetry(atmosConfig.Settings.Telemetry.Enabled, token, endpoint, cacheCfg.AtmosInstanceId)
}

func CaptureCmdString(cmdString string) {
	if t := InitializeTelemetry(); t != nil {
		t.CaptureEvent(DefaultEventName, map[string]interface{}{"command": cmdString})
	}
}

func CaptureCmdFailureString(cmdString string) {
	if t := InitializeTelemetry(); t != nil {
		t.CaptureError(DefaultEventName, map[string]interface{}{"command": cmdString})
	}
}

func CaptureCmd(cmd *cobra.Command) {
	CaptureCmdString(commandName(cmd))
}

func CaptureCmdFailure(cmd *cobra.Command) {
	CaptureCmdFailureString(commandName(cmd))
}

func commandName(command *cobra.Command) string {
	if command.HasParent() {
		return fmt.Sprintf("%s %s", commandName(command.Parent()), command.Name())
	}
	return command.Name()
}
