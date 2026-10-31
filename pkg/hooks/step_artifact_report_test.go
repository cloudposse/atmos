package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifactwire/wiretest"
)

// reportFiles enables CI reporting for the fixture and points the generic CI provider at files.
func reportFiles(t *testing.T, f *wiretest.Fixture) (summary, outputs string) {
	t.Helper()
	dir := t.TempDir()
	summary, outputs = filepath.Join(dir, "summary.md"), filepath.Join(dir, "output.txt")
	// A real CI runner advertises its own provider, which would receive the output instead of the files.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ATMOS_CI", "true")
	t.Setenv("ATMOS_CI_SUMMARY", summary)
	t.Setenv("ATMOS_CI_OUTPUT", outputs)
	f.Config.CI.Enabled = true
	return summary, outputs
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(content)
}

func TestArtifactStepHookWritesOneReportAndTheHookRendersNothing(t *testing.T) {
	f := wiretest.New(t)
	summary, outputs := reportFiles(t, f)
	require.NoError(t, os.WriteFile(summary, []byte("## Unrelated earlier content\n"), 0o644))
	h := artifactHooks(f, nil)

	require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))

	content := readFile(t, summary)
	assert.Equal(t, 1, strings.Count(content, "## Artifacts"), "one report per hook run")
	assert.Equal(t, 1, strings.Count(content, "### ✅ 1. Publish"), "one section per operation, no second copy")
	assert.True(t, strings.HasPrefix(content, "## Unrelated earlier content\n"), "existing summary content remains")
	assert.Contains(t, readFile(t, outputs), "artifact_ref=team/app:1.0.0")

	// The hook output itself carries no Markdown body, so the hook bridge cannot render a duplicate.
	hook := artifactHooks(f, nil).items["backup"]
	ctx := stepExecContext(&hook)
	ctx.AtmosConfig = f.Config
	out, err := stepEngine{}.Run(ctx)
	require.NoError(t, err)
	assert.Nil(t, out.Artifact, "the hook output has no artifact body to render")
	require.NotNil(t, out.Summary)
	assert.Empty(t, out.Summary.Body)
}

func TestArtifactStepHookFailureStillWritesTheReport(t *testing.T) {
	f := wiretest.New(t)
	summary, _ := reportFiles(t, f)
	f.Recorder.FailPublications(1, errors.New("repository unavailable"))
	h := artifactHooks(f, func(hook *Hook) { hook.OnFailure = OnFailureWarn })

	require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))

	content := readFile(t, summary)
	assert.Contains(t, content, "1 operation: 1 failed.")
	assert.Contains(t, content, "repository unavailable")
}

func TestArtifactStepHookRetryReportsTheFinalAttemptOnce(t *testing.T) {
	f := wiretest.New(t)
	summary, _ := reportFiles(t, f)
	f.Recorder.FailPublications(2, errors.New("temporary registry failure"))
	h := artifactHooks(f, func(hook *Hook) {
		hook.Retry = retryOf(3)
		hook.OnFailure = OnFailureFail
	})

	require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))

	content := readFile(t, summary)
	assert.Contains(t, content, "1 operation: 1 succeeded.")
	assert.Contains(t, content, "**Attempts:** 3")
	assert.NotContains(t, content, "temporary registry failure")
}

func TestArtifactStepHookReportingHonorsTheCIGates(t *testing.T) {
	f := wiretest.New(t)
	summary, outputs := reportFiles(t, f)
	f.Config.CI.Enabled = false
	h := artifactHooks(f, nil)

	require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))

	assert.Empty(t, readFile(t, summary), "ci.enabled=false writes no summary")
	assert.Empty(t, readFile(t, outputs), "ci.enabled=false writes no output variables")
	require.Len(t, f.Recorder.Requests(), 1, "the publication itself still ran")
}
