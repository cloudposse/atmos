package errors

import (
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/getsentry/sentry-go"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// CloseSentryTimeout is the timeout for flushing Sentry events before shutdown.
	CloseSentryTimeout = 2 * time.Second
)

// InitializeSentry initializes the Sentry SDK with the provided configuration.
func InitializeSentry(config *schema.SentryConfig) error {
	if config == nil || !config.Enabled {
		return nil
	}

	// Set default sample rate if not specified.
	sampleRate := config.SampleRate
	if sampleRate == 0 {
		sampleRate = 1.0
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              config.DSN,
		Environment:      config.Environment,
		Release:          config.Release,
		Debug:            config.Debug,
		SampleRate:       sampleRate,
		AttachStacktrace: config.CaptureStackContext,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize Sentry: %w", err)
	}

	// Set global tags if provided.
	for key, value := range config.Tags {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTag(key, value)
		})
	}

	return nil
}

// CloseSentry flushes any pending Sentry events and closes the client.
// Deprecated: Use CloseAllSentry() instead to properly close all component-specific clients.
func CloseSentry() {
	sentry.Flush(CloseSentryTimeout)
}

// CloseAllSentry closes all Sentry clients in the registry.
func CloseAllSentry() {
	// Close global client.
	CloseSentry()

	// Close all component-specific clients.
	GetRegistry().CloseAll()
}

// CaptureError captures an error and sends it to Sentry using cockroachdb/errors native support.
// This uses BuildSentryReport which automatically handles PII-free reporting, stack traces, and safe details.
func CaptureError(err error) {
	if err == nil {
		return
	}

	// Build Sentry report using cockroachdb/errors native support.
	// This automatically extracts stack traces, safe details, and ensures PII-free reporting.
	event, extraDetails := errors.BuildSentryReport(err)

	// Get hub to configure scope.
	hub := sentry.CurrentHub()

	hub.WithScope(func(scope *sentry.Scope) {
		// Add extra details from cockroachdb/errors as context.
		for key, value := range extraDetails {
			if contextMap, ok := value.(map[string]interface{}); ok {
				scope.SetContext(key, contextMap)
			}
		}

		// Extract and set hints as breadcrumbs.
		hints := errors.GetAllHints(err)
		for _, hint := range hints {
			scope.AddBreadcrumb(&sentry.Breadcrumb{
				Type:     "info",
				Category: "hint",
				Message:  hint,
				Level:    sentry.LevelInfo,
			}, 100) // Max breadcrumbs.
		}

		// Extract exit code if present and add to event tags.
		exitCode := GetExitCode(err)
		if exitCode != 0 && exitCode != 1 {
			event.Tags["atmos.exit_code"] = fmt.Sprintf("%d", exitCode)
		}

		// Capture the pre-built event.
		hub.CaptureEvent(event)
	})
}

// CaptureErrorWithContext captures an error with additional Atmos context using cockroachdb/errors native support.
// Context includes component, stack, region, etc.
func CaptureErrorWithContext(err error, context map[string]string) {
	if err == nil {
		return
	}

	// Build Sentry report using cockroachdb/errors native support.
	event, extraDetails := errors.BuildSentryReport(err)

	hub := sentry.CurrentHub()

	hub.WithScope(func(scope *sentry.Scope) {
		// Add extra details from cockroachdb/errors as context.
		for key, value := range extraDetails {
			if contextMap, ok := value.(map[string]interface{}); ok {
				scope.SetContext(key, contextMap)
			}
		}

		// Set Atmos context as tags with "atmos." prefix.
		for key, value := range context {
			event.Tags["atmos."+key] = value
		}

		// Extract and set hints as breadcrumbs.
		hints := errors.GetAllHints(err)
		for _, hint := range hints {
			scope.AddBreadcrumb(&sentry.Breadcrumb{
				Type:     "info",
				Category: "hint",
				Message:  hint,
				Level:    sentry.LevelInfo,
			}, 100)
		}

		// Extract exit code if present.
		exitCode := GetExitCode(err)
		if exitCode != 0 && exitCode != 1 {
			event.Tags["atmos.exit_code"] = fmt.Sprintf("%d", exitCode)
		}

		// Capture the pre-built event.
		hub.CaptureEvent(event)
	})
}

// CaptureErrorWithComponentConfig captures an error using component-specific Sentry configuration.
// It uses the merged error configuration from component settings, falling back to global config.
func CaptureErrorWithComponentConfig(err error, info *schema.ConfigAndStacksInfo, context map[string]string) {
	if err == nil {
		return
	}

	// Get component error configuration.
	componentErrorConfig, configErr := GetComponentErrorConfig(info)
	if configErr != nil {
		// Log error but continue with global config.
		if atmosConfig != nil {
			componentErrorConfig = &atmosConfig.Errors
		} else {
			// Can't get any config - use global Sentry hub.
			CaptureErrorWithContext(err, context)
			return
		}
	}

	// Merge with global config if available.
	var finalConfig *schema.ErrorsConfig
	if atmosConfig != nil {
		finalConfig = MergeErrorConfigs(&atmosConfig.Errors, componentErrorConfig)
	} else if componentErrorConfig != nil {
		finalConfig = componentErrorConfig
	} else {
		// No config available - use global hub.
		CaptureErrorWithContext(err, context)
		return
	}

	// Get or create Sentry client for this configuration.
	hub, hubErr := GetRegistry().GetOrCreateClient(&finalConfig.Sentry)
	if hubErr != nil {
		// Failed to create client - fall back to global hub.
		CaptureErrorWithContext(err, context)
		return
	}

	// If Sentry is disabled for this component, return early.
	if hub == nil {
		return
	}

	// Build Sentry report using cockroachdb/errors native support.
	event, extraDetails := errors.BuildSentryReport(err)

	hub.WithScope(func(scope *sentry.Scope) {
		// Add extra details from cockroachdb/errors as context.
		for key, value := range extraDetails {
			if contextMap, ok := value.(map[string]interface{}); ok {
				scope.SetContext(key, contextMap)
			}
		}

		// Set Atmos context as tags with "atmos." prefix.
		for key, value := range context {
			event.Tags["atmos."+key] = value
		}

		// Add component identification tags.
		if info != nil {
			if info.Component != "" {
				event.Tags["atmos.component"] = info.Component
			}
			if info.Stack != "" {
				event.Tags["atmos.stack"] = info.Stack
			}
			if info.ComponentType != "" {
				event.Tags["atmos.component_type"] = info.ComponentType
			}
		}

		// Extract and set hints as breadcrumbs.
		hints := errors.GetAllHints(err)
		for _, hint := range hints {
			scope.AddBreadcrumb(&sentry.Breadcrumb{
				Type:     "info",
				Category: "hint",
				Message:  hint,
				Level:    sentry.LevelInfo,
			}, 100)
		}

		// Extract exit code if present.
		exitCode := GetExitCode(err)
		if exitCode != 0 && exitCode != 1 {
			event.Tags["atmos.exit_code"] = fmt.Sprintf("%d", exitCode)
		}

		// Capture the pre-built event.
		hub.CaptureEvent(event)
	})
}
