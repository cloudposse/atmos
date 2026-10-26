package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkflowPublishDryRun(t *testing.T) {
	// The extended-step executor is shared; reset it before and after this test.
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "artifact.zip"), []byte("zip"), 0o600))
	workflow := &schema.WorkflowDefinition{WorkingDirectory: dir}
	step := &schema.WorkflowStep{Name: "publish", Type: "publish", Source: "artifact.zip", Target: map[string]any{
		"kind": "aws/s3", "bucket": "bucket", "region": "us-east-1",
	}}
	// An invalid credential file would fail if dry-run tried to initialize AWS.
	err := executeExtendedStep(t.Context(), step, workflow, []string{"AWS_SHARED_CREDENTIALS_FILE=" + filepath.Join(dir, "missing")}, extendedStepOptions{
		DryRun: true, AtmosConfig: &schema.AtmosConfiguration{},
	})
	require.NoError(t, err)
	result, ok := stepExecutorState.GetResult("publish")
	require.True(t, ok)
	assert.True(t, result.Skipped)
	assert.Empty(t, step.Identity)
}
