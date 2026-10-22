package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEmptyConfigRunsOnce(t *testing.T) {
	failure := errors.New("permanent failure")
	for _, execute := range []struct {
		name string
		run  func(context.Context, Func) error
	}{
		{"executor", func(ctx context.Context, fn Func) error { return New(schema.RetryConfig{}).Execute(ctx, fn) }},
		{"helper", func(ctx context.Context, fn Func) error { return Do(ctx, &schema.RetryConfig{}, fn) }},
		{"predicate", func(ctx context.Context, fn Func) error {
			return WithPredicate(ctx, &schema.RetryConfig{}, fn, func(error) bool { return true })
		}},
	} {
		t.Run(execute.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			attempts := 0
			err := execute.run(ctx, func() error {
				attempts++
				if attempts > 1 {
					cancel()
				}
				return failure
			})
			require.ErrorIs(t, err, failure)
			require.Equal(t, 1, attempts)
		})
	}
}

func TestConfiguredUnlimitedRetryStillRetries(t *testing.T) {
	attempts := 0
	policy := schema.RetryConfig{BackoffStrategy: schema.BackoffConstant}
	err := New(policy).Execute(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return errors.New("transient")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}
