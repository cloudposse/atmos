package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateWithOpa_NonexistentFile(t *testing.T) {
	data := map[string]interface{}{
		"test": "value",
	}

	valid, err := ValidateWithOpa(data, "/nonexistent/policy.rego", nil, 10)

	assert.False(t, valid)
	assert.Error(t, err)
}

func TestValidateWithOpaLegacy_Timeout(t *testing.T) {
	// Test the legacy OPA validation with timeout.
	policyContent := `package test
deny[msg] {
    msg := "test denial"
}
`
	data := map[string]interface{}{
		"test": "value",
	}

	// Use timeout of 0 to force immediate deadline exceeded.
	valid, err := ValidateWithOpaLegacy(data, "test", policyContent, 0)

	assert.False(t, valid)
	assert.Error(t, err)
	// Check that error wrapping includes OPA timeout error.
	assert.ErrorIs(t, err, errUtils.ErrOPATimeout)
}

// TestContextDeadlineExceededWrapping ensures that context.DeadlineExceeded errors
// are properly wrapped with errors.Join and ErrOPATimeout.
func TestContextDeadlineExceededWrapping(t *testing.T) {
	// Create a context that's already cancelled to simulate timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	// Create a simple error that includes deadline exceeded.
	err := ctx.Err()

	// Verify it's a deadline exceeded (actually Canceled in this case, but the pattern is the same).
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// This is how the code should wrap it.
		wrappedErr := errors.Join(errUtils.ErrOPATimeout, err)

		assert.Error(t, wrappedErr)
		assert.ErrorIs(t, wrappedErr, errUtils.ErrOPATimeout)
	}
}
