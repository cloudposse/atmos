//go:build windows

package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWindowsFileLockExecutesCallbacksWithoutNativeLocking(t *testing.T) {
	lock := NewFileLock("unused-cache-path")

	_, ok := lock.(*noopFileLock)
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	methods := []struct {
		name string
		run  func(func() error) error
	}{
		{name: "exclusive", run: lock.WithLock},
		{name: "context", run: func(fn func() error) error { return lock.WithLockContext(ctx, fn) }},
		{name: "shared", run: lock.WithRLock},
	}

	for _, tt := range methods {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			err := tt.run(func() error {
				called = true
				return nil
			})
			require.NoError(t, err)
			require.True(t, called)

			want := errors.New("callback error")
			require.ErrorIs(t, tt.run(func() error { return want }), want)
		})
	}
}
