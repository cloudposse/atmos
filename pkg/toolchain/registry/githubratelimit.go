package registry

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// GitHub REST API rate-limit headers this classification inspects when
// deciding whether a 403/429 response is worth retrying, and how long GitHub
// is asking the caller to wait.
// See: https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api.
const (
	githubHeaderRetryAfter         = "Retry-After"
	githubHeaderRateLimitRemaining = "X-RateLimit-Remaining"
	githubHeaderRateLimitReset     = "X-RateLimit-Reset"

	// Decimal base used to parse the X-RateLimit-Reset header's integer
	// epoch-second timestamp.
	decimalBase = 10
	// Bit size used to parse the X-RateLimit-Reset header's timestamp.
	bitSize64 = 64
)

// IsRetryableGitHubStatus reports whether a GitHub API response indicates a
// transient failure worth retrying, as opposed to a deterministic client
// error that a retry cannot fix.
//
// GitHub returns 403 for both secondary rate limiting and terminal
// authorization failures (bad token, no access) — only the rate-limit
// headers distinguish them, so a 403 is retryable only when Retry-After or
// X-RateLimit-Remaining: 0 signals a genuine rate limit. A 429 always implies
// rate limiting. A 5xx is always retryable. How long to wait before retrying
// is decided separately by GitHubRetryAfter.
func IsRetryableGitHubStatus(statusCode int, header http.Header) bool {
	defer perf.Track(nil, "registry.IsRetryableGitHubStatus")()

	switch statusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return GitHubSignalsRateLimit(header)
	default:
		return statusCode >= http.StatusInternalServerError
	}
}

// GitHubSignalsRateLimit reports whether a response's headers indicate rate
// limiting rather than a terminal authorization failure.
func GitHubSignalsRateLimit(header http.Header) bool {
	defer perf.Track(nil, "registry.GitHubSignalsRateLimit")()

	return header.Get(githubHeaderRetryAfter) != "" || header.Get(githubHeaderRateLimitRemaining) == "0"
}

// GitHubRetryAfter reports how long GitHub asks the caller to wait before
// retrying: Retry-After (relative seconds) takes precedence over
// X-RateLimit-Reset (a UTC epoch-second reset timestamp), matching GitHub's
// documented guidance. Returns false when neither header is present or
// parseable, in which case the caller falls back to its own retry backoff.
func GitHubRetryAfter(header http.Header) (time.Duration, bool) {
	defer perf.Track(nil, "registry.GitHubRetryAfter")()

	if v := header.Get(githubHeaderRetryAfter); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second, true
		}
	}
	if v := header.Get(githubHeaderRateLimitReset); v != "" {
		if resetUnix, err := strconv.ParseInt(v, decimalBase, bitSize64); err == nil {
			if wait := time.Until(time.Unix(resetUnix, 0)); wait > 0 {
				return wait, true
			}
			return 0, true
		}
	}
	return 0, false
}

// HTTPStatusError carries the status code and response headers of a non-2xx
// HTTP response, so a retry predicate can classify it (e.g. via
// IsRetryableGitHubStatus) without parsing the error message string.
type HTTPStatusError struct {
	StatusCode int
	Header     http.Header
	URL        string
}

func (e *HTTPStatusError) Error() string {
	defer perf.Track(nil, "registry.HTTPStatusError.Error")()

	return fmt.Sprintf("%s: HTTP %d: %s", ErrHTTPRequest, e.StatusCode, e.URL)
}

func (e *HTTPStatusError) Unwrap() error {
	defer perf.Track(nil, "registry.HTTPStatusError.Unwrap")()

	return ErrHTTPRequest
}
