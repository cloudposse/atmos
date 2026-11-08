package component

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func prepareWith(t *testing.T, command string) error {
	t.Helper()
	request := &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress(), Operation: &pluginv1.ComponentOperation{Command: command}}
	_, err := fixtureProvider(t).Validate(t.Context(), request)
	return err
}

func TestStructuredFailureRendersSafeDetail(t *testing.T) {
	err := prepareWith(t, "detail-safe")
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	assert.Equal(t, "Unsupported Terragrunt setting", err.Error())

	text := strings.Join(errUtils.AllDetails(err), "\n")
	for _, want := range []string{"Component `vpc` in stack `live/dev`", "live/dev/vpc/terragrunt.hcl:7:3", "setting `terraform_binary`", "during prepare", "terraform_binary is not implemented."} {
		assert.Contains(t, text, want)
	}
	assert.Equal(t, []string{"Remove terraform_binary."}, errUtils.AllHints(err))
	location, found := errUtils.GetLocation(err)
	require.True(t, found)
	assert.Equal(t, "live/dev/vpc/terragrunt.hcl", location.File)
	assert.Equal(t, 7, location.Line)
}

func TestStructuredFailureWithholdsUnsafeDetail(t *testing.T) {
	err := prepareWith(t, "detail-unsafe")
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	rendered := err.Error() + strings.Join(errUtils.AllDetails(err), "\n")
	assert.NotContains(t, rendered, "ghp_SECRET")
	hints := errUtils.AllHints(err)
	require.Len(t, hints, 2)
	assert.Equal(t, "Remove terraform_binary.", hints[0])
	assert.Contains(t, hints[1], "ATMOS_LOGS_LEVEL=Debug")
}

func TestErrorSeverityDiagnosticFailsAndWarningDoesNot(t *testing.T) {
	request := &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress(), Operation: &pluginv1.ComponentOperation{Command: "diagnostic-error"}}
	_, err := fixtureProvider(t).Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Equal(t, "Bad setting", err.Error())

	warning := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "skipped", Summary: "Skipped", Severity: pluginv1.Severity_SEVERITY_WARNING}}
	require.NoError(t, firstDiagnosticError([]*pluginv1.Diagnostic{warning}))
	require.ErrorIs(t, firstDiagnosticError([]*pluginv1.Diagnostic{warning, {Field: "x", Message: "legacy"}}), errUtils.ErrPluginInvalidConfig)
}

func TestMissingDetailFallsBackToSentinel(t *testing.T) {
	require.ErrorIs(t, DetailError(nil, 0), errUtils.ErrPluginAPI)
}

// A legacy diagnostic keeps its message as unsafe detail: never shown, but pointed at by the debug hint.
func TestLegacyDiagnosticKeepsItsMessageForDebugLogging(t *testing.T) {
	err := DiagnosticError(&pluginv1.Diagnostic{Field: "region", Message: "region ghp_LEGACY is invalid"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	rendered := err.Error() + strings.Join(errUtils.AllDetails(err), "\n")
	assert.NotContains(t, rendered, "ghp_LEGACY")
	hints := errUtils.AllHints(err)
	require.NotEmpty(t, hints)
	assert.Contains(t, hints[len(hints)-1], "ATMOS_LOGS_LEVEL=Debug")
	var failure *Failure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "region ghp_LEGACY is invalid", failure.Detail.Detail)
	assert.False(t, failure.Detail.UserSafe)
}

// The attached location is masked before annotation escaping can hide a mask literal from the
// final masker.
func TestDiagnosticLocationIsMaskedBeforeItIsAttached(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())
	atmosio.RegisterSecret("tok,en-7d3a")
	err := DetailError(&pluginv1.ErrorDetail{Code: "bad", Summary: "Bad", Location: &pluginv1.SourceLocation{File: "live/tok,en-7d3a/terragrunt.hcl", Line: 3}}, 0)
	location, found := errUtils.GetLocation(err)
	require.True(t, found)
	assert.NotContains(t, location.File, "tok,en-7d3a")
	assert.Equal(t, 3, location.Line)
}

// captureLogs sends the Atmos logger to a buffer at the given level for one test.
func captureLogs(t *testing.T, level charm.Level) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previousLevel := log.GetLevel()
	log.SetOutput(&output)
	log.SetLevel(level)
	ResetShownMessages()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(previousLevel)
		ResetShownMessages()
	})
	return &output
}

func logEntry(level pluginv1.LogLevel, message string, fields map[string]string) *pluginv1.LogEntry {
	return &pluginv1.LogEntry{Level: level, Message: message, Fields: fields, Unit: "live/dev/app"}
}

func TestRenderLogsHonorsAtmosLevelAndScopesEntries(t *testing.T) {
	entries := []*pluginv1.LogEntry{
		logEntry(pluginv1.LogLevel_LOG_LEVEL_TRACE, "trace event", nil),
		logEntry(pluginv1.LogLevel_LOG_LEVEL_DEBUG, "debug event", map[string]string{"b": "2", "a": "1"}),
		logEntry(pluginv1.LogLevel_LOG_LEVEL_INFO, "info event", nil),
		logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "warn event", nil),
		logEntry(pluginv1.LogLevel_LOG_LEVEL_ERROR, "error event", nil),
		logEntry(pluginv1.LogLevel_LOG_LEVEL_UNSPECIFIED, "ignored event", nil),
	}
	scope := LogScope{Provider: "terragrunt", Component: "app", Stack: "live/dev"}

	warn := captureLogs(t, charm.WarnLevel)
	RenderLogs(scope, entries)
	assert.Contains(t, warn.String(), "warn event")
	assert.Contains(t, warn.String(), "error event")
	for _, hidden := range []string{"trace event", "debug event", "info event", "ignored event"} {
		assert.NotContains(t, warn.String(), hidden, "hidden by default at the Warn level")
	}
	assert.Contains(t, warn.String(), "component=app")
	assert.Contains(t, warn.String(), "stack=live/dev")
	assert.Contains(t, warn.String(), "plugin=terragrunt")
	assert.Contains(t, warn.String(), "unit=live/dev/app")

	debug := captureLogs(t, charm.DebugLevel)
	RenderLogs(scope, entries)
	assert.Contains(t, debug.String(), "debug event")
	assert.Contains(t, debug.String(), "a=1 b=2", "fields render in key order")
	assert.NotContains(t, debug.String(), "trace event")

	trace := captureLogs(t, log.TraceLevel)
	RenderLogs(scope, entries)
	assert.Contains(t, trace.String(), "trace event")
}

func TestRenderLogsMasksSecretsInMessagesAndFields(t *testing.T) {
	atmosio.RegisterSecret("log-secret-value-77ab")
	output := captureLogs(t, charm.DebugLevel)

	RenderLogs(LogScope{Component: "app", Stack: "dev"}, []*pluginv1.LogEntry{
		logEntry(pluginv1.LogLevel_LOG_LEVEL_DEBUG, "token log-secret-value-77ab leaked", map[string]string{"auth": "Bearer log-secret-value-77ab"}),
	})

	assert.NotContains(t, output.String(), "log-secret-value-77ab")
	assert.Contains(t, output.String(), "leaked")
}

func TestRenderLogsShowsAWarningOncePerComponentAndStack(t *testing.T) {
	output := captureLogs(t, charm.DebugLevel)
	warning := []*pluginv1.LogEntry{logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "state is not encrypted", nil)}
	debug := []*pluginv1.LogEntry{logEntry(pluginv1.LogLevel_LOG_LEVEL_DEBUG, "evaluating again", nil)}

	for range 3 {
		RenderLogs(LogScope{Component: "app", Stack: "dev"}, append(slices.Clone(warning), debug...))
	}
	RenderLogs(LogScope{Component: "app", Stack: "prod"}, warning)

	assert.Equal(t, 2, strings.Count(output.String(), "state is not encrypted"), "once per component and stack")
	assert.Equal(t, 3, strings.Count(output.String(), "evaluating again"), "debug events are not deduplicated")
}

func TestRenderLogsKeepsWarningsThatDifferInTheirFields(t *testing.T) {
	output := captureLogs(t, charm.DebugLevel)
	scope := LogScope{Provider: "terragrunt", Component: "app", Stack: "dev"}
	for range 2 {
		RenderLogs(scope, []*pluginv1.LogEntry{
			logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "dependency has no outputs", map[string]string{"dependency": "network"}),
			logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "dependency has no outputs", map[string]string{"dependency": "database"}),
		})
	}
	RenderLogs(LogScope{Provider: "other", Component: "app", Stack: "dev"}, []*pluginv1.LogEntry{
		logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "dependency has no outputs", map[string]string{"dependency": "network"}),
	})

	assert.Equal(t, 3, strings.Count(output.String(), "dependency has no outputs"), "each dependency once, and once more for another provider")
	assert.Contains(t, output.String(), "dependency=network")
	assert.Contains(t, output.String(), "dependency=database")
}

func TestWarnDiagnosticShowsTheSameWarningOncePerInvocation(t *testing.T) {
	captureLogs(t, charm.WarnLevel)
	warning := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "remote_state_encryption_not_generated", Summary: "Encryption is not generated", Unit: "app", Stack: "dev", UserSafe: true, Severity: pluginv1.Severity_SEVERITY_WARNING}}

	assert.True(t, firstTime("diagnostic", "app", "dev", "remote_state_encryption_not_generated", "Encryption is not generated (Component `app` in stack `dev`)"), "unseen before the first call")
	ResetShownMessages()
	WarnDiagnostic(warning)
	assert.False(t, firstTime("diagnostic", "app", "dev", "remote_state_encryption_not_generated", "Encryption is not generated (Component `app` in stack `dev`)"), "the first call recorded it")
	WarnDiagnostic(warning)
	other := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "remote_state_encryption_not_generated", Summary: "Encryption is not generated", Unit: "app", Stack: "prod", Severity: pluginv1.Severity_SEVERITY_WARNING}}
	WarnDiagnostic(other)
	assert.False(t, firstTime("diagnostic", "app", "prod", "remote_state_encryption_not_generated", "Encryption is not generated (Component `app` in stack `prod`)"), "another stack is its own warning, recorded by its own call")
}

func TestFailureLogsRenderBeforeTheErrorAtTheAtmosLevel(t *testing.T) {
	output := captureLogs(t, charm.DebugLevel)
	detail := &pluginv1.ErrorDetail{Code: "evaluation_error", Summary: "Evaluation failed", Unit: "app", Stack: "dev", Logs: []*pluginv1.LogEntry{logEntry(pluginv1.LogLevel_LOG_LEVEL_DEBUG, "reading include", nil)}}

	err := DetailError(detail, codes.InvalidArgument)

	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, output.String(), "reading include")
	assert.NotContains(t, err.Error(), "reading include", "log entries are never part of the error")
}

func TestHostLogLevelUsesThePluginVocabulary(t *testing.T) {
	for level, want := range map[charm.Level]string{log.TraceLevel: "trace", charm.DebugLevel: "debug", charm.InfoLevel: "info", charm.WarnLevel: "warn", charm.ErrorLevel: "error", charm.FatalLevel + 1: "off"} {
		previous := log.GetLevel()
		log.SetLevel(level)
		assert.Equal(t, want, HostLogLevel())
		log.SetLevel(previous)
	}
}
