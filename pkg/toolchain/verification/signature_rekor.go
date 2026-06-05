package verification

import (
	"errors"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Cosign retry tuning. Mirrors the toolchain downloader retry budget
// (downloadRetryMaxAttempts/InitialDelay/MaxDelay) so transient cosign
// failures get the same treatment as transient HTTP failures.
const (
	cosignRetryMaxAttempts  = 5
	cosignRetryInitialDelay = 1 * time.Second
	cosignRetryMaxDelay     = 10 * time.Second
)

// rekorFlakeMarkers are substrings in cosign's combined stderr+stdout that
// indicate a transient Sigstore Rekor transparency-log error (i.e. an
// upstream-service problem, not a real signature failure). Match must stay
// narrow: anything not in this allowlist surfaces immediately so that real
// tampering, expired certs, or identity mismatches are never silently
// retried away.
var rekorFlakeMarkers = []string{
	// Observed: Rekor /api/v1/log/entries/retrieve sometimes returns HTTP 400
	// with a bogus signature-decode message even for valid signatures. The
	// generated go-openapi client surfaces this as "searchLogQueryBadRequest".
	"searchLogQueryBadRequest",
	// The specific Rekor backend decode error that accompanies the 400 above.
	// Including this as a secondary marker so a related Rekor regression that
	// surfaces the same decode message under a different operation name is
	// still classified as transient.
	"Invalid IEEE_P1363 encoded bytes",
}

// rekorTlogEndpointMarker plus a 5xx status code is also treated as
// transient. We do not match arbitrary 5xx anywhere in the cosign output —
// only those scoped to Rekor's transparency-log retrieve endpoint, which is
// the endpoint historically prone to short-window outages.
const rekorTlogEndpointMarker = "/api/v1/log/entries/retrieve]["

var rekorTlog5xxStatuses = []string{"500", "502", "503", "504"}

// classifyCosignError joins ErrSignatureRetryable into err when the cosign
// output contains a known transient Sigstore Rekor failure marker.
// Otherwise returns err unchanged.
//
// Never broaden what counts as retryable beyond known upstream-service
// flakes — real signature failures (tampering, expired cert, identity
// mismatch, missing signature) must surface immediately on the first
// attempt.
func classifyCosignError(err error) error {
	defer perf.Track(nil, "verification.classifyCosignError")()

	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, marker := range rekorFlakeMarkers {
		if strings.Contains(msg, marker) {
			return errors.Join(errUtils.ErrSignatureRetryable, err)
		}
	}
	if strings.Contains(msg, rekorTlogEndpointMarker) {
		for _, status := range rekorTlog5xxStatuses {
			if strings.Contains(msg, rekorTlogEndpointMarker+status+"]") {
				return errors.Join(errUtils.ErrSignatureRetryable, err)
			}
		}
	}
	return err
}

// isRetryableCosignError is the predicate handed to retry.WithPredicate.
func isRetryableCosignError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errUtils.ErrSignatureRetryable)
}

// cosignRetryConfig returns the retry budget used when invoking cosign.
func cosignRetryConfig() *schema.RetryConfig {
	maxAttempts := cosignRetryMaxAttempts
	initialDelay := cosignRetryInitialDelay
	maxDelay := cosignRetryMaxDelay
	return &schema.RetryConfig{
		MaxAttempts:     &maxAttempts,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &initialDelay,
		MaxDelay:        &maxDelay,
	}
}
