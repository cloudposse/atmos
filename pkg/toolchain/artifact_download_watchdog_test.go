package toolchain

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/github"
)

type artifactTestTransport struct {
	headerDelay   time.Duration
	readDelays    []time.Duration
	contentLength int64
}

func (tr artifactTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-time.After(tr.headerDelay):
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          &artifactTestBody{ctx: req.Context(), delays: tr.readDelays},
		Request:       req,
		ContentLength: tr.contentLength,
	}, nil
}

// artifactTestBody models a streaming HTTP response whose reads unblock when
// the request is canceled. Virtual time advances these delays without wall-clock waits.
type artifactTestBody struct {
	ctx    context.Context
	delays []time.Duration
}

func (b *artifactTestBody) Read(p []byte) (int, error) {
	if len(b.delays) == 0 {
		return 0, io.EOF
	}
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-time.After(b.delays[0]):
	}
	b.delays = b.delays[1:]
	return copy(p, "x"), nil
}

func (b *artifactTestBody) Close() error { return nil }

func TestDownloadPRArtifact_Inactivity(t *testing.T) {
	callerCause := errors.New("caller stopped download")
	tests := []struct {
		name        string
		headerDelay time.Duration
		readDelays  []time.Duration
		deadline    time.Duration
		cancelAfter time.Duration
		wantErr     error
		wantElapsed time.Duration
	}{
		{
			name:        "progress continues beyond total timeout",
			headerDelay: 750 * time.Millisecond,
			readDelays:  []time.Duration{750 * time.Millisecond, 750 * time.Millisecond, 750 * time.Millisecond},
			wantElapsed: 3 * time.Second,
		},
		{
			name:        "stalled before headers",
			headerDelay: 2 * time.Second,
			wantErr:     errArtifactDownloadStalled,
			wantElapsed: time.Second,
		},
		{
			name:        "headers reset timeout before first body byte",
			headerDelay: 750 * time.Millisecond,
			readDelays:  []time.Duration{2 * time.Second},
			wantErr:     errArtifactDownloadStalled,
			wantElapsed: 1750 * time.Millisecond,
		},
		{
			name:        "stalled after body progress",
			readDelays:  []time.Duration{750 * time.Millisecond, 2 * time.Second},
			wantErr:     errArtifactDownloadStalled,
			wantElapsed: 1750 * time.Millisecond,
		},
		{
			name:        "caller deadline overrides active transfer",
			readDelays:  []time.Duration{750 * time.Millisecond, 750 * time.Millisecond},
			deadline:    1250 * time.Millisecond,
			wantErr:     context.DeadlineExceeded,
			wantElapsed: 1250 * time.Millisecond,
		},
		{
			name:        "caller cancellation preserves cause",
			readDelays:  []time.Duration{750 * time.Millisecond},
			cancelAfter: 500 * time.Millisecond,
			wantErr:     callerCause,
			wantElapsed: 500 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// os.CreateTemp uses different variables across platforms.
			tempDir := t.TempDir()
			for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(name, tempDir)
			}
			original := http.DefaultTransport
			http.DefaultTransport = artifactTestTransport{headerDelay: tt.headerDelay, readDelays: tt.readDelays}
			t.Cleanup(func() { http.DefaultTransport = original })
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancelCause(context.Background())
				defer cancel(nil)
				if tt.deadline > 0 {
					var stop context.CancelFunc
					ctx, stop = context.WithTimeout(ctx, tt.deadline)
					defer stop()
				}
				if tt.cancelAfter > 0 {
					timer := time.AfterFunc(tt.cancelAfter, func() { cancel(callerCause) })
					defer timer.Stop()
				}
				start := time.Now()
				path, err := downloadPRArtifactWithOptions(ctx, "", &github.PRArtifactInfo{
					DownloadURL: "https://example.com/artifact.zip",
				}, artifactDownloadOptions{idleTimeout: time.Second})
				assert.Equal(t, tt.wantElapsed, time.Since(start))
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
					assert.ErrorIs(t, err, ErrPRArtifactDownloadFailed)
					assert.Empty(t, path)
					if errors.Is(tt.wantErr, errArtifactDownloadStalled) {
						assert.EqualError(t, err, "failed to download PR artifact: artifact download timed out: no download progress for 1s")
					} else {
						assert.NotErrorIs(t, err, errArtifactDownloadStalled)
					}
				} else {
					require.NoError(t, err)
					data, readErr := os.ReadFile(path)
					require.NoError(t, readErr)
					assert.Equal(t, strings.Repeat("x", len(tt.readDelays)), string(data))
					require.NoError(t, os.Remove(path))
					assert.NoError(t, ctx.Err(), "completion must not cancel the caller")
				}
				entries, err := os.ReadDir(tempDir)
				require.NoError(t, err)
				assert.Empty(t, entries, "failed downloads must remove their partial files")
			})
		})
	}
}

func TestArtifactDownloadWatchdog_StaleCallbackAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, watchdog := newArtifactDownloadWatchdog(context.Background(), time.Second)
		defer watchdog.stop()
		time.Sleep(750 * time.Millisecond)
		watchdog.progress()
		// Simulate a previously queued callback running after a progress reset.
		watchdog.expire()
		time.Sleep(750 * time.Millisecond)
		require.NoError(t, ctx.Err())
		watchdog.stop()
		time.Sleep(2 * time.Second)
		assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
		assert.NotErrorIs(t, context.Cause(ctx), errArtifactDownloadStalled)
	})
}

func TestArtifactDownloadWatchdog_DefaultTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, watchdog := newArtifactDownloadWatchdog(context.Background(), prArtifactIdleTimeout)
		defer watchdog.stop()
		<-ctx.Done()
		assert.EqualError(t, context.Cause(ctx), "artifact download timed out: no download progress for 5m")
	})
}

func TestDownloadPRArtifact_StallCancelsHTTPBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial artifact"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	// Bound the test even if the inactivity watchdog fails to cancel the request.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path, err := downloadPRArtifactWithOptions(ctx, "", &github.PRArtifactInfo{
		DownloadURL: server.URL,
	}, artifactDownloadOptions{idleTimeout: 100 * time.Millisecond})
	assert.Empty(t, path)
	assert.ErrorIs(t, err, ErrPRArtifactDownloadFailed)
	assert.ErrorIs(t, err, errArtifactDownloadStalled)
	assert.NoError(t, ctx.Err())
}
