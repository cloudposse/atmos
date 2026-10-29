package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/reported"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestFailureDoesNotExposePluginSecrets(t *testing.T) {
	t.Parallel()
	failure := safeFailure(errors.New("issuer response contains a refresh-token-value"))
	require.ErrorIs(t, failure, errUtils.ErrTestsFailed)
	require.NotContains(t, failure.Error(), "refresh-token-value")
	require.ErrorIs(t, safeFailure(context.Canceled), context.Canceled)
	require.ErrorIs(t, safeFailure(context.DeadlineExceeded), context.DeadlineExceeded)
	require.NoError(t, safeFailure(nil))
}

func TestCapturedCommandOutputIsBounded(t *testing.T) {
	t.Parallel()
	var output capturedOutput
	_, err := output.Write([]byte("about\n"))
	require.NoError(t, err)
	_, err = output.Write([]byte(strings.Repeat("x", 2<<20)))
	require.Error(t, err)
	require.Equal(t, "about\n", output.String())
}

func captureReport(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("ATMOS_FORCE_TTY", "false")
	t.Setenv("ATMOS_FORCE_COLOR", "false")
	t.Setenv("NO_COLOR", "1")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	previous := iolib.GetContext()
	streams := NewMockreportStreams(gomock.NewController(t))
	streams.EXPECT().RawOutput().Return(stdout).AnyTimes()
	streams.EXPECT().RawError().Return(stderr).AnyTimes()
	ctx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ctx)
	ui.InitFormatter(ctx)
	t.Cleanup(func() {
		data.InitWriter(previous)
		ui.InitFormatter(previous)
	})
	return stdout, stderr
}

func TestPresentKeepsMachineReportsSeparateFromProgress(t *testing.T) {
	stdout, stderr := captureReport(t)
	report := &Report{Mode: "mock", Results: []Result{
		{Name: "manifest-handler-agreement", Passed: true},
		{Name: "command/about", Detail: "expected about, observed empty output"},
	}}
	err := Present(report, errUtils.ErrTestsFailed, FormatJSON)
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	require.True(t, reported.IsReported(err), "the root must not print a duplicate failure after the JSON report")
	var actual Report
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
	require.Equal(t, report.Mode, actual.Mode)
	require.Equal(t, report.Results, actual.Results)
	require.Empty(t, stderr.String())
}

func TestPresentTextCountsResultsAndPreservesUnreportedErrors(t *testing.T) {
	for _, tt := range []struct {
		name, summary string
		report        *Report
		err           error
		reported      bool
	}{
		{"passed", "All 2 plugin checks passed", &Report{Results: []Result{{Passed: true}, {Passed: true}}}, nil, false},
		{"failed", "1 passed, 1 failed", &Report{Results: []Result{{Passed: true}, {}}}, errUtils.ErrTestsFailed, true},
		{"no match", "", &Report{Results: []Result{{Passed: true}}}, errUtils.ErrPluginTestsNotMatched, false},
		{"setup failure", "", nil, errUtils.ErrPluginInvalidConfig, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureReport(t)
			err := Present(tt.report, tt.err, FormatText)
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.err)
			}
			require.Equal(t, tt.reported, reported.IsReported(err))
			require.Empty(t, stdout.String())
			if tt.summary == "" {
				require.Empty(t, stderr.String())
			} else {
				require.Contains(t, stderr.String(), tt.summary)
			}
		})
	}
}

func TestShowReportsCheckMeaningAndFailureEvidence(t *testing.T) {
	for _, tt := range []struct{ name, label string }{
		{"manifest-handler-agreement", "Plugin contract:"},
		{"oidc-lifecycle", "OIDC lifecycle:"},
		{"authored-tests", "Authored tests:"},
		{"command/about", "Command about:"},
		{"extension/custom/fixture", "Extension custom/fixture:"},
		{"custom-check", "Check custom-check"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureReport(t)
			Show(Result{Name: tt.name, Passed: true})
			require.Contains(t, stderr.String(), tt.label)
			stderr.Reset()
			Show(Result{Name: tt.name, Detail: "expected: success\nobserved: failure\n"})
			require.Contains(t, stderr.String(), tt.label)
			require.Contains(t, stderr.String(), "    expected: success\n    observed: failure")
			require.Empty(t, stdout.String())
		})
	}
}
