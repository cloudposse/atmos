package automation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecutionPolicy(t *testing.T) {
	t.Parallel()
	maxAttempts := 3
	zero := time.Duration(0)
	policy := ExecutionPolicy{Retry: &schema.RetryConfig{MaxAttempts: &maxAttempts, InitialDelay: &zero}}
	attempts := 0
	err := policy.Execute(t.Context(), func(context.Context) error {
		attempts++
		return errUtils.ErrAutomation
	}, func(error) bool { return true })
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	assert.Equal(t, 3, attempts)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = policy.Execute(ctx, func(context.Context) error {
		t.Fatal("a canceled operation must not start")
		return nil
	}, func(error) bool { return true })
	require.ErrorIs(t, err, context.Canceled)
}

func TestExecutionPolicyDeadlineCoversBackoff(t *testing.T) {
	t.Parallel()
	delay := time.Hour
	policy := ExecutionPolicy{Timeout: 10 * time.Millisecond, Retry: &schema.RetryConfig{InitialDelay: &delay}}
	attempts := 0
	err := policy.Execute(t.Context(), func(context.Context) error {
		attempts++
		return errUtils.ErrAutomation
	}, func(error) bool { return true })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, attempts)
}
