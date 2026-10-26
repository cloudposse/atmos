package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

func runTestYAML(t *testing.T, source string) (*step.StepResult, string, error) {
	t.Helper()
	var s schema.WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(source), &s))
	var output bytes.Buffer
	vars := step.NewVariables()
	vars.OutputWriters.Stderr = &output
	result, err := step.NewStepExecutorWithVars(vars).Execute(context.Background(), &s)
	return result, output.String(), err
}

func TestTestRunnerContinueAndCapture(t *testing.T) {
	result, output, err := runTestYAML(t, `type: test
name: checks
steps:
 - {name: passing, type: shell, command: 'echo hidden-success'}
 - {name: broken, type: shell, command: 'echo failure-output; echo failure-stderr >&2; exit 1'}
 - {name: after, type: shell, command: 'echo hidden-after'}
`)
	require.Error(t, err)
	assert.Equal(t, 2, result.Metadata["passed"])
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 3, result.Metadata["total"])
	assert.NotContains(t, output, "hidden-success")
	assert.NotContains(t, output, "hidden-after")
	assert.Equal(t, 1, strings.Count(output, "failure-output"))
	assert.Equal(t, 1, strings.Count(output, "failure-stderr"))
	// Forced color is valid in static output, but cursor and other terminal controls are not.
	plain := regexp.MustCompile(`\x1b\[[0-9;:]*m`).ReplaceAllString(output, "")
	assert.NotContains(t, plain, "\x1b")
}

func TestTestRunnerPolicies(t *testing.T) {
	for _, mode := range []string{"wait_all", "fail_fast", "best_effort"} {
		t.Run(mode, func(t *testing.T) {
			result, _, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
fail: {mode: %s}
steps:
 - {name: broken, type: shell, command: 'exit 1'}
 - {name: after, type: shell, command: 'echo after'}
`, mode))
			if mode == "best_effort" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, 1, result.Metadata["failed"])
			if mode == "fail_fast" {
				assert.Equal(t, 1, result.Metadata["canceled"])
			} else {
				assert.Equal(t, 1, result.Metadata["passed"])
			}
		})
	}
}

func TestTestRunnerConditionsAndForgiveness(t *testing.T) {
	result, output, err := runTestYAML(t, `type: test
name: checks
output: all
steps:
 - {name: tolerated, type: shell, command: 'echo tolerated-failure; exit 1', continue: always}
 - {name: skipped, type: shell, command: 'echo never-run; exit 1', when: never}
 - {name: passing, type: shell, command: 'echo debug-success'}
 - {name: message, type: log, content: captured-test-message}
`)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 1, result.Metadata["skipped"])
	assert.Equal(t, 2, result.Metadata["passed"])
	assert.Contains(t, output, "tolerated-failure")
	assert.Contains(t, output, "debug-success")
	assert.Equal(t, 1, strings.Count(output, "captured-test-message"))
	assert.NotContains(t, output, "never-run")
}

func TestTestRunnerParallelHTTPAndMatrix(t *testing.T) {
	var active, peak atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		fmt.Fprint(w, "healthy-secret-body")
	}))
	defer server.Close()
	result, output, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
steps:
 - name: endpoints
   type: parallel
   max_concurrency: 2
   steps:
    - {name: a, type: http, url: %q}
    - {name: b, type: http, url: %q}
 - name: regions
   type: matrix
   max_concurrency: 2
   matrix: {region: [east, west]}
   steps:
    - name: check
      type: shell
      command: 'test "{{ .matrix.region }}" = east'
`, server.URL, server.URL))
	require.Error(t, err)
	assert.Equal(t, 4, result.Metadata["total"])
	assert.Equal(t, 3, result.Metadata["passed"])
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, int64(2), peak.Load())
	assert.Contains(t, output, "region=east")
	assert.Contains(t, output, "region=west")
	assert.NotContains(t, output, "Webhook")
	assert.NotContains(t, output, "healthy-secret-body")
}

func TestTestRunnerDependentSkipped(t *testing.T) {
	result, _, err := runTestYAML(t, `type: test
name: checks
steps:
 - name: fanout
   type: parallel
   steps:
    - {name: a, type: shell, command: "exit 1"}
    - {name: b, type: shell, command: "echo unreachable", needs: [a]}
    - {name: c, type: shell, command: "echo independent"}
`)
	require.Error(t, err)
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 1, result.Metadata["skipped"])
	assert.Equal(t, 1, result.Metadata["passed"])
}

func TestTestRunnerCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	vars := step.NewVariables()
	vars.OutputWriters.Stderr = &output
	result, err := step.NewStepExecutorWithVars(vars).Execute(ctx, &schema.WorkflowStep{Type: "test", Name: "checks", Steps: []schema.WorkflowStep{{Type: "sleep", Name: "waiting", Timeout: "10s"}}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assert.Equal(t, 1, result.Metadata["canceled"])
}

func TestTestRunnerMatrixOutputs(t *testing.T) {
	result, _, err := runTestYAML(t, `type: test
name: checks
steps:
 - name: regions
   type: matrix
   matrix: {region: [east, west]}
   max_concurrency: 2
   steps:
    - name: first
      type: shell
      command: 'printf %s {{ .matrix.region }}'
    - name: second
      type: shell
      command: 'test "{{ .steps.first.value }}" = "{{ .matrix.region }}"'
`)
	require.NoError(t, err)
	assert.Equal(t, 4, result.Metadata["passed"])
}

func TestTestRunnerNestedBestEffort(t *testing.T) {
	result, output, err := runTestYAML(t, `type: test
name: checks
fail: {mode: fail_fast}
steps:
 - name: tolerated
   type: parallel
   fail: {mode: best_effort}
   steps:
    - {name: failing, type: shell, command: 'echo tolerated-error; exit 1'}
 - {name: after, type: shell, command: 'echo hidden-after'}
`)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 1, result.Metadata["passed"])
	assert.Contains(t, output, "tolerated-error")
}

func TestTestRunnerRetryOutput(t *testing.T) {
	result, output, err := runTestYAML(t, fmt.Sprintf(`type: test
name: retries
steps:
 - name: fails-twice
   type: shell
   working_directory: %q
   retry: {max_attempts: 2, initial_delay: 1ms, max_delay: 1ms}
   command: 'if test -f attempted; then echo second-attempt; else echo first-attempt;  : > attempted; fi; exit 1'
`, t.TempDir()))
	require.Error(t, err)
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 1, result.Metadata["total"])
	assert.Contains(t, output, "first-attempt")
	assert.Contains(t, output, "second-attempt")
}

func TestTestRunnerThreshold(t *testing.T) {
	result, _, err := runTestYAML(t, `type: test
name: threshold
fail: {mode: fail_fast, max_failures: 2}
steps:
 - {name: first, type: shell, command: 'exit 1'}
 - {name: second, type: shell, command: 'exit 1'}
 - {name: last, type: shell, command: 'echo not-run'}
`)
	require.Error(t, err)
	assert.Equal(t, 2, result.Metadata["failed"])
	assert.Equal(t, 1, result.Metadata["canceled"])
}

func TestTestRunnerExplicitGroupContinue(t *testing.T) {
	for _, policy := range []string{"never", "always"} {
		t.Run(policy, func(t *testing.T) {
			result, _, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
fail: {mode: fail_fast}
steps:
 - name: group
   type: parallel
   continue: %s
   steps: [{name: bad, type: shell, command: 'exit 1'}]
 - {name: after, type: shell, command: 'echo after'}
`, policy))
			if policy == "never" {
				require.Error(t, err)
				assert.Equal(t, 1, result.Metadata["canceled"])
			} else {
				require.NoError(t, err)
				assert.Equal(t, 1, result.Metadata["passed"])
			}
			assert.Equal(t, 1, result.Metadata["failed"])
		})
	}
}

func TestTestRunnerConditionalDependencySkip(t *testing.T) {
	result, _, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
steps:
 - name: fanout
   type: parallel
   steps:
    - {name: a, type: shell, command: 'exit 1', when: never}
    - {name: b, type: shell, command: 'exit 1', needs: [a]}
    - {name: c, type: require, dirs: [%q]}
`, t.TempDir()))
	require.NoError(t, err)
	assert.Equal(t, 2, result.Metadata["skipped"])
	assert.Equal(t, 1, result.Metadata["passed"])
}

func TestTestRunnerMasksScriptOutput(t *testing.T) {
	secret := "test-runner-sensitive-token-39481"
	iolib.RegisterSecret(secret)
	executable, exeErr := os.Executable()
	require.NoError(t, exeErr)
	result, output, err := runTestYAML(t, fmt.Sprintf(`type: test
name: checks
steps:
 - name: script
   type: script
   working_directory: %q
   interpreter: %q
   script: 'input is supplied to the interpreter'
   env:
     _ATMOS_WORKFLOW_CONTROL_FAKE: '1'
     _ATMOS_WORKFLOW_CONTROL_FAKE_FAIL: '1'
     _ATMOS_WORKFLOW_CONTROL_MARKER: %q
`, t.TempDir(), executable, secret))
	require.Error(t, err)
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.NotContains(t, output, secret)
	assert.Contains(t, output, "<MASKED>")
}

func TestTestRunnerFailFastPreservesEarlierSkips(t *testing.T) {
	result, _, err := runTestYAML(t, `type: test
name: checks
fail: {mode: fail_fast}
steps:
 - name: group
   type: parallel
   max_concurrency: 1
   steps:
    - {name: a, type: shell, command: 'exit 1', when: never}
    - {name: b, type: shell, command: 'exit 1', needs: [a]}
    - {name: z, type: shell, command: 'exit 1'}
`)
	require.Error(t, err)
	assert.Equal(t, 2, result.Metadata["skipped"])
	assert.Equal(t, 1, result.Metadata["failed"])
	assert.Equal(t, 0, result.Metadata["canceled"])
}
