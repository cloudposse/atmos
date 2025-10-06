package errors

import (
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/getsentry/sentry-go"

	"github.com/cloudposse/atmos/pkg/schema"
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
func CloseSentry() {
	const flushTimeout = 2 * time.Second
	sentry.Flush(flushTimeout)
}

// CaptureError captures an error and sends it to Sentry with full context.
// It extracts hints, safe details, and stack context from cockroachdb/errors.
func CaptureError(err error) {
	if err == nil {
		return
	}

	// Get hub to configure scope.
	hub := sentry.CurrentHub()

	hub.WithScope(func(scope *sentry.Scope) {
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

		// Extract and set safe details as tags.
		details := errors.GetAllSafeDetails(err)
		for _, detail := range details {
			for _, safeDetail := range detail.SafeDetails {
				// Add safe details as tags with "error." prefix.
				scope.SetTag("error.detail", safeDetail)
			}
		}

		// Extract exit code if present.
		exitCode := GetExitCode(err)
		if exitCode != 0 && exitCode != 1 {
			scope.SetTag("atmos.exit_code", fmt.Sprintf("%d", exitCode))
		}

		// Capture the exception with stack trace.
		hub.CaptureException(err)
	})
}

// CaptureErrorWithContext captures an error with additional Atmos context.
// Context includes component, stack, region, etc.
func CaptureErrorWithContext(err error, context map[string]string) {
	if err == nil {
		return
	}

	hub := sentry.CurrentHub()

	hub.WithScope(func(scope *sentry.Scope) {
		// Set Atmos context as tags with "atmos." prefix.
		for key, value := range context {
			scope.SetTag("atmos."+key, value)
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

		// Extract and set safe details as tags.
		details := errors.GetAllSafeDetails(err)
		for _, detail := range details {
			for _, safeDetail := range detail.SafeDetails {
				scope.SetTag("error.detail", safeDetail)
			}
		}

		// Extract exit code if present.
		exitCode := GetExitCode(err)
		if exitCode != 0 && exitCode != 1 {
			scope.SetTag("atmos.exit_code", fmt.Sprintf("%d", exitCode))
		}

		// Capture the exception.
		hub.CaptureException(err)
	})
}
