package exec

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifactwire/wiretest"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// workflowReportFiles enables CI reporting on config and points the generic CI provider at files.
func workflowReportFiles(t *testing.T, config *schema.AtmosConfiguration) (summary, outputs string) {
	t.Helper()
	// Other tests in this package reset the provider registry, which drops the generic provider
	// that its init registered, so register it again.
	ci.Register(generic.NewProvider())
	dir := t.TempDir()
	summary, outputs = filepath.Join(dir, "summary.md"), filepath.Join(dir, "output.txt")
	// A real CI runner advertises its own provider, which would receive the output instead of the files.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ATMOS_CI", "true")
	t.Setenv("ATMOS_CI_SUMMARY", summary)
	t.Setenv("ATMOS_CI_OUTPUT", outputs)
	config.CI.Enabled = true
	return summary, outputs
}

func fileContent(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(content)
}

// distinctOperationIDs gives every logical operation its own ID, as production does. The fixture pins
// one ID for all operations so that independent runs compare byte for byte.
func distinctOperationIDs(t *testing.T) {
	t.Helper()
	count := 0
	restore := stepPkg.UseArtifactIdentity(func() string {
		count++
		return "op-" + strconv.Itoa(count)
	}, func() time.Time { return wiretest.PreparedAt })
	t.Cleanup(restore)
}

func twoPublications(f *wiretest.Fixture, second string) string {
	return `
workflows:
  publish:
    steps:
      - name: first
        type: artifact
        action: publish
        source: "` + yamlPath(f.Source) + `"
        repository: dir
        artifact_name: team/app
        version: "1.0.0"
      - name: second
        type: artifact
        action: publish
        source: "` + yamlPath(f.Source) + `"
        repository: dir
        artifact_name: team/app
        version: "` + second + `"
`
}

func TestWorkflowWritesOneOrderedArtifactReport(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	f := wiretest.New(t)
	distinctOperationIDs(t)
	config, definition, path := artifactWorkflow(t, twoPublications(f, "2.0.0"))
	summary, outputs := workflowReportFiles(t, config)
	require.NoError(t, os.WriteFile(summary, []byte("## Earlier job step\n"), 0o600))

	require.NoError(t, ExecuteWorkflow(*config, "publish", path, definition, false, "", "", ""))

	content := fileContent(t, summary)
	assert.True(t, strings.HasPrefix(content, "## Earlier job step\n"), "existing summary content remains")
	assert.Equal(t, 1, strings.Count(content, "## Artifacts"), "one report for the whole workflow")
	assert.Contains(t, content, "2 operations: 2 succeeded.")
	first := strings.Index(content, "### ✅ 1. Publish")
	second := strings.Index(content, "### ✅ 2. Publish")
	require.Positive(t, first)
	require.Greater(t, second, first)
	assert.Contains(t, content, "**Step:** first")
	assert.Contains(t, content, "**Step:** second")

	out := fileContent(t, outputs)
	assert.Contains(t, out, "artifact_first_outcome=created\n")
	assert.Contains(t, out, "artifact_second_outcome=created\n")
}

func TestWorkflowFailureStillWritesTheArtifactReport(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	f := wiretest.New(t)
	broken := errors.New("repository unavailable")
	f.Recorder.FailPublications(1, broken)
	config, definition, path := artifactWorkflow(t, twoPublications(f, "2.0.0"))
	summary, _ := workflowReportFiles(t, config)

	err := ExecuteWorkflow(*config, "publish", path, definition, false, "", "", "")

	require.ErrorIs(t, err, broken, "the workflow keeps failing")
	content := fileContent(t, summary)
	assert.Contains(t, content, "1 operation: 1 failed.", "the first failure ended the workflow, so one operation was reported")
	assert.Contains(t, content, "repository unavailable")
}

func TestWorkflowRetryReportsTheFinalAttemptOnce(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	f := wiretest.New(t)
	f.Recorder.FailPublications(2, errors.New("temporary registry failure"))
	config, definition, path := artifactWorkflow(t, `
workflows:
  publish:
    steps:
      - name: publish
        type: artifact
        action: publish
        source: "`+yamlPath(f.Source)+`"
        repository: dir
        artifact_name: team/app
        version: "1.0.0"
        retry:
          max_attempts: 3
          backoff_strategy: constant
          initial_delay: 1ms
`)
	summary, _ := workflowReportFiles(t, config)

	require.NoError(t, ExecuteWorkflow(*config, "publish", path, definition, false, "", "", ""))

	content := fileContent(t, summary)
	assert.Equal(t, 1, strings.Count(content, "### ✅ 1. Publish"))
	assert.Contains(t, content, "**Attempts:** 3")
	assert.NotContains(t, content, "temporary registry failure")
}

func TestWorkflowReportingIsOffWhenCIIsDisabled(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	f := wiretest.New(t)
	config, definition, path := artifactWorkflow(t, twoPublications(f, "2.0.0"))
	summary, outputs := workflowReportFiles(t, config)
	config.CI.Enabled = false

	require.NoError(t, ExecuteWorkflow(*config, "publish", path, definition, false, "", "", ""))

	assert.Empty(t, fileContent(t, summary))
	assert.Empty(t, fileContent(t, outputs))
	assert.Len(t, f.Recorder.Requests(), 2, "both publications still ran")
}
