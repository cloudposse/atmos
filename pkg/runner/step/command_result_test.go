package step

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

var errCommandResultRun = errors.New("command result runner failed")

func TestExecuteAndStoreCommandResult(t *testing.T) {
	t.Run("stores masked command output and declared outputs", func(t *testing.T) {
		iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
		secret := "step-output-secret-4d2793"
		iolib.GetContext().Masker().RegisterValue(secret)
		maskedSecret := iolib.MaskString(secret)
		vars := NewVariables()

		err := ExecuteAndStoreCommandResult(vars, "produce", map[string]string{
			"summary": "{{ .value }}:{{ .metadata.stderr }}",
		}, func(stdout, stderr io.Writer) error {
			_, writeErr := io.WriteString(stdout, "  "+secret+"\n")
			require.NoError(t, writeErr)
			_, writeErr = io.WriteString(stderr, "warning-"+secret)
			return writeErr
		})
		require.NoError(t, err)

		result, ok := vars.Steps["produce"]
		require.True(t, ok)
		assert.Equal(t, maskedSecret, result.Value)
		assert.Equal(t, "  "+maskedSecret+"\n", result.Metadata["stdout"])
		assert.Equal(t, "warning-"+maskedSecret, result.Metadata["stderr"])
		assert.Equal(t, 0, result.Metadata[exitCodeMetadata])
		assert.Equal(t, maskedSecret+":warning-"+maskedSecret, result.Outputs["summary"])
		assert.NotContains(t, result.Metadata["stdout"], secret)
		assert.NotContains(t, result.Metadata["stderr"], secret)
	})

	t.Run("runs unnamed commands without storing output", func(t *testing.T) {
		vars := NewVariables()
		called := false
		err := ExecuteAndStoreCommandResult(vars, "", nil, func(stdout, stderr io.Writer) error {
			called = true
			_, _ = io.WriteString(stdout, "ignored")
			_, _ = io.WriteString(stderr, "ignored")
			return nil
		})
		require.NoError(t, err)
		assert.True(t, called)
		assert.Empty(t, vars.Steps)
	})

	t.Run("does not store failed commands", func(t *testing.T) {
		vars := NewVariables()
		err := ExecuteAndStoreCommandResult(vars, "failed", nil, func(stdout, stderr io.Writer) error {
			_, _ = io.WriteString(stdout, "partial")
			_, _ = io.WriteString(stderr, "failure")
			return errCommandResultRun
		})
		require.ErrorIs(t, err, errCommandResultRun)
		_, ok := vars.Steps["failed"]
		assert.False(t, ok)
	})

	t.Run("does not store a result when declared output evaluation fails", func(t *testing.T) {
		vars := NewVariables()
		err := ExecuteAndStoreCommandResult(vars, "invalid-output", map[string]string{
			"broken": "{{",
		}, func(stdout, stderr io.Writer) error {
			_, _ = io.WriteString(stdout, "complete")
			return nil
		})
		require.Error(t, err)
		_, ok := vars.Steps["invalid-output"]
		assert.False(t, ok)
	})

	t.Run("stores an empty successful result", func(t *testing.T) {
		vars := NewVariables()
		err := ExecuteAndStoreCommandResult(vars, "session", nil, func(_, _ io.Writer) error {
			return nil
		})
		require.NoError(t, err)
		result, ok := vars.Steps["session"]
		require.True(t, ok)
		assert.Empty(t, result.Value)
		assert.Equal(t, "", result.Metadata["stdout"])
		assert.Equal(t, "", result.Metadata["stderr"])
		assert.Equal(t, 0, result.Metadata[exitCodeMetadata])
	})
}
