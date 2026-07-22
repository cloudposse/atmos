package exec

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func requireWorkflowOutputHelper(t *testing.T) {
	t.Helper()

	exePath, err := os.Executable()
	require.NoError(t, err)
	cmd := osexec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_STDOUT=produced", "_ATMOS_TEST_STDERR=warning")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	require.NoError(t, cmd.Run())
	assert.Equal(t, "produced", stdout.String())
	assert.Equal(t, "warning", stderr.String())
}

func TestWorkflowLegacyStepOutputAvailableToMarkdown(t *testing.T) {
	if !t.Run("subprocess prerequisite", requireWorkflowOutputHelper) {
		return
	}

	for _, stepType := range []string{"shell", "atmos"} {
		t.Run(stepType, func(t *testing.T) {
			ResetStepExecutorState()
			t.Cleanup(ResetStepExecutorState)

			exePath, err := os.Executable()
			require.NoError(t, err)
			producerCommand := fmt.Sprintf("%q", exePath)
			if stepType == "atmos" {
				installWorkflowAtmosTestBinary(t, exePath)
				producerCommand = "version"
			}

			const expected = "value=produced stdout=produced stderr=warning exit=0 output=produced:warning"
			workflowDef := &schema.WorkflowDefinition{
				Description: "Expose legacy step output to markdown",
				Steps: []schema.WorkflowStep{
					{
						Name:    "produce",
						Type:    stepType,
						Command: producerCommand,
						Env: map[string]string{
							"_ATMOS_TEST_STDOUT": "produced",
							"_ATMOS_TEST_STDERR": "warning",
						},
						Outputs: map[string]string{
							"summary": "{{ .value }}:{{ .metadata.stderr }}",
						},
					},
					{
						Name:    "render",
						Type:    "markdown",
						Content: "value={{ .steps.produce.value }} stdout={{ .steps.produce.metadata.stdout }} stderr={{ .steps.produce.metadata.stderr }} exit={{ .steps.produce.metadata.exit_code }} output={{ .steps.produce.outputs.summary }}",
					},
				},
			}
			tmpDir := t.TempDir()
			err = ExecuteWorkflow(
				schema.AtmosConfiguration{BasePath: tmpDir},
				"test-step-output-"+stepType,
				filepath.Join(tmpDir, "workflow.yaml"),
				workflowDef,
				false,
				"",
				"",
				"",
			)
			require.NoError(t, err)

			producer, ok := stepExecutorState.GetResult("produce")
			require.True(t, ok)
			assert.Equal(t, "produced", producer.Value)
			assert.Equal(t, "produced", producer.Metadata["stdout"])
			assert.Equal(t, "warning", producer.Metadata["stderr"])
			assert.Equal(t, 0, producer.Metadata["exit_code"])
			assert.Equal(t, "produced:warning", producer.Outputs["summary"])

			rendered, ok := stepExecutorState.GetResult("render")
			require.True(t, ok)
			assert.Equal(t, expected, rendered.Value)
		})
	}
}

func TestWorkflowShellStepStoresOnlySuccessfulRetryOutput(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)

	attemptsFile := filepath.Join(t.TempDir(), "attempts.txt")
	maxAttempts := 2
	initialDelay := time.Millisecond
	workflowDef := &schema.WorkflowDefinition{
		Description: "Store only successful retry output",
		Steps: []schema.WorkflowStep{
			{
				Name:    "produce",
				Type:    "shell",
				Command: workflowRetryOutputHelperCommand(t, attemptsFile),
				Retry: &schema.RetryConfig{
					MaxAttempts:     &maxAttempts,
					InitialDelay:    &initialDelay,
					BackoffStrategy: "constant",
				},
			},
			{
				Name:    "render",
				Type:    "markdown",
				Content: "{{ .steps.produce.value }}",
			},
		},
	}
	tmpDir := t.TempDir()
	err := ExecuteWorkflow(
		schema.AtmosConfiguration{BasePath: tmpDir},
		"test-step-output-retry",
		filepath.Join(tmpDir, "workflow.yaml"),
		workflowDef,
		false,
		"",
		"",
		"",
	)
	require.NoError(t, err)

	result, ok := stepExecutorState.GetResult("produce")
	require.True(t, ok)
	assert.Equal(t, "attempt-2", result.Value)
	assert.Equal(t, "attempt-2", result.Metadata["stdout"])
	assert.Equal(t, "warning-2", result.Metadata["stderr"])

	attempts, err := os.ReadFile(attemptsFile)
	require.NoError(t, err)
	assert.Equal(t, "2", strings.TrimSpace(string(attempts)))
}

func TestWorkflowContainerShellOutputAvailableToMarkdown(t *testing.T) {
	for _, mode := range []string{"step", "workflow"} {
		t.Run(mode, func(t *testing.T) {
			ResetStepExecutorState()
			t.Cleanup(ResetStepExecutorState)
			testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
				Name: string(container.TypeDocker),
				Mode: testhelpers.FakeContainerRuntimeWorkflowEnv,
			})

			containerConfig := &schema.WorkflowContainer{
				Image:    "alpine",
				Provider: string(container.TypeDocker),
			}
			producer := schema.WorkflowStep{
				Name:    "produce",
				Type:    "shell",
				Command: "echo produced",
				Outputs: map[string]string{"summary": "{{ .value }}"},
			}
			workflowDef := &schema.WorkflowDefinition{
				Description: "Expose container shell output to markdown",
				Env:         map[string]string{"ATMOS_FAKE_AUTH": "present"},
				Steps: []schema.WorkflowStep{
					producer,
					{Name: "render", Type: "markdown", Content: "container={{ .steps.produce.outputs.summary }}"},
				},
			}
			if mode == "step" {
				workflowDef.Steps[0].Container = containerConfig
			} else {
				workflowDef.Container = containerConfig
			}

			tmpDir := t.TempDir()
			err := ExecuteWorkflow(
				schema.AtmosConfiguration{BasePath: tmpDir},
				"test-container-step-output-"+mode,
				filepath.Join(tmpDir, "workflow.yaml"),
				workflowDef,
				false,
				"",
				"",
				"",
			)
			require.NoError(t, err)

			producerResult, ok := stepExecutorState.GetResult("produce")
			require.True(t, ok)
			assert.Equal(t, "container stdout", producerResult.Value)
			assert.Equal(t, "container stdout\n", producerResult.Metadata["stdout"])
			assert.Equal(t, "container stdout", producerResult.Outputs["summary"])
			rendered, ok := stepExecutorState.GetResult("render")
			require.True(t, ok)
			assert.Equal(t, "container=container stdout", rendered.Value)
		})
	}
}

func installWorkflowAtmosTestBinary(t *testing.T, source string) {
	t.Helper()

	name := "atmos"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binDir := t.TempDir()
	require.NoError(t, copyFile(source, filepath.Join(binDir, name)))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func workflowRetryOutputHelperCommand(t *testing.T, path string) string {
	t.Helper()

	exePath, err := os.Executable()
	require.NoError(t, err)
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	return fmt.Sprintf("%q -test.run=TestWorkflowStepOutputRetryHelper -- %s", exePath, encodedPath)
}

func TestWorkflowStepOutputRetryHelper(t *testing.T) {
	separator := slices.Index(os.Args, "--")
	if separator == -1 {
		return
	}

	args := os.Args[separator+1:]
	require.Len(t, args, 1)
	pathBytes, err := base64.RawURLEncoding.DecodeString(args[0])
	require.NoError(t, err)
	path := string(pathBytes)

	attempt := 1
	if existing, readErr := os.ReadFile(path); readErr == nil {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(existing)))
		require.NoError(t, parseErr)
		attempt = parsed + 1
	}
	require.NoError(t, os.WriteFile(path, []byte(strconv.Itoa(attempt)), 0o600))
	_, _ = fmt.Fprintf(os.Stdout, "attempt-%d", attempt)
	_, _ = fmt.Fprintf(os.Stderr, "warning-%d", attempt)
	if attempt < 2 {
		os.Exit(1)
	}
	os.Exit(0)
}
