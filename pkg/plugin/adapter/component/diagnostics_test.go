package component

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"google.golang.org/grpc/codes"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/ui"
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

// When the user asks for Debug or Trace output, the error itself explains the failure with the plugin's
// unsafe detail, masked, instead of pointing at the debug log; at the default level it stays hidden.
func TestUnsafeDetailExplainsTheErrorAtDebug(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())
	atmosio.RegisterSecret("registered-secret-5e1f")
	detail := &pluginv1.ErrorDetail{Code: "evaluation_error", Summary: "Terragrunt could not evaluate the configuration", Detail: "unit file cannot be exported: registered-secret-5e1f is a lock file"}

	for _, level := range []charm.Level{charm.DebugLevel, log.TraceLevel} {
		output := captureLogs(t, level)
		err := DetailError(detail, codes.InvalidArgument)
		text := strings.Join(errUtils.AllDetails(err), "\n")
		assert.Contains(t, text, "unit file cannot be exported: <MASKED> is a lock file")
		assert.NotContains(t, text, "registered-secret-5e1f")
		assert.NotContains(t, strings.Join(errUtils.AllHints(err), "\n"), "ATMOS_LOGS_LEVEL=Debug", "the reason is already shown")
		assert.Contains(t, output.String(), "Plugin diagnostic", "the detail is still logged at Debug")
	}

	captureLogs(t, charm.WarnLevel)
	err := DetailError(detail, codes.InvalidArgument)
	assert.NotContains(t, strings.Join(errUtils.AllDetails(err), "\n"), "lock file")
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "ATMOS_LOGS_LEVEL=Debug")
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
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(previousLevel)
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
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())
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
	warnings := &WarningDeduper{}
	warning := []*pluginv1.LogEntry{logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "state is not encrypted", nil)}
	debug := []*pluginv1.LogEntry{logEntry(pluginv1.LogLevel_LOG_LEVEL_DEBUG, "evaluating again", nil)}

	for range 3 {
		RenderLogs(LogScope{Component: "app", Stack: "dev", Warnings: warnings}, append(slices.Clone(warning), debug...))
	}
	RenderLogs(LogScope{Component: "app", Stack: "prod", Warnings: warnings}, warning)

	assert.Equal(t, 2, strings.Count(output.String(), "state is not encrypted"), "once per component and stack")
	assert.Equal(t, 3, strings.Count(output.String(), "evaluating again"), "debug events are not deduplicated")
}

func TestWarningDeduplicationIsIsolatedPerQuery(t *testing.T) {
	output := captureLogs(t, charm.WarnLevel)
	warning := []*pluginv1.LogEntry{logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "repeated warning", nil)}
	first, second := &WarningDeduper{}, &WarningDeduper{}
	for range 2 {
		RenderLogs(LogScope{Component: "app", Stack: "dev", Warnings: first}, warning)
	}
	RenderLogs(LogScope{Component: "app", Stack: "dev", Warnings: second}, warning)
	assert.Equal(t, 2, strings.Count(output.String(), "repeated warning"))
}

func TestRenderLogsKeepsWarningsThatDifferInTheirFields(t *testing.T) {
	output := captureLogs(t, charm.DebugLevel)
	warnings := &WarningDeduper{}
	scope := LogScope{Provider: "terragrunt", Component: "app", Stack: "dev", Warnings: warnings}
	for range 2 {
		RenderLogs(scope, []*pluginv1.LogEntry{
			logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "dependency has no outputs", map[string]string{"dependency": "network"}),
			logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "dependency has no outputs", map[string]string{"dependency": "database"}),
		})
	}
	RenderLogs(LogScope{Provider: "other", Component: "app", Stack: "dev", Warnings: warnings}, []*pluginv1.LogEntry{
		logEntry(pluginv1.LogLevel_LOG_LEVEL_WARN, "dependency has no outputs", map[string]string{"dependency": "network"}),
	})

	assert.Equal(t, 3, strings.Count(output.String(), "dependency has no outputs"), "each dependency once, and once more for another provider")
	assert.Contains(t, output.String(), "dependency=network")
	assert.Contains(t, output.String(), "dependency=database")
}

func TestWarnDiagnosticShowsTheSameWarningOncePerInvocation(t *testing.T) {
	captureLogs(t, charm.WarnLevel)
	ioCtx, err := atmosio.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.ReinitFormatter)
	var output bytes.Buffer
	restore := atmosio.PushUIWriter(&output)
	t.Cleanup(restore)
	warning := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "remote_state_encryption_not_generated", Summary: "Encryption is not generated", Unit: "app", Stack: "dev", UserSafe: true, Severity: pluginv1.Severity_SEVERITY_WARNING}}
	warnings := &WarningDeduper{}

	WarnDiagnostic(warning, warnings)
	assert.False(t, warnings.firstTime("diagnostic", "app", "dev", "remote_state_encryption_not_generated", "Encryption is not generated (Component `app` in stack `dev`)"), "the first call recorded it")
	WarnDiagnostic(warning, warnings)
	other := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "remote_state_encryption_not_generated", Summary: "Encryption is not generated", Unit: "app", Stack: "prod", Severity: pluginv1.Severity_SEVERITY_WARNING}}
	WarnDiagnostic(other, warnings)
	assert.False(t, warnings.firstTime("diagnostic", "app", "prod", "remote_state_encryption_not_generated", "Encryption is not generated (Component `app` in stack `prod`)"), "another stack is its own warning, recorded by its own call")
	// Styling may split the visible text with ANSI sequences, so count what the user sees.
	visible := ansi.Strip(output.String())
	assert.Equal(t, 2, strings.Count(visible, "Encryption is not generated"), "%q", visible)
	assert.Equal(t, 1, strings.Count(visible, "stack dev"), "%q", visible)
	assert.Equal(t, 1, strings.Count(visible, "stack prod"), "%q", visible)
}

// The configuration session prints preparation warnings once through its deduper, so checking the
// response must not print them as well. The inputs are still registered, so the session's own
// rendering of the same response masks them.
func TestPreparationCheckRegistersInputsAndLeavesWarningsToTheCaller(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())
	captureLogs(t, charm.WarnLevel)
	ioCtx, err := atmosio.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.ReinitFormatter)
	var output bytes.Buffer
	restore := atmosio.PushUIWriter(&output)
	t.Cleanup(restore)

	root := t.TempDir()
	inputs := objectWireValue(t, cty.ObjectVal(map[string]cty.Value{"password": cty.StringVal("warning-secret-9b2e"), "public": cty.StringVal("visible-value")}))
	inputs.SensitivePaths = []*pluginv1.ComponentValuePath{{Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Attribute{Attribute: "password"}}}}}
	warning := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "deprecated_setting", Summary: "Deprecated setting", Detail: "password warning-secret-9b2e is deprecated", UserSafe: true, Unit: "app", Stack: "dev", Severity: pluginv1.Severity_SEVERITY_WARNING}}
	result := &pluginv1.PrepareComponentResponse{Engine: "terraform", WorkingDirectory: root, ModuleSource: "./module", State: fixtureState(root), Inputs: inputs, Diagnostics: []*pluginv1.Diagnostic{warning}}

	// The first and every later preparation print nothing from the check.
	for range 3 {
		require.NoError(t, checkPreparationResponse(result, ""))
	}
	assert.Empty(t, ansi.Strip(output.String()), "the caller prints warnings, once")
	assert.NotContains(t, mask("password warning-secret-9b2e is deprecated"), "warning-secret-9b2e", "the inputs were registered for masking")

	// The caller's deduped print still shows the warning exactly once and masked.
	warnings := &WarningDeduper{}
	for range 3 {
		WarnDiagnostic(warning, warnings)
	}
	visible := ansi.Strip(output.String())
	assert.Equal(t, 1, strings.Count(visible, "Deprecated setting"), "%q", visible)
	assert.NotContains(t, visible, "warning-secret-9b2e")

	// An error diagnostic still fails the check, and still prints nothing by itself.
	result.Diagnostics = append(result.Diagnostics, &pluginv1.Diagnostic{Field: "x", Message: "legacy failure"})
	require.ErrorIs(t, checkPreparationResponse(result, ""), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, checkPreparationResponse(nil, ""), errUtils.ErrPluginProtocol)
}

// Inputs that cannot be decoded register nothing, so no warning of that response is printed, while the
// input decode error takes precedence over any diagnostics it carries.
func TestPreparationWarningsAreNotPrintedWhenTheInputsCannotBeRegistered(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())
	captureLogs(t, charm.WarnLevel)
	ioCtx, err := atmosio.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.ReinitFormatter)
	var output bytes.Buffer
	restore := atmosio.PushUIWriter(&output)
	t.Cleanup(restore)

	root := t.TempDir()
	inputs := objectWireValue(t, cty.ObjectVal(map[string]cty.Value{"pending": cty.UnknownVal(cty.String), "password": cty.StringVal("unregistered-secret-73ea")}))
	inputs.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
	warning := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "deprecated_setting", Summary: "Deprecated setting", Unit: "app", Stack: "dev", Severity: pluginv1.Severity_SEVERITY_WARNING}}
	result := &pluginv1.PrepareComponentResponse{Engine: "terraform", WorkingDirectory: root, ModuleSource: "./module", State: fixtureState(root), Inputs: inputs, Diagnostics: []*pluginv1.Diagnostic{warning}}

	require.ErrorIs(t, checkPreparationResponse(result, ""), errUtils.ErrPluginProtocol)
	assert.Empty(t, ansi.Strip(output.String()), "the caller rejects the inputs before any warning matters")
	assert.Len(t, result.Diagnostics, 1, "the response itself is not modified")

	result.Diagnostics = append(result.Diagnostics, &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "evaluation_error", Summary: "Evaluation failed", Detail: "unregistered-secret-73ea", UserSafe: true, Severity: pluginv1.Severity_SEVERITY_ERROR}})
	err = checkPreparationResponse(result, "")
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	assert.NotContains(t, err.Error(), "unregistered-secret-73ea")
	assert.NotContains(t, strings.Join(errUtils.AllDetails(err), "\n"), "unregistered-secret-73ea")
	assert.Empty(t, ansi.Strip(output.String()))
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

func TestIAMProbeRegistersTokenBeforeDiagnostics(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprint(invalid), func(t *testing.T) {
			atmosio.Reset()
			t.Cleanup(atmosio.Reset)
			require.NoError(t, atmosio.Initialize())
			output := captureLogs(t, charm.DebugLevel)
			token := objectWireValue(t, cty.StringVal("probe-token-9a3b"))
			token.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
			if invalid {
				token.TypeJson = []byte("{")
			}
			result := &pluginv1.PrepareComponentResponse{
				Iam:         &pluginv1.ComponentIam{Role: "role", WebIdentityToken: token},
				Diagnostics: []*pluginv1.Diagnostic{{Detail: &pluginv1.ErrorDetail{Code: "warning", Summary: "Probe warning", Detail: "token probe-token-9a3b", Severity: pluginv1.Severity_SEVERITY_WARNING}}},
			}
			err := checkPreparationResponse(result, "iam-probe")
			if invalid {
				require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
				require.Empty(t, output.String())
			} else {
				require.NoError(t, err)
				require.Contains(t, output.String(), "<MASKED>")
			}
			require.NotContains(t, output.String(), "probe-token-9a3b")
			require.Len(t, result.Diagnostics, 1)
		})
	}
}

// The module source authorization is registered for masking before any diagnostic of the response is
// checked, and one that cannot be decoded takes precedence over the diagnostics.
func TestPreparationCheckRegistersTheSourceAuthorizationBeforeDiagnostics(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())
	root := t.TempDir()
	authorization := wireValue(t, cty.StringVal("Bearer registry-secret-5c1d"))
	authorization.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
	failure := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "evaluation_error", Summary: "Evaluation failed", Detail: "registry rejected Bearer registry-secret-5c1d", UserSafe: true, Severity: pluginv1.Severity_SEVERITY_ERROR}}
	result := &pluginv1.PrepareComponentResponse{
		Engine: "terraform", WorkingDirectory: root, ModuleSource: "https://registry.example/archive.tgz", State: fixtureState(root),
		ModuleSourceAuthorization: authorization, ModuleSourceAuthOrigin: "https://registry.example", Diagnostics: []*pluginv1.Diagnostic{failure},
	}

	require.ErrorIs(t, checkPreparationResponse(result, ""), errUtils.ErrPluginInvalidConfig)
	assert.NotContains(t, mask(failure.Detail.Detail), "registry-secret-5c1d", "the authorization was registered even though the preparation failed")

	unknown := wireValue(t, cty.UnknownVal(cty.String))
	unknown.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
	result.ModuleSourceAuthorization = unknown
	require.ErrorIs(t, checkPreparationResponse(result, ""), errUtils.ErrPluginUnsupportedExtension, "the undecodable authorization outranks the diagnostic error")
}

// A native preparation registers a marked IAM web identity token for masking before its diagnostics are
// checked, and a token that cannot be decoded takes precedence over the diagnostics.
func TestNativePreparationRegistersTheIAMTokenBeforeDiagnostics(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())

	root := t.TempDir()
	token := objectWireValue(t, cty.StringVal("native-iam-token-4e8c"))
	token.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
	failure := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "evaluation_error", Summary: "Evaluation failed", Detail: "role assumption rejected native-iam-token-4e8c", UserSafe: true, Severity: pluginv1.Severity_SEVERITY_ERROR}}
	result := &pluginv1.PrepareComponentResponse{
		Engine: "terraform", WorkingDirectory: root, ModuleSource: "./module", State: fixtureState(root),
		Iam:         &pluginv1.ComponentIam{Role: "role", WebIdentityToken: token},
		Diagnostics: []*pluginv1.Diagnostic{failure},
	}

	require.ErrorIs(t, checkPreparationResponse(result, ""), errUtils.ErrPluginInvalidConfig)
	assert.NotContains(t, mask(failure.Detail.Detail), "native-iam-token-4e8c", "the token was registered even though the preparation failed")

	unknown := objectWireValue(t, cty.UnknownVal(cty.String))
	unknown.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
	result.Iam.WebIdentityToken = unknown
	require.ErrorIs(t, checkPreparationResponse(result, ""), errUtils.ErrPluginUnsupportedExtension, "the undecodable token outranks the diagnostic error")
}

// A preparation without an IAM payload, or with an IAM payload that has no token, stays valid.
func TestNativePreparationAcceptsMissingIAMPayloadOrToken(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())

	root := t.TempDir()
	for name, iam := range map[string]*pluginv1.ComponentIam{
		"nil iam":      nil,
		"no token set": {Role: "role"},
	} {
		t.Run(name, func(t *testing.T) {
			result := &pluginv1.PrepareComponentResponse{Engine: "terraform", WorkingDirectory: root, ModuleSource: "./module", State: fixtureState(root), Iam: iam}
			require.NoError(t, checkPreparationResponse(result, ""))
		})
	}
}
