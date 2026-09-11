package cmd

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomCommandTestFailureEndsWithReport(t *testing.T) {
	_ = NewTestKit(t)
	originalExit := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = originalExit })
	errUtils.OsExit = func(code int) { panic(code) }
	config := schema.AtmosConfiguration{BasePath: t.TempDir(), Commands: []schema.Command{{
		Name: "check-report", Steps: schema.Tasks{{Name: "checks", Type: "test", Steps: []schema.WorkflowStep{
			{Name: "failure", Type: "shell", Command: "printf failure-detail >&2; exit 1"},
		}}},
	}}}
	require.NoError(t, processCustomCommands(config, config.Commands, RootCmd))
	command, _, err := RootCmd.Find([]string{"check-report"})
	require.NoError(t, err)
	_, stderr := captureStdoutStderr(t, func() { assert.PanicsWithValue(t, 1, func() { command.Run(command, nil) }) })
	assert.Contains(t, stderr, "failure-detail")
	assert.Contains(t, stderr, "1/1 · 0 passed, 1 failed, 0 skipped, 0 canceled")
	assert.NotContains(t, stderr, "# Error")
	assert.NotContains(t, stderr, "Workflow Error")
}
