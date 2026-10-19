package exec

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkflowRetryConditionsMatchFailedOutput(t *testing.T) {
	for _, stepType := range []string{"shell", "atmos"} {
		for _, stepName := range []string{"produce", ""} {
			t.Run(stepType+"/name="+stepName, func(t *testing.T) {
				for _, tc := range []struct {
					name       string
					conditions []string
					attempts   string
				}{
					{name: "stdout", conditions: []string{"^attempt-1$"}, attempts: "2"},
					{name: "stderr", conditions: []string{"^warning-1$"}, attempts: "2"},
					{name: "nonmatching", conditions: []string{"unrelated"}, attempts: "1"},
					{name: "error text", conditions: []string{"exit.*1"}, attempts: "2"},
					{name: "unconditional", attempts: "2"},
				} {
					t.Run(tc.name, func(t *testing.T) {
						checkWorkflowRetryOutput(t, stepType, stepName, tc.conditions, tc.attempts)
					})
				}
			})
		}
	}
}

func checkWorkflowRetryOutput(t *testing.T, stepType, stepName string, conditions []string, expectedAttempts string) {
	t.Helper()
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)

	tmpDir := t.TempDir()
	attemptsFile := filepath.Join(tmpDir, "attempts.txt")
	command := workflowRetryOutputHelperCommand(t, attemptsFile)
	if stepType == "atmos" {
		exePath, err := os.Executable()
		require.NoError(t, err)
		installWorkflowAtmosTestBinary(t, exePath)
		command = fmt.Sprintf("-test.run=TestWorkflowStepOutputRetryHelper -- %s", base64.RawURLEncoding.EncodeToString([]byte(attemptsFile)))
	}
	maxAttempts := 2
	initialDelay := time.Millisecond
	workflowDef := &schema.WorkflowDefinition{Steps: []schema.WorkflowStep{{
		Name: stepName, Type: stepType, Command: command,
		Retry: &schema.RetryConfig{
			MaxAttempts: &maxAttempts, InitialDelay: &initialDelay,
			BackoffStrategy: "constant", Conditions: conditions,
		},
	}}}

	err := ExecuteWorkflow(schema.AtmosConfiguration{BasePath: tmpDir}, "retry-output", filepath.Join(tmpDir, "workflow.yaml"), workflowDef, false, "", "", "")
	if expectedAttempts == "1" {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
	}
	attempts, readErr := os.ReadFile(attemptsFile)
	require.NoError(t, readErr)
	assert.Equal(t, expectedAttempts, string(attempts))

	// Unnamed workflow steps receive a generated name before command execution.
	result, stored := stepExecutorState.GetResult(workflowDef.Steps[0].Name)
	if expectedAttempts == "1" {
		assert.False(t, stored, "failed attempts must not be published as step results")
		return
	}
	require.True(t, stored)
	assert.Equal(t, "attempt-2", result.Value)
	assert.Equal(t, "attempt-2", result.Metadata["stdout"])
	assert.Equal(t, "warning-2", result.Metadata["stderr"])
	assert.Equal(t, 0, result.Metadata["exit_code"])
}
