package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifactwire/wiretest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandArtifactStepWritesOneReport asserts that a custom command owns one artifact
// report: written once after the command's steps, with the retry's final attempt only.
func TestCustomCommandArtifactStepWritesOneReport(t *testing.T) {
	_ = NewTestKit(t)
	f := wiretest.New(t)
	f.Recorder.FailPublications(1, os.ErrDeadlineExceeded)
	dir := t.TempDir()
	summary, outputs := filepath.Join(dir, "summary.md"), filepath.Join(dir, "output.txt")
	// A real CI runner advertises its own provider, which would receive the output instead of the files.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ATMOS_CI", "true")
	t.Setenv("ATMOS_CI_SUMMARY", summary)
	t.Setenv("ATMOS_CI_OUTPUT", outputs)
	f.Config.CI.Enabled = true
	require.NoError(t, os.WriteFile(summary, []byte("## Earlier job step\n"), 0o600))

	fields := f.StepFields()
	fields["type"] = "artifact"
	fields["name"] = "publish"
	fields["retry"] = map[string]any{"max_attempts": 2, "backoff_strategy": "constant", "initial_delay": time.Millisecond.String()}

	runCustomCommand(t, f, "artifact-report", decodeCustomCommandSteps(t, fields))

	content, err := os.ReadFile(summary)
	require.NoError(t, err)
	text := string(content)
	assert.True(t, strings.HasPrefix(text, "## Earlier job step\n"), "existing summary content remains")
	assert.Equal(t, 1, strings.Count(text, "## Artifacts"))
	assert.Equal(t, 1, strings.Count(text, "### ✅ 1. Publish"))
	assert.Contains(t, text, "**Attempts:** 2")
	out, err := os.ReadFile(outputs)
	require.NoError(t, err)
	assert.Contains(t, string(out), "artifact_publish_outcome=created\n")
}

var _ = schema.CIConfig{Enabled: true}
