package pro

import (
	"context"
	"errors"
	"maps"
	"os"
	"reflect"
	"strconv"
	"sync"

	"github.com/getsentry/sentry-go"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

// ErrorReporter owns exception destinations and duplicate tracking for one invocation.
type ErrorReporter struct {
	mu           sync.Mutex
	settings     schema.ProSettings
	errors       schema.ErrorsConfig
	runtime      map[string]string
	repositoryID string
	eligible     bool
	envEnabled   *bool
	options      ExceptionTransportOptions
	proHub       *sentry.Hub
	registry     errUtils.SentryClientRegistry
	seen         map[error]bool
	closed       bool
}

var _ errUtils.ErrorReporter = (*ErrorReporter)(nil)

// NewErrorReporter snapshots invocation settings. No network requests occur at startup.
func NewErrorReporter(config *schema.AtmosConfiguration, executionID string, options ExceptionTransportOptions) *ErrorReporter {
	defer perf.Track(config, "pro.NewErrorReporter")()

	r := &ErrorReporter{runtime: make(map[string]string), seen: make(map[error]bool), options: options}
	if config != nil {
		r.settings, r.errors = config.Settings.Pro, config.Errors
	}
	r.errors.Sentry.Tags = maps.Clone(r.errors.Sentry.Tags)
	r.runtime["execution_id"], r.runtime["version"] = executionID, version.Version
	// These are runtime CI identity values, not user configuration.
	//nolint:forbidigo // CI identity must come from the current process environment.
	r.repositoryID = os.Getenv("GITHUB_REPOSITORY_ID")
	//nolint:forbidigo // CI runtime detection, matching the endpoint's authentication contract.
	r.eligible = os.Getenv("GITHUB_ACTIONS") == "true" && r.repositoryID != "" && r.settings.GithubOIDC.RequestURL != "" && r.settings.GithubOIDC.RequestToken != ""
	for key, env := range map[string]string{"git_sha": "GITHUB_SHA", "atmos_pro_run_id": "ATMOS_PRO_RUN_ID", "github_run_id": "GITHUB_RUN_ID", "github_job": "GITHUB_JOB"} {
		//nolint:forbidigo // Runtime CI correlation values, like execution uploads.
		r.runtime[key] = os.Getenv(env)
	}
	// Remember whether the environment override was supplied, so component settings cannot undo it.
	if value, ok := os.LookupEnv("ATMOS_PRO_ERRORS_ENABLED"); ok {
		if enabled, err := strconv.ParseBool(value); err == nil {
			r.envEnabled = &enabled
		}
	}
	return r
}

// Capture sends each component failure once, even when an aggregate is later printed again.
func (r *ErrorReporter) Capture(err error, context map[string]string) {
	defer perf.Track(nil, "pro.ErrorReporter.Capture")()

	if err == nil {
		return
	}
	if components := reportingErrors(err); len(components) > 0 {
		for _, component := range components {
			if !silentReportingError(component) && component.Claim() {
				info, executionID := component.ReportingContext()
				r.capture(component.Unwrap(), &info, executionID, context)
			}
		}
		return
	}
	if silentReportingError(err) {
		return
	}
	r.mu.Lock()
	duplicate := false
	if reflect.ValueOf(err).Comparable() {
		duplicate = r.seen[err]
		r.seen[err] = true
	}
	r.mu.Unlock()
	if !duplicate {
		r.capture(err, &schema.ConfigAndStacksInfo{}, "", context)
	}
}

func silentReportingError(err error) bool {
	var exit errUtils.ExitCodeError
	return errors.As(err, &exit) && (exit.Code == 0 || exit.Silent)
}

//nolint:errorlint // Traverse immediate children so errors.Join preserves every component occurrence.
func reportingErrors(err error) []*errUtils.ReportingError {
	if reported, ok := err.(*errUtils.ReportingError); ok {
		return []*errUtils.ReportingError{reported}
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		var result []*errUtils.ReportingError
		for _, cause := range wrapped.Unwrap() {
			result = append(result, reportingErrors(cause)...)
		}
		return result
	case interface{ Unwrap() error }:
		return reportingErrors(wrapped.Unwrap())
	}
	return nil
}

func (r *ErrorReporter) capture(err error, info *schema.ConfigAndStacksInfo, executionID string, context map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	componentConfig, configErr := errUtils.GetComponentErrorConfig(info)
	if configErr != nil {
		log.Debug("Invalid component error settings; using invocation defaults")
	}
	config := errUtils.MergeErrorConfigs(&r.errors, componentConfig)
	runtime := r.eventContext(info, executionID, context)
	event := buildExceptionEvent(err, info, config.Sentry.Tags, runtime)
	event.Environment = config.Sentry.Environment
	if config.Sentry.Release != "" {
		event.Release = config.Sentry.Release
	}
	masked, maskErr := maskedExceptionEvent(event)
	if maskErr != nil {
		log.Debug("Exception skipped: event cannot be safely encoded")
		return
	}
	// Tags are already merged, identity-protected, and masked in the event.
	// A scope would overwrite those values with raw configuration after masking.
	userConfig := config.Sentry
	userConfig.Tags = nil
	userHub, hubErr := r.registry.GetOrCreateClient(&userConfig)
	if hubErr != nil {
		log.Debug("Failed to initialize Sentry destination")
	}
	if userHub != nil {
		userHub.CaptureEvent(masked)
	}
	if !r.eligible || !r.proEnabled(info) {
		return
	}
	r.capturePro(event)
}

func (r *ErrorReporter) eventContext(info *schema.ConfigAndStacksInfo, executionID string, context map[string]string) map[string]string {
	runtime := maps.Clone(r.runtime)
	for key, value := range context {
		runtime[key] = value
	}
	if executionID != "" {
		runtime["execution_id"] = executionID
	}
	if info.SubCommand != "" {
		runtime["command"] = info.ComponentType + " " + info.SubCommand
	}
	return runtime
}

func (r *ErrorReporter) capturePro(event *sentry.Event) {
	if r.proHub == nil {
		transport, transportErr := NewExceptionTransport(&r.settings, r.repositoryID, r.options)
		if transportErr != nil {
			log.Debug("Invalid Pro exception destination")
			return
		}
		client, clientErr := sentry.NewClient(sentry.ClientOptions{Dsn: exceptionDSN, Transport: transport, Integrations: func([]sentry.Integration) []sentry.Integration { return nil }})
		if clientErr != nil {
			transport.Close()
			return
		}
		r.proHub = sentry.NewHub(client, sentry.NewScope())
	}
	// The SDK mutates captured events; Pro receives its own detached copy.
	copy, copyErr := maskedExceptionEvent(event)
	if copyErr == nil {
		r.proHub.CaptureEvent(copy)
	}
}

func (r *ErrorReporter) proEnabled(info *schema.ConfigAndStacksInfo) bool {
	enabled, report := r.settings.Enabled, true
	if r.settings.Errors.Enabled != nil {
		report = *r.settings.Errors.Enabled
	}
	if settings, ok := info.ComponentSettingsSection["pro"].(map[string]any); ok {
		if value, ok := settings["enabled"].(bool); ok {
			enabled = value
		}
	}
	settings, _ := info.ComponentSettingsSection["pro"].(map[string]any)
	errorSettings, _ := settings["errors"].(map[string]any)
	if value, ok := errorSettings["enabled"].(bool); ok {
		report = value
	}
	if r.envEnabled != nil {
		report = *r.envEnabled
	}
	return enabled && report
}

// Flush drains all destinations against one deadline, then closes their workers.
func (r *ErrorReporter) Flush(ctx context.Context) {
	defer perf.Track(nil, "pro.ErrorReporter.Flush")()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	hub := r.proHub
	r.mu.Unlock()
	r.registry.Flush(ctx)
	if hub != nil {
		hub.FlushWithContext(ctx)
		hub.Client().Close()
	}
}
