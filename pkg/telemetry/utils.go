package telemetry

import (
	"fmt"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	DefaultEventName = "command"
)

// GetTelemetryFromConfig initializes a new Telemetry client.
func GetTelemetryFromConfig(provider ...TelemetryClientProvider) *Telemetry {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		log.Warn(fmt.Sprintf("Could not load config: %s", err))
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

	enabled := atmosConfig.Settings.Telemetry.Enabled
	token := atmosConfig.Settings.Telemetry.Token
	endpoint := atmosConfig.Settings.Telemetry.Endpoint
	distinctId := cacheCfg.AtmosInstanceId
	clientProvider := PosthogClientProvider
	if len(provider) > 0 {
		clientProvider = provider[0]
	}

	return NewTelemetry(enabled, token, endpoint, distinctId, clientProvider)
}

func CaptureCmdString(cmdString string, provider ...TelemetryClientProvider) {
	if t := GetTelemetryFromConfig(provider...); t != nil {
		t.CaptureEvent(DefaultEventName, map[string]interface{}{"command": cmdString})
	}
}

func CaptureCmdFailureString(cmdString string, provider ...TelemetryClientProvider) {
	if t := GetTelemetryFromConfig(provider...); t != nil {
		t.CaptureError(DefaultEventName, map[string]interface{}{"command": cmdString})
	}
}

func CaptureCmd(cmd *cobra.Command, provider ...TelemetryClientProvider) {
	CaptureCmdString(cmd.CommandPath(), provider...)
}

func CaptureCmdFailure(cmd *cobra.Command, provider ...TelemetryClientProvider) {
	CaptureCmdFailureString(cmd.CommandPath(), provider...)
}
