package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/taskgraph"
	"github.com/cloudposse/atmos/pkg/taskgraph/adapters"
)

func requireCustomCommandOutputHelper(t *testing.T) {
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

func TestCustomCommandLegacyStepOutputAvailableToMarkdown(t *testing.T) {
	if !t.Run("subprocess prerequisite", requireCustomCommandOutputHelper) {
		return
	}

	for _, stepType := range []string{"shell", "atmos"} {
		t.Run(stepType, func(t *testing.T) {
			_ = NewTestKit(t)

			exePath, err := os.Executable()
			require.NoError(t, err)
			resultPath := filepath.Join(t.TempDir(), "result.env")
			producerCommand := "version"
			if stepType == "shell" {
				producerCommand = fmt.Sprintf("%q", exePath)
			}

			const expected = "value=produced stdout=produced stderr=warning exit=0 output=produced:warning"
			commandConfig := schema.Command{
				Name:        "test-step-output-" + stepType,
				Description: "Expose legacy step output to markdown",
				Steps: schema.Tasks{
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
					{
						Name:    "persist",
						Type:    "shell",
						Command: fmt.Sprintf("%q", exePath),
						Output:  "none",
						Env: map[string]string{
							"_ATMOS_TEST_DUMP_ENV": resultPath,
							"CAPTURED_RESULT":      "{{ .steps.render.value }}",
						},
					},
				},
			}
			atmosConfig := schema.AtmosConfiguration{
				BasePath: t.TempDir(),
				Commands: []schema.Command{commandConfig},
			}

			parentCmd := &cobra.Command{Use: "atmos"}
			err = processCustomCommands(atmosConfig, atmosConfig.Commands, parentCmd)
			require.NoError(t, err)

			originalOsExit := errUtils.OsExit
			t.Cleanup(func() { errUtils.OsExit = originalOsExit })
			errUtils.OsExit = func(code int) {
				panic(fmt.Sprintf("unexpected custom command exit %d", code))
			}

			customCmd := findSubcommand(parentCmd, commandConfig.Name)
			require.NotNil(t, customCmd)
			require.NotPanics(t, func() { customCmd.Run(customCmd, nil) })

			envContent, err := os.ReadFile(resultPath)
			require.NoError(t, err)
			assert.Equal(t, expected, extractEnvVar(string(envContent), "CAPTURED_RESULT"))
		})
	}
}

func TestCustomCommandMasksLiveAndStoredOutput(t *testing.T) {
	if !t.Run("subprocess prerequisite", requireCustomCommandOutputHelper) {
		return
	}

	for _, stepType := range []string{"shell", "atmos"} {
		t.Run(stepType, func(t *testing.T) {
			_ = NewTestKit(t)
			t.Cleanup(iolib.Reset)

			iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
			secret := "step-live-secret-8f14a2"
			iolib.GetContext().Masker().RegisterValue(secret)
			maskedSecret := iolib.MaskString(secret)
			exePath, err := os.Executable()
			require.NoError(t, err)
			stepCommand := fmt.Sprintf("%q", exePath)
			if stepType == "atmos" {
				stepCommand = "version"
			}
			resultPath := filepath.Join(t.TempDir(), "result.env")
			commandConfig := schema.Command{
				Name:        "test-masked-step-output-" + stepType,
				Description: "Mask live and stored command output",
				Steps: schema.Tasks{
					{
						Name:    "produce",
						Type:    stepType,
						Command: stepCommand,
						Env: map[string]string{
							"_ATMOS_TEST_STDOUT": secret,
							"_ATMOS_TEST_STDERR": secret,
						},
					},
					{
						Type:    stepType,
						Command: stepCommand,
						Output:  "none",
						Env: map[string]string{
							"_ATMOS_TEST_DUMP_ENV": resultPath,
							"CAPTURED_RESULT":      "{{ .steps.produce.value }}|{{ .steps.produce.metadata.stderr }}",
						},
					},
				},
			}
			atmosConfig := schema.AtmosConfiguration{BasePath: t.TempDir(), Commands: []schema.Command{commandConfig}}
			parentCmd := &cobra.Command{Use: "atmos"}
			require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, parentCmd))

			originalOsExit := errUtils.OsExit
			t.Cleanup(func() { errUtils.OsExit = originalOsExit })
			errUtils.OsExit = func(code int) { panic(fmt.Sprintf("unexpected custom command exit %d", code)) }
			customCmd := findSubcommand(parentCmd, commandConfig.Name)
			require.NotNil(t, customCmd)

			stdout, stderr := captureStdoutStderr(t, func() {
				require.NotPanics(t, func() { customCmd.Run(customCmd, nil) })
			})
			assert.NotContains(t, stdout, secret)
			assert.NotContains(t, stderr, secret)
			assert.Equal(t, 1, strings.Count(stdout, maskedSecret))
			assert.Equal(t, 1, strings.Count(stderr, maskedSecret))

			envContent, err := os.ReadFile(resultPath)
			require.NoError(t, err)
			assert.Equal(t, maskedSecret+"|"+maskedSecret, extractEnvVar(string(envContent), "CAPTURED_RESULT"))
		})
	}
}

func TestCustomCommandOutputNoneCapturesWithoutStreaming(t *testing.T) {
	if !t.Run("subprocess prerequisite", requireCustomCommandOutputHelper) {
		return
	}

	for _, stepType := range []string{"shell", "atmos"} {
		t.Run(stepType, func(t *testing.T) {
			_ = NewTestKit(t)

			exePath, err := os.Executable()
			require.NoError(t, err)
			stepCommand := fmt.Sprintf("%q", exePath)
			if stepType == "atmos" {
				stepCommand = "version"
			}
			resultPath := filepath.Join(t.TempDir(), "result.env")
			commandConfig := schema.Command{
				Name:        "test-cross-platform-output-none-" + stepType,
				Description: "Capture a suppressed command result",
				Steps: schema.Tasks{
					{
						Name:    "produce",
						Type:    stepType,
						Command: stepCommand,
						Output:  "none",
						Env: map[string]string{
							"_ATMOS_TEST_STDOUT": "produced",
							"_ATMOS_TEST_STDERR": "warning",
						},
					},
					{
						Type:    stepType,
						Command: stepCommand,
						Output:  "none",
						Env: map[string]string{
							"_ATMOS_TEST_DUMP_ENV": resultPath,
							"CAPTURED_RESULT":      "{{ .steps.produce.value }}|{{ .steps.produce.metadata.stdout }}|{{ .steps.produce.metadata.stderr }}",
						},
					},
				},
			}
			atmosConfig := schema.AtmosConfiguration{BasePath: t.TempDir(), Commands: []schema.Command{commandConfig}}
			parentCmd := &cobra.Command{Use: "atmos"}
			require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, parentCmd))

			originalOsExit := errUtils.OsExit
			t.Cleanup(func() { errUtils.OsExit = originalOsExit })
			errUtils.OsExit = func(code int) { panic(fmt.Sprintf("unexpected custom command exit %d", code)) }
			customCmd := findSubcommand(parentCmd, commandConfig.Name)
			require.NotNil(t, customCmd)

			stdout, stderr := captureStdoutStderr(t, func() {
				require.NotPanics(t, func() { customCmd.Run(customCmd, nil) })
			})
			assert.NotContains(t, stdout, "produced")
			assert.NotContains(t, stdout, "warning")
			assert.NotContains(t, stderr, "produced")
			assert.NotContains(t, stderr, "warning")

			envContent, err := os.ReadFile(resultPath)
			require.NoError(t, err)
			assert.Equal(t, "produced|produced|warning", extractEnvVar(string(envContent), "CAPTURED_RESULT"))
		})
	}
}

func TestCustomCommandOutputEvaluationFailureDoesNotRetryCommand(t *testing.T) {
	for _, stepType := range []string{"shell", "atmos"} {
		t.Run(stepType, func(t *testing.T) {
			_ = NewTestKit(t)

			attemptsFile := filepath.Join(t.TempDir(), "attempts.txt")
			producerCommand := customCommandAttemptHelperCommand(t, attemptsFile)
			if stepType == "atmos" {
				producerCommand = customCommandAtmosAttemptHelperArgs(attemptsFile)
			}
			maxAttempts := 3
			initialDelay := time.Millisecond
			commandConfig := &schema.Command{
				Name:        "test-output-evaluation-no-retry-" + stepType,
				Description: "Do not retry successful commands when output evaluation fails",
				Steps: schema.Tasks{
					{
						Name:    "produce",
						Type:    stepType,
						Command: producerCommand,
						Outputs: map[string]string{"broken": "{{"},
						Retry: &schema.RetryConfig{
							MaxAttempts:     &maxAttempts,
							InitialDelay:    &initialDelay,
							BackoffStrategy: "constant",
						},
					},
				},
			}

			originalOsExit := errUtils.OsExit
			t.Cleanup(func() { errUtils.OsExit = originalOsExit })
			var exitCode int
			errUtils.OsExit = func(code int) {
				exitCode = code
				panic(code)
			}

			assert.Panics(t, func() {
				executeCustomCommand(
					schema.AtmosConfiguration{BasePath: t.TempDir()},
					&cobra.Command{Use: commandConfig.Name, Annotations: map[string]string{}},
					nil,
					&cobra.Command{Use: "atmos"},
					commandConfig,
				)
			})
			assert.Equal(t, 1, exitCode)

			attempts, err := os.ReadFile(attemptsFile)
			require.NoError(t, err)
			assert.Equal(t, "1", strings.TrimSpace(string(attempts)))
		})
	}
}

// TestCustomCommandRetryConditionsMatchFailedOutput exercises real failing subprocesses.
func TestCustomCommandRetryConditionsMatchFailedOutput(t *testing.T) {
	for _, stepType := range []string{"shell", "atmos"} {
		for _, name := range []string{"produce", ""} {
			for _, tc := range []struct {
				label    string
				pattern  string
				attempts string
			}{
				{"stdout", "^attempt-1$", "2"},
				{"stderr", "^warning-1$", "2"},
				{"unmatched", "^not-emitted$", "1"},
			} {
				t.Run(stepType+"/"+name+"/"+tc.label, func(t *testing.T) {
					_ = NewTestKit(t)
					attemptsFile := filepath.Join(t.TempDir(), "attempts.txt")
					command := customCommandRetryHelperCommand(t, attemptsFile)
					if stepType == "atmos" {
						command = "-test.run=TestCustomCommandIntegrationRetryHelper -- " + base64.RawURLEncoding.EncodeToString([]byte(attemptsFile))
					}
					maxAttempts := 3
					initialDelay := time.Millisecond
					config := schema.Command{Name: "retry-output", Steps: schema.Tasks{{
						Name: name, Type: stepType, Command: command,
						Retry: &schema.RetryConfig{
							MaxAttempts: &maxAttempts, InitialDelay: &initialDelay,
							BackoffStrategy: "constant", Conditions: []string{tc.pattern},
						},
					}}}
					parent := &cobra.Command{Use: "atmos"}
					require.NoError(t, processCustomCommands(schema.AtmosConfiguration{BasePath: t.TempDir()}, []schema.Command{config}, parent))
					// Dependency dispatch returns the real command error without terminating this test process.
					runner := adapters.CustomCommandRunner(parent, func(*cobra.Command) bool { return true })
					err := runner(t.Context(), taskgraph.Ref{Name: config.Name})
					if tc.attempts == "2" {
						require.NoError(t, err, "a matching failed attempt must retry and recover")
					} else {
						require.Error(t, err)
						assert.Equal(t, 1, errUtils.GetExitCode(err))
						var outputErr *stepPkg.StepOutputError
						require.ErrorAs(t, err, &outputErr)
						assert.Equal(t, "attempt-1", outputErr.Stdout)
						assert.Equal(t, "warning-1", outputErr.Stderr)
						var exitErr errUtils.ExitCodeError
						require.ErrorAs(t, err, &exitErr, "the real subprocess exit error must survive wrapping")
						assert.Equal(t, 1, exitErr.Code)
					}
					attempts, readErr := os.ReadFile(attemptsFile)
					require.NoError(t, readErr)
					assert.Equal(t, tc.attempts, string(attempts))
				})
			}
		}
	}
}
