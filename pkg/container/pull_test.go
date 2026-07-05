package container

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// shrinkPullDelays makes the pull retry backoff negligible for the duration of a
// test so retry paths run fast, restoring the production values afterwards.
func shrinkPullDelays(t *testing.T) {
	t.Helper()
	origInitial, origMax := pullInitialDelay, pullMaxDelay
	pullInitialDelay = time.Millisecond
	pullMaxDelay = time.Millisecond
	t.Cleanup(func() {
		pullInitialDelay = origInitial
		pullMaxDelay = origMax
	})
}

func TestIsTransientPullError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not transient", nil, false},
		{"context deadline exceeded", errors.New(`Get "https://registry-1.docker.io/v2/": context deadline exceeded`), true},
		{"client timeout waiting for connection", errors.New("net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)"), true},
		{"429 too many requests", errors.New("toomanyrequests: You have reached your pull rate limit"), true},
		{"502 bad gateway", errors.New("received unexpected HTTP status: 502 Bad Gateway"), true},
		{"503 service unavailable", errors.New("error pulling image: 503 Service Unavailable"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"no such host", errors.New(`dial tcp: lookup registry-1.docker.io: no such host`), true},
		{"i/o timeout", errors.New("dial tcp 1.2.3.4:443: i/o timeout"), true},
		{"not found is terminal", errors.New("manifest for foo/bar:latest not found: manifest unknown"), false},
		{"unauthorized is terminal", errors.New("unauthorized: authentication required"), false},
		{"invalid reference is terminal", errors.New("invalid reference format"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTransientPullError(tt.err))
		})
	}
}

// TestPullWithRetry_RetriesTransientThenSucceeds verifies a transient failure is
// retried and a subsequent success ends the loop.
func TestPullWithRetry_RetriesTransientThenSucceeds(t *testing.T) {
	shrinkPullDelays(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)

	transient := errors.New(`Get "https://registry-1.docker.io/v2/": context deadline exceeded`)
	gomock.InOrder(
		rt.EXPECT().Pull(gomock.Any(), "rancher/k3s:latest").Return(transient),
		rt.EXPECT().Pull(gomock.Any(), "rancher/k3s:latest").Return(transient),
		rt.EXPECT().Pull(gomock.Any(), "rancher/k3s:latest").Return(nil),
	)

	err := pullWithRetry(context.Background(), rt, "rancher/k3s:latest")
	require.NoError(t, err)
}

// TestPullWithRetry_TerminalErrorFailsFast verifies a non-transient error is NOT
// retried (Pull is called exactly once).
func TestPullWithRetry_TerminalErrorFailsFast(t *testing.T) {
	shrinkPullDelays(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)

	terminal := errors.New("manifest unknown: manifest unknown")
	rt.EXPECT().Pull(gomock.Any(), "ghost:latest").Return(terminal).Times(1)

	err := pullWithRetry(context.Background(), rt, "ghost:latest")
	require.Error(t, err)
	assert.ErrorContains(t, err, "manifest unknown")
}

// TestPullWithRetry_ExhaustsAttempts verifies retries stop after pullMaxAttempts
// when the transient error never clears.
func TestPullWithRetry_ExhaustsAttempts(t *testing.T) {
	shrinkPullDelays(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)

	transient := errors.New("502 Bad Gateway")
	rt.EXPECT().Pull(gomock.Any(), "img:latest").Return(transient).Times(pullMaxAttempts)

	err := pullWithRetry(context.Background(), rt, "img:latest")
	require.Error(t, err)
}

// TestPullWithRetry_SuccessFirstTry verifies the happy path makes exactly one call.
func TestPullWithRetry_SuccessFirstTry(t *testing.T) {
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().Pull(gomock.Any(), "img:latest").Return(nil).Times(1)

	require.NoError(t, pullWithRetry(context.Background(), rt, "img:latest"))
}
