package container

import (
	"context"
	"strings"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Backoff shape for image-pull retries.
const (
	pullBackoffMultiplier = 2.0 // Exponential growth factor between attempts.
	pullBackoffJitter     = 0.2 // ±20% randomization to avoid thundering-herd retries.
)

// Image-pull retry tuning. Declared as vars (not consts) so tests can shrink the
// delays; production keeps a few attempts with exponential backoff and jitter.
var (
	pullMaxAttempts  = 4
	pullInitialDelay = 2 * time.Second
	pullMaxDelay     = 20 * time.Second
)

// pullRetryConfig is the default retry policy for image pulls, so a transient
// registry failure (Docker Hub rate limits, gateway 5xx, network timeouts)
// doesn't fail the whole container/emulator operation. Image pulls are
// idempotent, so retrying is safe.
func pullRetryConfig() *schema.RetryConfig {
	maxAttempts := pullMaxAttempts
	initialDelay := pullInitialDelay
	maxDelay := pullMaxDelay
	multiplier := pullBackoffMultiplier
	jitter := pullBackoffJitter
	return &schema.RetryConfig{
		MaxAttempts:     &maxAttempts,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &initialDelay,
		MaxDelay:        &maxDelay,
		Multiplier:      &multiplier,
		RandomJitter:    &jitter,
	}
}

// pullWithRetry pulls an image, retrying transient registry/network failures via
// the shared Atmos retry executor. Non-transient errors (image not found,
// unauthorized, invalid reference, etc.) fail fast so a real misconfiguration is
// not masked by retries. Emulators run through this path because they build on
// container.Up.
func pullWithRetry(ctx context.Context, runtime Runtime, image string) error {
	defer perf.Track(nil, "container.pullWithRetry")()

	attempt := 0
	return retry.WithPredicate(ctx, pullRetryConfig(), func() error {
		attempt++
		err := runtime.Pull(ctx, image)
		if err != nil && isTransientPullError(err) {
			log.Debug("transient image pull error; retrying", "image", image, "attempt", attempt, "error", err)
		}
		return err
	}, isTransientPullError)
}

// transientPullErrorSubstrings are lowercased fragments that mark a recoverable
// registry/network failure in `docker pull` / `podman pull` output. The CLI
// surfaces these as plain text in the combined output, so substring matching is
// the practical way to classify them.
var transientPullErrorSubstrings = []string{
	"context deadline exceeded",
	"timeout",
	"timed out",
	"time-out",
	"connection refused",
	"connection reset",
	"no route to host",
	"network is unreachable",
	"temporary failure in name resolution",
	"no such host",
	"server misbehaving",
	"request canceled",
	"unexpected eof",
	"toomanyrequests",
	"too many requests",
	"500 internal server error",
	"502 bad gateway",
	"503 service unavailable",
	"504 gateway",
	"service unavailable",
	"bad gateway",
	"registry is unavailable",
}

// isTransientPullError reports whether an image-pull error looks like a
// recoverable registry/network failure worth retrying.
func isTransientPullError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range transientPullErrorSubstrings {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
