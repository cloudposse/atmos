package telemetry

import (
	"fmt"
	"runtime"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
	"github.com/spf13/cobra"
)

const (
	CommandEventName = "command"
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

	if cacheCfg.InstallationId == "" {
		cacheCfg.InstallationId = uuid.New().String()
	}
	if saveErr := cfg.SaveCache(cacheCfg); saveErr != nil {
		log.Warn(fmt.Sprintf("Unable to save cache: %s", saveErr))
	}

	enabled := atmosConfig.Settings.Telemetry.Enabled
	token := atmosConfig.Settings.Telemetry.Token
	endpoint := atmosConfig.Settings.Telemetry.Endpoint
	distinctId := cacheCfg.InstallationId
	clientProvider := PosthogClientProvider
	if len(provider) > 0 {
		clientProvider = provider[0]
	}

	return NewTelemetry(enabled, token, endpoint, distinctId, clientProvider)
}

func captureCmdString(cmdString string, err error, provider ...TelemetryClientProvider) {
	if t := GetTelemetryFromConfig(provider...); t != nil {
		properties := posthog.NewProperties().
			Set("version", version.Version).
			Set("os", runtime.GOOS).
			Set("arch", runtime.GOARCH).
			Set("error", err != nil).
			Set("command", cmdString)

		t.Capture(CommandEventName, properties)
	}
}

func captureCmd(cmd *cobra.Command, err error, provider ...TelemetryClientProvider) {
	captureCmdString(cmd.CommandPath(), err, provider...)
}

func CaptureCmdString(cmdString string, err ...error) {
	var inErr error
	if len(err) > 0 {
		inErr = err[0]
	}
	captureCmdString(cmdString, inErr)
}

func CaptureCmd(cmd *cobra.Command, err ...error) {
	var inErr error
	if len(err) > 0 {
		inErr = err[0]
	}
	captureCmd(cmd, inErr)
}
