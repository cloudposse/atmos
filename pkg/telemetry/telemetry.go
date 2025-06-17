package telemetry

import (
	"fmt"
	"runtime"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/google/uuid"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/posthog/posthog-go"
	"github.com/spf13/cobra"
)

const (
	DefaultTelemetryToken = "phc_7s7MrHWxPR2if1DHHDrKBRgx7SvlaoSM59fIiQueexS"
	DefaultEventName      = "command"
)

type Telemetry struct {
	isEnabled  bool
	token      string
	endpoint   string
	distinctId string
}

func NewTelemetry(isEnabled bool, token string, distinctId string) *Telemetry {
	return &Telemetry{
		isEnabled:  isEnabled,
		token:      token,
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
		u.LogWarning(fmt.Sprintf("Could not create PostHog client: %s", err))
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

	client.Enqueue(posthog.Capture{
		DistinctId: t.distinctId,
		Event:      eventName,
		Properties: propertiesMap,
	})
}

func (t *Telemetry) CaptureError(eventName string, properties map[string]interface{}) {
	if !t.isEnabled {
		return
	}

	client := posthog.New(t.token)
	defer client.Close()

	propertiesMap := posthog.NewProperties().
		Set("error", true).
		Set("version", version.GetVersion()).
		Set("os", runtime.GOOS).
		Set("arch", runtime.GOARCH)

	for k, v := range properties {
		propertiesMap.Set(k, v)
	}

	client.Enqueue(posthog.Capture{
		DistinctId: t.distinctId,
		Event:      eventName,
		Properties: propertiesMap,
	})
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
		u.LogWarning(fmt.Sprintf("Could not load cache: %s", err))
		return nil
	}

	if cacheCfg.AtmosInstanceId == "" {
		cacheCfg.AtmosInstanceId = uuid.New().String()
	}
	if saveErr := cfg.SaveCache(cacheCfg); saveErr != nil {
		u.LogWarning(fmt.Sprintf("Unable to save cache: %s", saveErr))
	}
	token := DefaultTelemetryToken

	if atmosConfig.Settings.Telemetry.Token != "" {
		token = atmosConfig.Settings.Telemetry.Token
	}

	return NewTelemetry(atmosConfig.Settings.Telemetry.Enabled, token, cacheCfg.AtmosInstanceId)
}

func CaptureCmdString(cmdString string) {
	telemetry := InitializeTelemetry()
	telemetry.CaptureEvent(DefaultEventName, map[string]interface{}{"command": cmdString})
}

func CaptureCmdFailureString(cmdString string) {
	telemetry := InitializeTelemetry()
	telemetry.CaptureError(DefaultEventName, map[string]interface{}{"command": cmdString})
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
