package workflow

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestControlExecutorPassesScriptSourceToTheEngine(t *testing.T) {
	engine := registerRecordingEngine(t, "recording-script-source")
	source := filepath.Join(t.TempDir(), "scripts", "main.star")

	for _, dryRun := range []bool{true, false} {
		executor := &ControlCommandExecutor{DryRun: dryRun}
		_, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
			Name: "child", Type: schema.TaskTypeScript, Interpreter: "recording-script-source",
			Script: "ignored", ScriptSource: source,
		}}, ControlChildOutput{})
		require.NoError(t, err)
	}
	// An inline script has no source file.
	_, err := (&ControlCommandExecutor{}).Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name: "inline", Type: schema.TaskTypeScript, Interpreter: "recording-script-source", Script: "ignored",
	}}, ControlChildOutput{})
	require.NoError(t, err)

	calls := engine.calls()
	require.Len(t, calls, 3)
	assert.True(t, calls[0].DryRun, "the dry-run parse call")
	assert.Equal(t, source, calls[0].SourcePath)
	assert.False(t, calls[1].DryRun, "the execute call")
	assert.Equal(t, source, calls[1].SourcePath)
	assert.Empty(t, calls[2].SourcePath)
}
