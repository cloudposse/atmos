package step

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

var errCommandResultRun = errors.New("command result runner failed")

func TestCommandResult(t *testing.T) {
	t.Parallel()

	t.Run("stores masked command output and declared outputs", func(t *testing.T) {
		t.Parallel()
		t.Cleanup(iolib.Reset)
		iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
		secret := "step-output-secret-4d2793"
		iolib.GetContext().Masker().RegisterValue(secret)
		maskedSecret := iolib.MaskString(secret)
		vars := NewVariables()

		result, err := ExecuteCommandResult("produce", func(stdout, stderr io.Writer) error {
			_, writeErr := io.WriteString(stdout, "  "+secret+"\n")
			require.NoError(t, writeErr)
			_, writeErr = io.WriteString(stderr, "warning-"+secret)
			return writeErr
		})
		require.NoError(t, err)
		err = StoreCommandResult(vars, "produce", map[string]string{
			"summary": "{{ .value }}:{{ .metadata.stderr }}",
		}, result)
		require.NoError(t, err)

		stored, ok := vars.Steps["produce"]
		require.True(t, ok)
		assert.Equal(t, maskedSecret, stored.Value)
		assert.Equal(t, "  "+maskedSecret+"\n", stored.Metadata["stdout"])
		assert.Equal(t, "warning-"+maskedSecret, stored.Metadata["stderr"])
		assert.Equal(t, 0, stored.Metadata[exitCodeMetadata])
		assert.Equal(t, maskedSecret+":warning-"+maskedSecret, stored.Outputs["summary"])
		assert.NotContains(t, stored.Metadata["stdout"], secret)
		assert.NotContains(t, stored.Metadata["stderr"], secret)
	})

	t.Run("runs unnamed commands without storing output", func(t *testing.T) {
		t.Parallel()
		for _, runErr := range []error{nil, errCommandResultRun} {
			vars := NewVariables()
			called := false
			result, err := ExecuteCommandResult("", func(stdout, stderr io.Writer) error {
				called = true
				assert.Equal(t, io.Discard, stdout, "unnamed commands must not buffer output by default")
				assert.Equal(t, io.Discard, stderr, "unnamed commands must not buffer output by default")
				_, _ = io.WriteString(stdout, "ignored")
				_, _ = io.WriteString(stderr, "ignored")
				return runErr
			})
			require.ErrorIs(t, err, runErr)
			require.NoError(t, StoreCommandResult(vars, "", nil, result))
			assert.True(t, called)
			assert.Nil(t, result)
			assert.Empty(t, vars.Steps)
		}
	})

	t.Run("retains failed output for retry evaluation", func(t *testing.T) {
		t.Parallel()
		result, err := ExecuteCommandResult("failed", func(stdout, stderr io.Writer) error {
			_, _ = io.WriteString(stdout, "partial")
			_, _ = io.WriteString(stderr, "failure")
			return errCommandResultRun
		})
		require.ErrorIs(t, err, errCommandResultRun)
		require.NotNil(t, result)
		assert.Equal(t, "partial", result.Value)
		assert.Equal(t, "partial", result.Metadata["stdout"])
		assert.Equal(t, "failure", result.Metadata["stderr"])
		assert.Equal(t, 1, result.Metadata[exitCodeMetadata])
	})

	t.Run("does not store a result when declared output evaluation fails", func(t *testing.T) {
		t.Parallel()
		vars := NewVariables()
		result, err := ExecuteCommandResult("invalid-output", func(stdout, stderr io.Writer) error {
			_, _ = io.WriteString(stdout, "complete")
			return nil
		})
		require.NoError(t, err)
		err = StoreCommandResult(vars, "invalid-output", map[string]string{
			"broken": "{{",
		}, result)
		require.Error(t, err)
		_, ok := vars.Steps["invalid-output"]
		assert.False(t, ok)
	})

	t.Run("stores an empty successful result", func(t *testing.T) {
		t.Parallel()
		vars := NewVariables()
		result, err := ExecuteCommandResult("session", func(_, _ io.Writer) error {
			return nil
		})
		require.NoError(t, err)
		require.NoError(t, StoreCommandResult(vars, "session", nil, result))
		stored, ok := vars.Steps["session"]
		require.True(t, ok)
		assert.Empty(t, stored.Value)
		assert.Equal(t, "", stored.Metadata["stdout"])
		assert.Equal(t, "", stored.Metadata["stderr"])
		assert.Equal(t, 0, stored.Metadata[exitCodeMetadata])
	})
}

func TestFailedCommandResultPreservesMaskedOutputAndExitCode(t *testing.T) {
	const secret = "failed-command-secret-8eb741"
	// Environment secret detection and the global I/O context require serial test execution.
	t.Setenv("GITHUB_TOKEN", secret)
	iolib.Reset()
	t.Cleanup(iolib.Reset)
	iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
	masked := iolib.MaskString(secret)
	require.NotEqual(t, secret, masked)
	for _, name := range []string{"named", ""} {
		runErr := errUtils.WithExitCode(errCommandResultRun, 37)
		result, err := ExecuteCommandResult(name, func(stdout, stderr io.Writer) error {
			_, _ = io.WriteString(stdout, "  "+secret+"\n")
			_, _ = io.WriteString(stderr, "warning-"+secret)
			return runErr
		}, WithUnnamedCommandOutput())
		require.ErrorIs(t, err, errCommandResultRun)
		require.NotNil(t, result, "command name %q", name)
		assert.Equal(t, masked, result.Value)
		assert.Equal(t, "  "+masked+"\n", result.Metadata["stdout"])
		assert.Equal(t, "warning-"+masked, result.Metadata["stderr"])
		assert.Equal(t, 37, result.Metadata[exitCodeMetadata])
		attached := WithStepOutput(err, result)
		require.ErrorIs(t, attached, errCommandResultRun)
		assert.Equal(t, runErr.Error(), attached.Error())
		assert.Equal(t, 37, errUtils.GetExitCode(attached))
		var outputErr *StepOutputError
		require.ErrorAs(t, attached, &outputErr)
		assert.NotContains(t, outputErr.Stdout, secret)
		assert.NotContains(t, outputErr.Stderr, secret)
	}
}

func TestUnnamedCommandOutputCaptureDoesNotStoreSuccessfulOutput(t *testing.T) {
	t.Parallel()
	result, err := ExecuteCommandResult("", func(stdout, stderr io.Writer) error {
		_, _ = io.WriteString(stdout, "successful")
		_, _ = io.WriteString(stderr, "warning")
		return nil
	}, WithUnnamedCommandOutput())
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestUnnamedCommandOutputLargeSuccessUsesBoundedMemory(t *testing.T) {
	// Serial: allocation measurements must not overlap parallel package tests.
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	chunk := bytes.Repeat([]byte("x"), 64<<10)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	result, err := ExecuteCommandResult("", func(stdout, stderr io.Writer) error {
		for range 512 {
			if _, err := stdout.Write(chunk); err != nil {
				return err
			}
			if _, err := stderr.Write(chunk); err != nil {
				return err
			}
		}
		return nil
	}, WithUnnamedCommandOutput())
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(4<<20), "64MiB of discarded output must not be buffered in memory")
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestSpooledCommandPreservesCompleteFailedOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createTemp := func(_ string, pattern string) (*os.File, error) { return os.CreateTemp(dir, pattern) }
	payload := "BEGIN " + strings.Repeat("x", 1<<20) + " END"
	runErr := errUtils.WithExitCode(errCommandResultRun, 37)
	result, err := executeSpooledCommand(func(stdout, stderr io.Writer) error {
		_, writeErr := io.WriteString(stdout, payload)
		require.NoError(t, writeErr)
		_, writeErr = io.WriteString(stderr, "complete error stream")
		require.NoError(t, writeErr)
		return runErr
	}, createTemp)
	require.ErrorIs(t, err, errCommandResultRun)
	require.NotNil(t, result)
	assert.Equal(t, payload, result.Metadata["stdout"])
	assert.Equal(t, "complete error stream", result.Metadata["stderr"])
	assert.Equal(t, 37, result.Metadata[exitCodeMetadata])
	predicate, err := RetryPredicate(&schema.RetryConfig{Conditions: []string{`^BEGIN x+ END$`}})
	require.NoError(t, err)
	assert.True(t, predicate(WithStepOutput(runErr, result)), "retry matching must retain the beginning and end of large output")
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestSpooledCommandCreationFailureCleansUp(t *testing.T) {
	t.Parallel()
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			calls := 0
			result, err := executeSpooledCommand(func(_, _ io.Writer) error {
				t.Error("command must not run without both capture files")
				return nil
			}, func(_ string, pattern string) (*os.File, error) {
				calls++
				if calls == failAt {
					return nil, os.ErrPermission
				}
				return os.CreateTemp(dir, pattern)
			})
			require.ErrorIs(t, err, os.ErrPermission)
			assert.Nil(t, result)
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	}
}

func TestSpooledCommandIOFailuresPreserveExitCodeAndCleanup(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"ignored write error", "ignored write error on success", "read error", "cleanup error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var captures []*os.File
			createTemp := func(_ string, pattern string) (*os.File, error) {
				file, err := os.CreateTemp(dir, pattern)
				if err == nil {
					captures = append(captures, file)
					if runtime.GOOS != "windows" {
						info, statErr := file.Stat()
						require.NoError(t, statErr)
						assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
					}
				}
				return file, err
			}
			runErr := errUtils.WithExitCode(errCommandResultRun, 37)
			run := func(stdout, _ io.Writer) error {
				switch mode {
				case "ignored write error", "ignored write error on success":
					require.NoError(t, captures[0].Close())
					_, _ = io.WriteString(stdout, "ignored failure")
					_, _ = io.WriteString(stdout, "later ignored failure")
					if mode == "ignored write error on success" {
						return nil
					}
				case "read error":
					require.NoError(t, captures[0].Close())
				case "cleanup error":
					// Closing first also permits removing the file on Windows.
					require.NoError(t, captures[0].Close())
					require.NoError(t, os.Remove(captures[0].Name()))
					return nil
				case "panic":
					panic("command panic")
				}
				return runErr
			}
			if mode == "panic" {
				assert.Panics(t, func() { _, _ = executeSpooledCommand(run, createTemp) })
			} else {
				result, err := executeSpooledCommand(run, createTemp)
				assert.Nil(t, result)
				require.ErrorIs(t, err, os.ErrClosed)
				if mode != "cleanup error" && mode != "ignored write error on success" {
					require.ErrorIs(t, err, errCommandResultRun)
					assert.Equal(t, 37, errUtils.GetExitCode(err))
				}
				if strings.HasPrefix(mode, "ignored write error") {
					require.ErrorIs(t, err, errUtils.ErrWriteOutput)
				}
			}
			for _, file := range captures {
				_, err := file.WriteString("closed")
				assert.ErrorIs(t, err, os.ErrClosed)
			}
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	}
}
