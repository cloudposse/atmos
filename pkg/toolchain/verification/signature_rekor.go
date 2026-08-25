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

// transportFlakeMarkers are substrings that indicate the network transport
// itself failed before any verification verdict was rendered. A broken
// connection can never represent a signature verdict (tampering, identity
// mismatch, expired cert), so retrying these can never mask a real failure.
var transportFlakeMarkers = []string{
	// Go net/http2 stream errors, e.g.
	// "stream error: stream ID 1; INTERNAL_ERROR; received from peer".
	// Matches all HTTP/2 error codes and both send/recv variants.
	"stream error: stream ID",
	"connection reset by peer",
	"TLS handshake timeout",
	// macOS Security.framework can report this while cosign fetches a remote
	// certificate sidecar. It is a transport/trust-store operation that happens
	// before any signature verdict, so retrying it cannot mask bad evidence.
	"SecPolicyCreateSSL error",
	// A macOS process can be terminated by its security subsystem before
	// Cosign reaches a verification verdict. This is transient execution
	// failure, not evidence that a signature is invalid; a fresh, serialized
	// invocation (see installer.runTrustedVerifier) is safe to retry.
	"signal: killed",
	"i/o timeout",
	"unexpected EOF",
}

// rekorTlogEndpointMarker plus a 5xx status code is also treated as
// transient. We do not match arbitrary 5xx anywhere in the cosign output —
// only those scoped to Rekor's transparency-log retrieve endpoint, which is
// the endpoint historically prone to short-window outages.
const rekorTlogEndpointMarker = "/api/v1/log/entries/retrieve]["

const (
	rekorSearchLogQueryMarker     = "searching log query"
	rekorStreamInternalErrMarker  = "INTERNAL_ERROR"
	rekorStreamReceivedPeerMarker = "received from peer"
)

var rekorTlog5xxStatuses = []string{"500", "502", "503", "504"}

// cosignHTTPFetchMarker is the literal prefix cosign's own HTTP client uses
// when a direct fetch (e.g. of a --certificate/--signature URL pointing at a
// release asset) returns a non-2xx response, such as:
//
//	loading cert: loading URL https://.../tofu_1.12.2_SHA256SUMS.pem: server returned HTTP 504
//
// This is distinct from rekorTlogEndpointMarker above: cosign fetches
// --certificate/--signature URLs itself rather than atmos's own downloader
// pre-fetching them, so the toolchain's asset-download retry logic
// (pkg/toolchain/installer/download.go) never sees this request. The status
// codes mirror download.go's isRetryableHTTPStatus so a transient CDN error
// gets the same treatment whether atmos or cosign made the request.
const cosignHTTPFetchMarker = "server returned HTTP "

var cosignHTTPFetchRetryableStatuses = []string{"429", "500", "502", "503", "504"}

// tufCDNHostMarker is the Sigstore TUF trust-root CDN hostname. Before every
// verification, cosign refreshes its TUF trust-root/timestamp metadata from
// this host (distinct from both the Rekor tlog endpoint and a direct
// --certificate/--signature asset fetch); a transient CDN blip here has
// nothing to do with the signature actually being verified.
const tufCDNHostMarker = "tuf-repo-cdn.sigstore.dev"

// tufCDNRetryableStatusMarkers are the two distinct phrasings cosign's TUF
// client uses to report a non-2xx response from the CDN, observed in CI
// (cloudposse/atmos#2974) as a transient 403 fetching both 16.root.json and
// timestamp.json:
//
//	failed to download https://tuf-repo-cdn.sigstore.dev/16.root.json, http status code: 403
//	failed to download timestamp.json: GET "https://tuf-repo-cdn.sigstore.dev/timestamp.json": unexpected HTTP status 403
//
// Kept as literal message substrings, scoped to tufCDNHostMarker, rather than
// matching a bare "403" anywhere in cosign output: 403 is a terminal
// authorization failure elsewhere in this codebase (see
// pkg/toolchain/registry/githubratelimit.go), so an unscoped match would risk
// silently retrying away a real permissions problem on an unrelated request.
var tufCDNRetryableStatusMarkers = []string{"http status code: 403", "unexpected HTTP status 403"}

// classifySignatureVerificationError joins ErrSignatureRetryable into err when
// signature-verifier output contains a known transient failure marker. Four classes qualify:
// Sigstore Rekor API flakes (rekorFlakeMarkers, tlog 5xx), transport-level
// network errors (transportFlakeMarkers), generic upstream HTTP 5xx/429
// responses cosign surfaces when it directly fetches a URL
// (cosignHTTPFetchMarker), and Sigstore TUF trust-root CDN fetch failures
// (tufCDNHostMarker). Otherwise returns err unchanged.
//
// Never broaden what counts as retryable beyond known upstream-service
// flakes and transport failures — real signature failures (tampering,
// expired cert, identity mismatch, missing signature) must surface
// immediately on the first attempt.
func classifySignatureVerificationError(err error) error {
	defer perf.Track(nil, "verification.classifySignatureVerificationError")()

	if err == nil {
		return nil
	}
	msg := err.Error()
	if isRetryableSignatureError(msg) {
		return errors.Join(errUtils.ErrSignatureRetryable, err)
	}
	return err
}

// isRetryableSignatureError reports whether msg matches one of the known
// transient-failure marker categories: Sigstore Rekor API flakes
// (rekorFlakeMarkers), transport-level network errors (transportFlakeMarkers),
// Rekor tlog 5xx responses, Rekor stream internal errors, generic upstream
// HTTP 5xx/429 responses cosign surfaces when it directly fetches a URL, or a
// Sigstore TUF trust-root CDN fetch failure. Kept as a single predicate so
// classifySignatureVerificationError stays a flat check-then-wrap pipeline.
func isRetryableSignatureError(msg string) bool {
	return matchesAnyMarker(msg, rekorFlakeMarkers) ||
		matchesAnyMarker(msg, transportFlakeMarkers) ||
		hasRekorTlog5xxStatus(msg) ||
		isRekorStreamInternalError(msg) ||
		hasCosignHTTPFetchRetryableStatus(msg) ||
		hasTUFCDNRetryableStatus(msg)
}

// matchesAnyMarker reports whether msg contains any of the given substrings.
func matchesAnyMarker(msg string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// hasRekorTlog5xxStatus reports whether msg is a Rekor tlog retrieve-endpoint
// error carrying one of rekorTlog5xxStatuses.
func hasRekorTlog5xxStatus(msg string) bool {
	if !strings.Contains(msg, rekorTlogEndpointMarker) {
		return false
	}
	for _, status := range rekorTlog5xxStatuses {
		if strings.Contains(msg, rekorTlogEndpointMarker+status+"]") {
			return true
		}
	}
	return false
}

// hasCosignHTTPFetchRetryableStatus reports whether msg is a direct cosign
// HTTP fetch failure carrying one of cosignHTTPFetchRetryableStatuses.
func hasCosignHTTPFetchRetryableStatus(msg string) bool {
	if !strings.Contains(msg, cosignHTTPFetchMarker) {
		return false
	}
	for _, status := range cosignHTTPFetchRetryableStatuses {
		if strings.Contains(msg, cosignHTTPFetchMarker+status) {
			return true
		}
	}
	return false
}

// hasTUFCDNRetryableStatus reports whether msg is a Sigstore TUF trust-root
// CDN fetch failure carrying one of tufCDNRetryableStatusMarkers.
func hasTUFCDNRetryableStatus(msg string) bool {
	if !strings.Contains(msg, tufCDNHostMarker) {
		return false
	}
	return matchesAnyMarker(msg, tufCDNRetryableStatusMarkers)
}

func isRekorStreamInternalError(msg string) bool {
	return strings.Contains(msg, rekorSearchLogQueryMarker) &&
		strings.Contains(msg, rekorStreamInternalErrMarker) &&
		strings.Contains(msg, rekorStreamReceivedPeerMarker)
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
