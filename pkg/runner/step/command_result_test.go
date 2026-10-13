package step

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
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
