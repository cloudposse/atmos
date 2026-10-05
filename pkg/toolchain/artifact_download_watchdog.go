package toolchain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

var errArtifactDownloadStalled = errors.New("artifact download timed out")

// artifactDownloadWatchdog cancels a request only when progress stops. The mutex
// and timestamp also protect against an expired timer callback racing a reset.
type artifactDownloadWatchdog struct {
	mu           sync.Mutex
	timer        *time.Timer
	cancel       context.CancelCauseFunc
	idleTimeout  time.Duration
	lastProgress time.Time
	stopped      bool
}

func newArtifactDownloadWatchdog(parent context.Context, idleTimeout time.Duration) (context.Context, *artifactDownloadWatchdog) {
	ctx, cancel := context.WithCancelCause(parent)
	w := &artifactDownloadWatchdog{cancel: cancel, idleTimeout: idleTimeout, lastProgress: time.Now()}
	w.mu.Lock()
	w.timer = time.AfterFunc(idleTimeout, w.expire)
	w.mu.Unlock()
	return ctx, w
}

func (w *artifactDownloadWatchdog) expire() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	if remaining := w.idleTimeout - time.Since(w.lastProgress); remaining > 0 {
		w.timer.Reset(remaining)
		return
	}
	w.stopped = true
	duration := w.idleTimeout.String()
	if w.idleTimeout%time.Minute == 0 {
		duration = fmt.Sprintf("%dm", w.idleTimeout/time.Minute)
	}
	w.cancel(fmt.Errorf("%w: no download progress for %s", errArtifactDownloadStalled, duration))
}

func (w *artifactDownloadWatchdog) progress() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.stopped {
		w.lastProgress = time.Now()
		w.timer.Reset(w.idleTimeout)
	}
}

func (w *artifactDownloadWatchdog) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	w.timer.Stop()
	w.cancel(context.Canceled)
}

type artifactProgressReader struct {
	reader     io.Reader
	watchdog   *artifactDownloadWatchdog
	downloaded int64
	total      int64
	report     func(downloaded, total int64)
}

func (r *artifactProgressReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "toolchain.artifactProgressReader.Read")()

	n, err := r.reader.Read(p)
	if n > 0 {
		r.watchdog.progress()
		r.downloaded += int64(n)
		if r.report != nil {
			r.report(r.downloaded, r.total)
		}
	}
	return n, err
}

// artifactDownloadError surfaces the inactivity cause instead of the generic
// context cancellation returned by the HTTP transport when the watchdog fires.
func artifactDownloadError(ctx context.Context, operation string, err error) error {
	cause := context.Cause(ctx)
	if errors.Is(cause, errArtifactDownloadStalled) {
		return fmt.Errorf("%w: %w", ErrPRArtifactDownloadFailed, cause)
	}
	return fmt.Errorf("%w: %s: %w", ErrPRArtifactDownloadFailed, operation, errors.Join(err, cause))
}
