package cloudformation

import (
	"errors"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// stubConfirmOperation replaces confirmOperation for a single test with a
// function returning the given confirmed/error pair. Auto-restores on cleanup.
func stubConfirmOperation(t *testing.T, confirmed bool, err error) {
	t.Helper()
	original := confirmOperation
	confirmOperation = func(_ string) (bool, error) { return confirmed, err }
	t.Cleanup(func() { confirmOperation = original })
}

// requireConfirmation must never prompt for operations other than apply/delete.
func TestRequireConfirmation_SkipsNonMutatingOperations(t *testing.T) {
	// No confirmOperation stub installed — a call would panic against the real
	// (interactive) implementation, proving these operations never prompt.
	original := confirmOperation
	confirmOperation = func(_ string) (bool, error) {
		t.Fatal("confirmOperation must not be called for a non-mutating operation")
		return false, nil
	}
	t.Cleanup(func() { confirmOperation = original })

	for _, op := range []Operation{OperationDiff, OperationRender, OperationValidate, OperationOutput} {
		require.NoError(t, requireConfirmation(op, "vpc", map[string]any{}))
	}
}

// --auto-approve must skip the prompt entirely for apply and delete.
func TestRequireConfirmation_AutoApproveSkipsPrompt(t *testing.T) {
	original := confirmOperation
	confirmOperation = func(_ string) (bool, error) {
		t.Fatal("confirmOperation must not be called when --auto-approve is set")
		return false, nil
	}
	t.Cleanup(func() { confirmOperation = original })

	flags := map[string]any{"auto-approve": true}
	require.NoError(t, requireConfirmation(OperationApply, "vpc", flags))
	require.NoError(t, requireConfirmation(OperationDelete, "vpc", flags))
}

// changeset-execute must prompt for confirmation, using the distinct "execute
// changeset against" verb, and must skip the prompt when --auto-approve is set.
func TestRequireConfirmation_ChangesetExecutePrompts(t *testing.T) {
	var gotMessage string
	original := confirmOperation
	confirmOperation = func(message string) (bool, error) {
		gotMessage = message
		return true, nil
	}
	t.Cleanup(func() { confirmOperation = original })

	require.NoError(t, requireConfirmation(OperationChangesetExecute, "vpc", map[string]any{}))
	assert.Contains(t, gotMessage, "execute changeset against")
	assert.Contains(t, gotMessage, "vpc")
}

// changeset-execute must respect --auto-approve like apply/delete.
func TestRequireConfirmation_ChangesetExecuteAutoApproveSkipsPrompt(t *testing.T) {
	original := confirmOperation
	confirmOperation = func(_ string) (bool, error) {
		t.Fatal("confirmOperation must not be called when --auto-approve is set")
		return false, nil
	}
	t.Cleanup(func() { confirmOperation = original })

	require.NoError(t, requireConfirmation(OperationChangesetExecute, "vpc", map[string]any{"auto-approve": true}))
}

// changeset-execute must abort with ErrUserAborted when declined, same as
// apply/delete.
func TestRequireConfirmation_ChangesetExecuteDeclinedAborts(t *testing.T) {
	stubConfirmOperation(t, false, nil)
	err := requireConfirmation(OperationChangesetExecute, "vpc", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// changeset-delete must prompt, for consistency with every other mutating
// verb (stackset delete already did; changeset delete previously didn't).
func TestRequireConfirmation_ChangesetDeletePrompts(t *testing.T) {
	var gotMessage string
	original := confirmOperation
	confirmOperation = func(message string) (bool, error) {
		gotMessage = message
		return true, nil
	}
	t.Cleanup(func() { confirmOperation = original })

	require.NoError(t, requireConfirmation(OperationChangesetDelete, "vpc", map[string]any{}))
	assert.Contains(t, gotMessage, "delete changeset for")
	assert.Contains(t, gotMessage, "vpc")
}

// changeset-delete must respect --auto-approve.
func TestRequireConfirmation_ChangesetDeleteAutoApproveSkipsPrompt(t *testing.T) {
	original := confirmOperation
	confirmOperation = func(_ string) (bool, error) {
		t.Fatal("confirmOperation must not be called when --auto-approve is set")
		return false, nil
	}
	t.Cleanup(func() { confirmOperation = original })

	require.NoError(t, requireConfirmation(OperationChangesetDelete, "vpc", map[string]any{"auto-approve": true}))
}

// changeset-delete must abort with ErrUserAborted when declined.
func TestRequireConfirmation_ChangesetDeleteDeclinedAborts(t *testing.T) {
	stubConfirmOperation(t, false, nil)
	err := requireConfirmation(OperationChangesetDelete, "vpc", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// A confirmed prompt allows the operation to proceed.
func TestRequireConfirmation_ConfirmedProceeds(t *testing.T) {
	stubConfirmOperation(t, true, nil)
	require.NoError(t, requireConfirmation(OperationApply, "vpc", map[string]any{}))
}

// A declined prompt (confirmed == false) must abort with ErrUserAborted.
func TestRequireConfirmation_DeclinedAborts(t *testing.T) {
	stubConfirmOperation(t, false, nil)
	err := requireConfirmation(OperationDelete, "vpc", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// stackset create/update/delete must each prompt for confirmation, using
// their own distinct verb, and must skip the prompt when --auto-approve is set.
func TestRequireConfirmation_StackSetOperationsPrompt(t *testing.T) {
	tests := []struct {
		op       Operation
		wantVerb string
	}{
		{OperationStackSetCreate, "create stackset for"},
		{OperationStackSetUpdate, "update stackset for"},
		{OperationStackSetDelete, "delete stackset for"},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			var gotMessage string
			original := confirmOperation
			confirmOperation = func(message string) (bool, error) {
				gotMessage = message
				return true, nil
			}
			t.Cleanup(func() { confirmOperation = original })

			require.NoError(t, requireConfirmation(tt.op, "vpc", map[string]any{}))
			assert.Contains(t, gotMessage, tt.wantVerb)
			assert.Contains(t, gotMessage, "vpc")
		})
	}
}

// stackset create/update/delete must respect --auto-approve like apply/delete.
func TestRequireConfirmation_StackSetOperationsAutoApproveSkipsPrompt(t *testing.T) {
	original := confirmOperation
	confirmOperation = func(_ string) (bool, error) {
		t.Fatal("confirmOperation must not be called when --auto-approve is set")
		return false, nil
	}
	t.Cleanup(func() { confirmOperation = original })

	flags := map[string]any{"auto-approve": true}
	require.NoError(t, requireConfirmation(OperationStackSetCreate, "vpc", flags))
	require.NoError(t, requireConfirmation(OperationStackSetUpdate, "vpc", flags))
	require.NoError(t, requireConfirmation(OperationStackSetDelete, "vpc", flags))
}

// stackset create/update/delete must abort with ErrUserAborted when declined.
func TestRequireConfirmation_StackSetOperationsDeclinedAborts(t *testing.T) {
	stubConfirmOperation(t, false, nil)
	for _, op := range []Operation{OperationStackSetCreate, OperationStackSetUpdate, OperationStackSetDelete} {
		err := requireConfirmation(op, "vpc", map[string]any{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrUserAborted)
	}
}

// An error from the underlying prompt propagates unchanged.
func TestRequireConfirmation_PromptErrorPropagates(t *testing.T) {
	sentinel := errors.New("tty unavailable")
	stubConfirmOperation(t, false, sentinel)
	err := requireConfirmation(OperationApply, "vpc", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// stubRunConfirmField replaces runConfirmField for a single test. Mirrors the
// runForm seam pattern in pkg/auth/profile_fallback.go: a real huh field
// requires an interactive TTY unit tests can't provide.
func stubRunConfirmField(t *testing.T, err error) {
	t.Helper()
	original := runConfirmField
	runConfirmField = func(_ huh.Field) error { return err }
	t.Cleanup(func() { runConfirmField = original })
}

// defaultConfirmOperation: a nil form error with the bound value left at its
// default (false) reports "not confirmed" without error.
func TestDefaultConfirmOperation_DefaultIsNotConfirmed(t *testing.T) {
	stubRunConfirmField(t, nil)
	confirmed, err := defaultConfirmOperation(`apply stack "vpc"?`)
	require.NoError(t, err)
	assert.False(t, confirmed)
}

// defaultConfirmOperation: the user aborting the field (ctrl+c/esc) must
// translate huh.ErrUserAborted to errUtils.ErrUserAborted.
func TestDefaultConfirmOperation_UserAborted(t *testing.T) {
	stubRunConfirmField(t, huh.ErrUserAborted)
	_, err := defaultConfirmOperation(`apply stack "vpc"?`)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// defaultConfirmOperation: any other field error propagates as-is (not
// translated to ErrUserAborted).
func TestDefaultConfirmOperation_OtherError(t *testing.T) {
	sentinel := errors.New("render failed")
	stubRunConfirmField(t, sentinel)
	_, err := defaultConfirmOperation(`apply stack "vpc"?`)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, errUtils.ErrUserAborted)
}
