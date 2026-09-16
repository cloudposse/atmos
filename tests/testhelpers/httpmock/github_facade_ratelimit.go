package httpmock

import (
	"net/http"
	"strconv"
	"time"
)

// Default rate-limit values reported when SetRateLimit has never been called: a full,
// unthrottled 5,000/hour authenticated budget, matching GitHub's real default so tests that
// never touch rate limiting see byte-identical headers to today.
const (
	defaultRateLimitLimit     = 5000
	defaultRateLimitRemaining = 5000
)

// rateLimitState holds the mock's current core rate-limit budget. It is stamped as
// X-RateLimit-Limit/Remaining/Reset headers on every API response, and served as JSON from
// GET /api/v3/rate_limit. Both pkg/github.CheckRateLimit (client.RateLimit.Get) and
// pkg/downloader's isGitHubHTTPURL pre-check (github.WaitForRateLimit) read one of these two
// surfaces.
type rateLimitState struct {
	limit     int
	remaining int
	reset     time.Time
}

// SetRateLimit configures the core rate-limit budget the mock reports: `remaining` requests
// left out of a 5,000 budget, resetting at `reset`. Call with remaining=0 and a past `reset`
// to simulate an already-recovered exhausted limit (WaitForRateLimit returns immediately);
// call with remaining=0 and a future `reset` to simulate a still-active primary limit.
func (m *GitHubMockServer) SetRateLimit(remaining int, reset time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimit = &rateLimitState{limit: defaultRateLimitLimit, remaining: remaining, reset: reset}
}

// defaultRateLimitState returns a full, unthrottled 5,000/hour authenticated budget, matching
// GitHub's real default. Computed once at server construction -- rather than freshly on every
// call -- so a default (never-SetRateLimit'd) server always reports the same reset instant:
// recomputing time.Now().Add(time.Hour) separately for the X-RateLimit-Reset header and the
// JSON body's resources.core.reset let the two straddle a second boundary and disagree.
func defaultRateLimitState() *rateLimitState {
	return &rateLimitState{limit: defaultRateLimitLimit, remaining: defaultRateLimitRemaining, reset: time.Now().Add(time.Hour)}
}

// currentRateLimit returns the mock's configured rate-limit state, which is always set --
// either by SetRateLimit or, by default, at server construction (see defaultRateLimitState).
func (m *GitHubMockServer) currentRateLimit() rateLimitState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return *m.rateLimit
}

// Base for formatting the X-RateLimit-Reset Unix-seconds timestamp.
const decimalBase = 10

// stampRateLimitHeaders sets X-RateLimit-Limit/Remaining/Reset on every API response,
// mirroring what the real GitHub REST API does on every request, not just GET /rate_limit.
func (m *GitHubMockServer) stampRateLimitHeaders(w http.ResponseWriter) {
	state := m.currentRateLimit()
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(state.limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(state.remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(state.reset.Unix(), decimalBase))
}

// rateLimitJSON is the JSON shape go-github's RateLimitService.Get decodes:
// {"resources":{"core":{"limit":N,"remaining":N,"reset":unixSeconds}}}.
type rateLimitJSON struct {
	Resources rateLimitResourcesJSON `json:"resources"`
}

type rateLimitResourcesJSON struct {
	Core rateJSON `json:"core"`
}

type rateJSON struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

// tryRateLimit handles GET /api/v3/rate_limit. Returns false (unhandled) for any other path.
func (m *GitHubMockServer) tryRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/v3/rate_limit" {
		return false
	}
	state := m.currentRateLimit()
	writeJSON(w, rateLimitJSON{Resources: rateLimitResourcesJSON{Core: rateJSON{
		Limit:     state.limit,
		Remaining: state.remaining,
		Reset:     state.reset.Unix(),
	}}})
	return true
}
