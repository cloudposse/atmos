package artifacts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	_ "github.com/cloudposse/atmos/pkg/ci/providers/generic" // Register the generic CI provider, as the binary does.
)

// ciReportFiles enables CI reporting on the harness and points the generic CI provider at files.
func ciReportFiles(t *testing.T, h *harness) (summary, outputs string) {
	t.Helper()
	dir := t.TempDir()
	summary, outputs = filepath.Join(dir, "summary.md"), filepath.Join(dir, "output.txt")
	// A real CI runner advertises its own provider, which would receive the output instead of the files.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ATMOS_CI", "true")
	t.Setenv("ATMOS_CI_SUMMARY", summary)
	t.Setenv("ATMOS_CI_OUTPUT", outputs)
	h.config.CI.Enabled = true
	return summary, outputs
}

func contentOf(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(content)
}

// The harness service is a strict mock: every report below is built from the result the operation
// returned, so an extra metadata or evidence request would fail the test as an unexpected call.
func TestCommandWritesOneReportAndKeepsStructuredStdoutValid(t *testing.T) {
	h, svc := newHarness(t)
	summary, outputs := ciReportFiles(t, h)
	svc.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(operation(artifact.ActionPublish, artifact.OutcomeCreated), nil)

	got := h.run("publish", "./dist", "--repository=dir", "--name=team/app", "--version=1.0.0", "--format=json")

	require.NoError(t, got.err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(got.stdout), &envelope), "stdout stays a valid JSON document")
	assert.Equal(t, "created", envelope["outcome"])
	assert.NotContains(t, got.stdout, "## Artifacts", "the report never goes to stdout")

	assert.Equal(t, 1, strings.Count(contentOf(t, summary), "### ✅ 1. Publish `team/app:1.0.0`"))
	assert.Contains(t, contentOf(t, outputs), "artifact_ref=team/app:1.0.0\n")
	assert.Contains(t, contentOf(t, outputs), "artifact_resolved_digest="+string(manifestDigest)+"\n")
}

func TestFailedCommandReportsTheFailureAndStaysFailed(t *testing.T) {
	h, svc := newHarness(t)
	summary, _ := ciReportFiles(t, h)
	failure := artifact.OperationResult{OperationID: "op-9", Action: artifact.ActionVerify, Repository: "dir", Requested: "team/app:1.0.0"}
	svc.EXPECT().Verify(gomock.Any(), gomock.Any()).Return(
		&artifact.VerifyResult{Operation: failure, Verification: artifact.VerificationResult{Status: artifact.VerificationUnchecked}},
		errors.Join(errUtils.ErrArtifactVerificationFailed, errors.New("no permitted signature")),
	)

	got := h.run("verify", "team/app:1.0.0", "--repository=dir", "--format=json")

	require.ErrorIs(t, got.err, errUtils.ErrArtifactVerificationFailed, "reporting never turns a verification failure into success")
	content := contentOf(t, summary)
	assert.Contains(t, content, "1 operation: 1 failed.")
	assert.Contains(t, content, "no permitted signature")
	assert.Contains(t, content, "No trust configuration was supplied", "an unchecked signature is never reported as verified")
}

func TestReportingFailurePreservesTheSuccessfulExitStatus(t *testing.T) {
	cases := map[string]func(t *testing.T, h *harness){
		"template failure": func(t *testing.T, h *harness) {
			h.config.CI.Templates.BasePath = t.TempDir()
			h.config.CI.Templates.Artifacts = map[string]string{"summary": "does-not-exist.md"}
		},
		"provider failure": func(t *testing.T, _ *harness) {
			t.Setenv("ATMOS_CI_SUMMARY", filepath.Join(t.TempDir(), "missing-dir", "summary.md"))
			t.Setenv("ATMOS_CI_OUTPUT", filepath.Join(t.TempDir(), "missing-dir", "output.txt"))
		},
	}
	for name, breakReporting := range cases {
		t.Run(name, func(t *testing.T) {
			h, svc := newHarness(t)
			ciReportFiles(t, h)
			breakReporting(t, h)
			svc.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(operation(artifact.ActionPublish, artifact.OutcomeCreated), nil)

			got := h.run("publish", "./dist", "--repository=dir", "--name=team/app", "--version=1.0.0", "--format=json")

			require.NoError(t, got.err, "a reporting error is logged and never changes the exit status")
			assert.Contains(t, got.stdout, `"outcome": "created"`)
		})
	}
}

func TestListQueryIsReportedOnce(t *testing.T) {
	h, svc := newHarness(t)
	summary, _ := ciReportFiles(t, h)
	svc.EXPECT().List(gomock.Any(), gomock.Any()).Return(&artifact.ListResult{Artifacts: []artifact.ListEntry{
		{Name: "team/app", Version: "1.0.0", Kind: "files"},
	}}, nil)

	got := h.run("list", "--format=json")

	require.NoError(t, got.err)
	assert.Equal(t, 1, strings.Count(contentOf(t, summary), "## Artifacts"))
	assert.Contains(t, contentOf(t, summary), "Listed 1 artifact.")
}

func TestDisabledReportingWritesNothing(t *testing.T) {
	h, svc := newHarness(t)
	summary, outputs := ciReportFiles(t, h)
	h.config.CI.Enabled = false
	svc.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(operation(artifact.ActionPublish, artifact.OutcomeCreated), nil)

	got := h.run("publish", "./dist", "--repository=dir", "--name=team/app", "--version=1.0.0", "--format=json")

	require.NoError(t, got.err)
	assert.Empty(t, contentOf(t, summary))
	assert.Empty(t, contentOf(t, outputs))
}
