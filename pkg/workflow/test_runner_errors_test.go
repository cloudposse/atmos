package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTestRunnerPreparationFailures(t *testing.T) {
	for _, tc := range []struct {
		name, setup             string
		skipped, failed, passed int
	}{
		{"suite environment", "env: {BROKEN: '{{'}\n", 2, 0, 0},
		{"group environment", "", 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupEnv := ""
			if tc.name == "group environment" {
				groupEnv = "   env: {BROKEN: '{{'}\n"
			}
			result, output, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
%ssteps:
 - name: group
   type: parallel
%s   steps: [{name: skipped, type: require, files: [must-not-exist]}]
 - {name: after, type: require, dirs: [%q]}
`, tc.setup, groupEnv, t.TempDir()))
			require.Error(t, err)
			assert.Equal(t, tc.skipped, result.Metadata["skipped"])
			assert.Equal(t, tc.failed, result.Metadata["failed"])
			assert.Equal(t, tc.passed, result.Metadata["passed"])
			assert.Contains(t, output, "unclosed action")
			assert.NotContains(t, output, "must-not-exist")
			var displayed *step.TestFailureError
			assert.ErrorAs(t, err, &displayed)
		})
	}
}

func TestTestRunnerGroupConditions(t *testing.T) {
	for _, tc := range []struct {
		name, when, continuation string
		wantError                bool
		skipped, failed          int
	}{
		{"skip group", "never", "never", false, 1, 0},
		{"invalid condition", "!cel flags.missing == 'yes'", "never", true, 1, 0},
		{"invalid continuation", "always", "!cel flags.missing == 'yes'", true, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, output, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
steps:
 - name: group
   type: parallel
   when: %s
   continue: %s
   steps: [{name: broken, type: require, files: [%q]}]
 - {name: after, type: require, dirs: [%q]}
`, tc.when, tc.continuation, t.TempDir()+"/missing-file", t.TempDir()))
			if tc.wantError {
				require.ErrorIs(t, err, schema.ErrInvalidWhenCondition)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.skipped, result.Metadata["skipped"])
			assert.Equal(t, tc.failed, result.Metadata["failed"])
			assert.Equal(t, 1, result.Metadata["passed"])
			if tc.skipped > 0 {
				assert.NotContains(t, output, "missing-file")
			}
		})
	}
}

func TestTestRunnerLeafPreparationFailures(t *testing.T) {
	for _, tc := range []struct{ name, config string }{
		{"condition", "when: !cel flags.missing == 'yes'"},
		{"continuation", "continue: !cel flags.missing == 'yes'"},
		{"directory", "working_directory: '{{'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, _, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
steps:
 - name: broken
   type: require
   files: [%q]
   %s
 - {name: after, type: require, dirs: [%q]}
`, t.TempDir()+"/missing-file", tc.config, t.TempDir()))
			require.Error(t, err)
			assert.Equal(t, 1, result.Metadata["failed"])
			assert.Equal(t, 1, result.Metadata["passed"])
			if tc.name == "continuation" {
				assert.ErrorIs(t, err, errUtils.ErrRequirementsNotMet)
				assert.ErrorIs(t, err, schema.ErrInvalidWhenCondition)
			}
		})
	}
}

func TestTestRunnerDeferredResolutionFailure(t *testing.T) {
	vars := step.NewVariables()
	var output bytes.Buffer
	vars.OutputWriters.Stderr = &output
	resolveErr := errors.New("could not resolve test hook")
	vars.ResolveTestStep = func(s *schema.WorkflowStep, _ *step.Variables) (*schema.WorkflowStep, error) {
		if s.Name == "broken" {
			return nil, resolveErr
		}
		return s, nil
	}
	parent := &schema.WorkflowStep{Type: "test", Steps: []schema.WorkflowStep{
		{Name: "broken", Type: "require", Files: []string{"must-not-exist"}},
		{Name: "after", Title: "Following check", Type: "require", Dirs: []string{t.TempDir()}},
	}}
	result, err := (testBridge{}).RunTest(context.Background(), parent, vars, nil)
	require.ErrorIs(t, err, resolveErr)
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 1, result.Metadata["passed"])
	assert.Contains(t, ansi.Strip(output.String()), "     Tests\n")
	assert.Contains(t, output.String(), "Following check")
	assert.Equal(t, 1, bytes.Count(output.Bytes(), []byte(resolveErr.Error())))
	assert.Equal(t, resolveErr.Error(), vars.Steps["broken"].Error)
	assert.NotContains(t, output.String(), "must-not-exist")
}

func TestTestRunnerGroupFailFastCancelsRemainingCases(t *testing.T) {
	result, _, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
steps:
 - name: group
   type: parallel
   max_concurrency: 1
   fail: {mode: fail_fast}
   steps:
    - {name: first, type: require, files: [%q]}
    - {name: remaining, type: require, dirs: [%q]}
 - {name: after, type: require, dirs: [%q]}
`, t.TempDir()+"/missing-file", t.TempDir(), t.TempDir()))
	require.Error(t, err)
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 1, result.Metadata["passed"])
	assert.Equal(t, 1, result.Metadata["canceled"])
}

func TestTestRunnerDryRunInheritsWorkflowDefaults(t *testing.T) {
	vars := step.NewVariables()
	var output bytes.Buffer
	vars.OutputWriters.Stderr = &output
	vars.SetFlag("enabled", "yes")
	workflow := &schema.WorkflowDefinition{WorkingDirectory: t.TempDir(), Stack: "test-stack"}
	resolved := false
	vars.ResolveTestStep = func(s *schema.WorkflowStep, local *step.Variables) (*schema.WorkflowStep, error) {
		resolved = true
		assert.Equal(t, workflow.WorkingDirectory, s.WorkingDirectory)
		assert.Equal(t, workflow.Stack, s.Stack)
		assert.Equal(t, "value", local.Env["SUITE"])
		assert.True(t, s.DryRun)
		return s, nil
	}
	parent := &schema.WorkflowStep{Type: "test", DryRun: true, Env: map[string]string{"SUITE": "value"}, Steps: []schema.WorkflowStep{
		{Name: "check", Type: "require", Files: []string{"must-not-exist"}, When: schema.MustCondition("!cel flags.enabled == 'yes' && stack == 'test-stack'")},
	}}
	result, err := (testBridge{}).RunTest(context.Background(), parent, vars, workflow)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Metadata["skipped"])
	assert.True(t, resolved, "the inherited condition must allow the dry-run case")
	assert.True(t, vars.Steps["check"].Skipped)
	assert.NotContains(t, vars.Env, "SUITE", "suite environment must not leak into its caller")
	assert.Empty(t, parent.WorkingDirectory)
	assert.Empty(t, parent.Steps[0].Stack, "inherited defaults must not mutate the source tree")
}
