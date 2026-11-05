package install

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func captureBatchUI(t *testing.T) *bytes.Buffer {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	ctx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ctx)
	t.Cleanup(ui.Reset)
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))
	return &output
}

func TestBatchReportsEveryFailureInAliasOrderAndNeverClaimsInstalledForRolledBackJobs(t *testing.T) {
	service, source, digest := batchRegistry(t)
	output := captureBatchUI(t)
	requests := map[string]*Request{
		"c-bad":  {Source: strings.Replace(source, "/fixture/", "/missing-c/", 1), Constraint: "*"},
		"a-good": {Source: source, Constraint: "*", PinnedDigest: digest},
		"b-bad":  {Source: strings.Replace(source, "/fixture/", "/missing-b/", 1), Constraint: "*"},
	}
	entries, err := service.PrepareBatch(t.Context(), requests)
	require.Error(t, err)
	assert.Nil(t, entries, "a failed batch must not return partially prepared entries")

	message := err.Error()
	require.Contains(t, message, "missing-b")
	require.Contains(t, message, "missing-c")
	assert.Less(t, strings.Index(message, "missing-b"), strings.Index(message, "missing-c"), "errors.Join order follows alias order")

	text := ansi.Strip(output.String())
	require.Contains(t, text, "✗ b-bad:")
	require.Contains(t, text, "✗ c-bad:")
	assert.Less(t, strings.Index(text, "✗ b-bad:"), strings.Index(text, "✗ c-bad:"), "failure lines follow alias order")
	assert.Equal(t, 1, strings.Count(text, "No plugin changes were written."), text)
	assert.NotContains(t, text, "a-good", "a job that finished before the failure must not be reported as installed")
	assert.NotContains(t, text, "✓")
}

func TestBatchFailureIsReportedOnceAndStaysProgrammaticallyInspectable(t *testing.T) {
	service, source, _ := batchRegistry(t)
	captureBatchUI(t)
	_, err := service.PrepareBatch(t.Context(), map[string]*Request{
		"only-bad": {Source: strings.Replace(source, "/fixture/", "/missing/", 1), Constraint: "*"},
	})
	require.Error(t, err)
	var exit errUtils.ExitCodeError
	require.ErrorAs(t, err, &exit, "the root error handler must not print an error the batch already rendered")
	assert.True(t, exit.Silent)
	assert.Equal(t, 1, errUtils.GetExitCode(err))
	assert.False(t, errors.Is(err, errUtils.ErrPluginVersionNotFound))
}

func TestBatchSuccessPrintsSemanticStatusAfterAllJobsFinish(t *testing.T) {
	service, source, digest := batchRegistry(t)
	output := captureBatchUI(t)
	entries, err := service.PrepareBatch(t.Context(), map[string]*Request{
		"b-two": {Source: source, Constraint: "*", PinnedDigest: digest},
		"a-one": {Source: source, Constraint: "*", PinnedDigest: digest},
	})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	text := ansi.Strip(output.String())
	require.Contains(t, text, "✓ a-one")
	require.Contains(t, text, "✓ b-two")
	assert.Less(t, strings.Index(text, "a-one"), strings.Index(text, "b-two"))
	assert.NotContains(t, text, "No plugin changes were written.")
}
