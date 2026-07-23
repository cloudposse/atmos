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

func TestCommandResult(t *testing.T) {
	t.Run("stores masked command output and declared outputs", func(t *testing.T) {
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
		vars := NewVariables()
		called := false
		result, err := ExecuteCommandResult("", func(stdout, stderr io.Writer) error {
			called = true
			_, _ = io.WriteString(stdout, "ignored")
			_, _ = io.WriteString(stderr, "ignored")
			return nil
		})
		require.NoError(t, err)
		require.NoError(t, StoreCommandResult(vars, "", nil, result))
		assert.True(t, called)
		assert.Nil(t, result)
		assert.Empty(t, vars.Steps)
	})

	t.Run("does not store failed commands", func(t *testing.T) {
		vars := NewVariables()
		result, err := ExecuteCommandResult("failed", func(stdout, stderr io.Writer) error {
			_, _ = io.WriteString(stdout, "partial")
			_, _ = io.WriteString(stderr, "failure")
			return errCommandResultRun
		})
		require.ErrorIs(t, err, errCommandResultRun)
		assert.Nil(t, result)
		require.NoError(t, StoreCommandResult(vars, "failed", nil, result))
		_, ok := vars.Steps["failed"]
		assert.False(t, ok)
	})

	t.Run("does not store a result when declared output evaluation fails", func(t *testing.T) {
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
