package telemetry

import (
	_ "embed"
	"runtime"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
	"github.com/spf13/cobra"
)

// CommandEventName is the standard event name used for command telemetry.
const CommandEventName = "command"

//go:embed markdown/telemetry_notice.md
var telemetryNoticeMarkdown string

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

// PrintTelemetryDisclosure displays the telemetry disclosure message if one is available.
// It calls disclosureMessage() to get the message and prints to stderr with markdown
// formatting if a message is returned.
func PrintTelemetryDisclosure() {
	if message := disclosureMessage(); message != "" {
		utils.PrintfMarkdownToTUI("%s", message)
	}
}

// getTelemetryFromConfig initializes a new Telemetry client by loading configuration
// from the Atmos config file and cache. It handles installation ID generation
// and provides optional dependency injection for testing via the provider parameter.
func getTelemetryFromConfig(provider ...TelemetryClientProvider) *Telemetry {
	// Load Atmos configuration from config file
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		log.Warn("Could not load config", "error", err)
		return nil
	}

	// Extract telemetry settings from config.
	enabled := atmosConfig.Settings.Telemetry.Enabled
	token := atmosConfig.Settings.Telemetry.Token
	endpoint := atmosConfig.Settings.Telemetry.Endpoint
	logging := atmosConfig.Settings.Telemetry.Logging
	distinctId := getOrInitializeCacheValue(
		func(cfg *cfg.CacheConfig) string {
			return cfg.InstallationId
		},
		func(cfg *cfg.CacheConfig, value string) {
			cfg.InstallationId = value
		},
		uuid.New().String(),
	)

	// Use provided client provider or default to PostHog provider.
	clientProvider := PosthogClientProvider
	if len(provider) > 0 {
		clientProvider = provider[0]
	}

	return NewTelemetry(enabled, token, Options{
		Endpoint:   endpoint,
		DistinctID: distinctId,
		Logging:    logging,
	}, clientProvider)
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
	if t := getTelemetryFromConfig(provider...); t != nil {
		// Build comprehensive properties for the telemetry event
		properties := posthog.NewProperties().
			Set("version", version.Version).                      // Atmos version
			Set("os", runtime.GOOS).                              // Operating system
			Set("arch", runtime.GOARCH).                          // Architecture
			Set("error", err != nil).                             // Whether an error occurred
			Set("command", cmdString).                            // The command that was executed
			Set("ci", IsCI()).                                    // Whether running in CI
			Set("ci_provider", ciProvider()).                     // Which CI provider is being used
			Set("atmos_pro_workspace_id", atmosProWorkspaceID()). // Atmos Pro workspace ID
			Set("docker", isDocker())                             // Whether running in Docker

		// Capture the telemetry event
		t.Capture(CommandEventName, properties)
	}
}

// captureCmd captures telemetry for a cobra command with the given error.
// It extracts the command path and delegates to captureCmdString.
func captureCmd(cmd *cobra.Command, err error, provider ...TelemetryClientProvider) {
	captureCmdString(cmd.CommandPath(), err, provider...)
}

// disclosureMessage returns a telemetry disclosure message if it hasn't been shown before.
// It checks if telemetry is enabled and not running in CI, then loads the cache to
// determine if the disclosure has been displayed previously. If not shown, it marks
// the disclosure as shown in the cache and returns the disclosure message.
func disclosureMessage() string {
	// Do not show disclosure if running in CI
	if IsCI() {
		return ""
	}

	// Only show disclosure if telemetry is enabled
	telemetry := getTelemetryFromConfig()
	if telemetry == nil || !telemetry.isEnabled {
		return ""
	}

	TelemetryDisclosureShown := getOrInitializeCacheValue(
		func(cfg *cfg.CacheConfig) bool {
			return cfg.TelemetryDisclosureShown
		},
		func(cfg *cfg.CacheConfig, _ bool) {
			cfg.TelemetryDisclosureShown = true
		},
		false,
	)

	// If disclosure has already been shown, return empty
	if TelemetryDisclosureShown {
		return ""
	}
	return telemetryNoticeMarkdown
}

// getOrInitializeCacheValue retrieves a value from cache or initializes it with a default value if not present.
// It uses getter and initializer functions to access and modify the cache configuration, and saves the cache
// if a new value needs to be initialized.
//
// Parameters:
//   - getter: Function to retrieve the current value from cache configuration
//   - initializer: Function to set a new value in cache configuration
//   - defaultValue: The default value to use if the current value is empty/zero
//
// Returns:
//   - T: The current value from cache if it exists, otherwise the default value
//
// The function loads the cache configuration, checks if a value already exists using the getter.
// If the value is empty/zero, it initializes it with the default value using the initializer and saves
// the updated cache. If cache operations fail, it logs a disclosure but continues execution.
func getOrInitializeCacheValue[T comparable](getter func(cfg *cfg.CacheConfig) T, initializer func(cfg *cfg.CacheConfig, value T), defaultValue T) T {
	var emptyValue T

	cacheCfg, err := cfg.LoadCache()
	canCreateCache := err == nil

	currentValue := getter(&cacheCfg)

	if currentValue != emptyValue {
		return currentValue
	}

	if canCreateCache {
		initializer(&cacheCfg, defaultValue)
		if err := cfg.SaveCache(cacheCfg); err != nil {
			log.Warn("Could not save cache", "error", err)
		}
	}

	return defaultValue
}
