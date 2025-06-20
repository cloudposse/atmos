package telemetry

import (
	"runtime"

	log "github.com/charmbracelet/log"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
	"github.com/spf13/cobra"
)

// CommandEventName is the standard event name used for command telemetry
const (
	CommandEventName = "command"
)

// GetTelemetryFromConfig initializes a new Telemetry client by loading configuration
// from the Atmos config file and cache. It handles installation ID generation
// and provides optional dependency injection for testing via the provider parameter.
func GetTelemetryFromConfig(provider ...TelemetryClientProvider) *Telemetry {
	// Load Atmos configuration from config file
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		log.Warn("Could not load config", "error", err)
		return nil
	}

	// Load the cache to retrieve or generate installation ID
	cacheCfg, err := cfg.LoadCache()
	if err != nil {
		log.Warn("Could not load cache", "error", err)
		return nil
	}

	// Generate new installation ID if one doesn't exist
	if cacheCfg.InstallationId == "" {
		cacheCfg.InstallationId = uuid.New().String()
	}
	// Save the cache with the installation ID
	if saveErr := cfg.SaveCache(cacheCfg); saveErr != nil {
		log.Warn("Unable to save cache", "error", saveErr)
	}

	// Extract telemetry settings from config
	enabled := atmosConfig.Settings.Telemetry.Enabled
	token := atmosConfig.Settings.Telemetry.Token
	endpoint := atmosConfig.Settings.Telemetry.Endpoint
	distinctId := cacheCfg.InstallationId

	// Use provided client provider or default to PostHog provider
	clientProvider := PosthogClientProvider
	if len(provider) > 0 {
		clientProvider = provider[0]
	}

	return NewTelemetry(enabled, token, endpoint, distinctId, clientProvider)
}

// atmosProWorkspaceID retrieves the Atmos Pro workspace ID from the configuration.
// Returns an empty string if the config cannot be loaded.
func atmosProWorkspaceID() string {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		log.Warn("Could not load config", "error", err)
		return ""
	}

	return atmosConfig.Settings.Pro.WorkspaceID
}

// captureCmdString captures telemetry for a command string with the given error.
// It creates a telemetry client and sends an event with comprehensive properties
// including system info, CI environment, and command details.
func captureCmdString(cmdString string, err error, provider ...TelemetryClientProvider) {
	// Get telemetry client from config
	if t := GetTelemetryFromConfig(provider...); t != nil {
		// Build comprehensive properties for the telemetry event
		properties := posthog.NewProperties().
			Set("version", version.Version).                      // Atmos version
			Set("os", runtime.GOOS).                              // Operating system
			Set("arch", runtime.GOARCH).                          // Architecture
			Set("error", err != nil).                             // Whether an error occurred
			Set("command", cmdString).                            // The command that was executed
			Set("ci", isCI()).                                    // Whether running in CI
			Set("ci_provider", ciProvider()).                     // Which CI provider is being used
			Set("atmos_pro_workspace_id", atmosProWorkspaceID()). // Atmos Pro workspace ID
			Set("is_docker", isDocker())                          // Whether running in Docker

		// Capture the telemetry event
		t.Capture(CommandEventName, properties)
	}
}

// captureCmd captures telemetry for a cobra command with the given error.
// It extracts the command path and delegates to captureCmdString.
func captureCmd(cmd *cobra.Command, err error, provider ...TelemetryClientProvider) {
	captureCmdString(cmd.CommandPath(), err, provider...)
}

// CaptureCmdString is the public API for capturing command string telemetry.
// It accepts an optional error parameter and handles the case where no error is provided.
func CaptureCmdString(cmdString string, err ...error) {
	var inErr error
	if len(err) > 0 {
		inErr = err[0]
	}
	captureCmdString(cmdString, inErr)
}

// CaptureCmd is the public API for capturing cobra command telemetry.
// It accepts an optional error parameter and handles the case where no error is provided.
func CaptureCmd(cmd *cobra.Command, err ...error) {
	var inErr error
	if len(err) > 0 {
		inErr = err[0]
	}
	captureCmd(cmd, inErr)
}
