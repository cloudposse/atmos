package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

const (
	// DefaultBaseURL is the GitHub API address.
	DefaultBaseURL = "https://api.github.com"
	// DefaultUserAgent identifies the plugin. GitHub rejects requests without a User-Agent.
	DefaultUserAgent = "atmos-ssh-plugin"

	apiVersion     = "2022-11-28"
	requestTimeout = 30 * time.Second
	maxBodyBytes   = 4 << 20
	maxPages       = 1000
	perPage        = 100
)

var nextLink = regexp.MustCompile(`<([^>]*)>\s*;\s*rel="next"`)

// client makes authenticated GitHub API calls. The token lives only here and in the request
// header; it is scrubbed from anything that becomes an error message.
type client struct {
	http    *http.Client
	base    *url.URL
	token   string
	agent   string
	timeout time.Duration
}

// call describes one request for error reporting.
type call struct {
	operation  string
	use        authority.Use
	permission string
}

// response is a completed request.
type response struct {
	status int
	header http.Header
	body   []byte
}

func (c *client) send(ctx context.Context, method, endpoint string, query url.Values, payload any) (*response, error) {
	var reader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	target := *c.base
	target.Path = strings.TrimRight(c.base.Path, "/") + endpoint
	target.RawQuery = query.Encode()
	// The caller's client may have no timeout, so every request carries its own.
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", c.agent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New(c.scrub(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, errors.New(c.scrub(err.Error()))
	}
	return &response{status: resp.StatusCode, header: resp.Header, body: body}, nil
}

// do sends a request and turns a failed status into a typed error.
func (c *client) do(ctx context.Context, info call, method, endpoint string, query url.Values, payload any) (*response, error) {
	resp, err := c.send(ctx, method, endpoint, query, payload)
	if err != nil {
		return nil, &authority.Error{Authority: Name, Operation: info.operation, Use: info.use, Err: authority.ErrUnavailable, Detail: err.Error()}
	}
	if resp.status >= 200 && resp.status < 300 {
		return resp, nil
	}
	return resp, c.classify(info, method, resp)
}

// classify maps a failed response onto the error sentinels.
func (c *client) classify(info call, method string, resp *response) error {
	e := &authority.Error{
		Authority: Name, Operation: info.operation, Use: info.use,
		Status: resp.status, Detail: c.message(resp),
	}
	switch {
	case resp.status == http.StatusUnauthorized:
		e.Err = authority.ErrUnauthorized
	case resp.status == http.StatusTooManyRequests, resp.status == http.StatusForbidden && rateLimited(resp):
		e.Err = authority.ErrRateLimited
		e.RetryAfter = retryAfter(resp.header)
	case resp.status == http.StatusForbidden, resp.status == http.StatusNotFound:
		e.Err = authority.ErrMissingPermission
		e.Permission = info.permission
		e.Detail = joinDetail(e.Detail, scopeDetail(resp.header))
	case resp.status == http.StatusUnprocessableEntity && method == http.MethodPost:
		e.Err = authority.ErrKeyInUse
	default:
		e.Err = authority.ErrUnexpectedStatus
	}
	return e
}

func joinDetail(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "; ")
}

// message extracts GitHub's explanation from an error body, with the token scrubbed.
func (c *client) message(resp *response) string {
	var parsed struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	text := strings.TrimSpace(string(resp.body))
	if json.Unmarshal(resp.body, &parsed) == nil && parsed.Message != "" {
		text = parsed.Message
		for _, e := range parsed.Errors {
			if e.Message != "" {
				text += ": " + e.Message
			}
		}
	}
	if len(text) > 300 {
		text = text[:300] + "..."
	}
	return c.scrub(text)
}

// scrub removes the token from text.
func (c *client) scrub(text string) string {
	if c.token == "" {
		return text
	}
	return strings.ReplaceAll(text, c.token, "[redacted]")
}

func rateLimited(resp *response) bool {
	if resp.header.Get("X-RateLimit-Remaining") == "0" || resp.header.Get("Retry-After") != "" {
		return true
	}
	return strings.Contains(strings.ToLower(string(resp.body)), "rate limit")
}

func retryAfter(h http.Header) time.Duration {
	if secs, err := strconv.Atoi(h.Get("Retry-After")); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		if d := time.Until(time.Unix(reset, 0)); d > 0 {
			return d
		}
	}
	return 0
}

// scopeDetail reports the scopes GitHub says the endpoint accepts and the token has.
func scopeDetail(h http.Header) string {
	accepted, granted := h.Get("X-Accepted-OAuth-Scopes"), h.Get("X-OAuth-Scopes")
	if accepted == "" && granted == "" {
		return ""
	}
	return fmt.Sprintf("endpoint accepts [%s]; token has [%s]", accepted, granted)
}

// lacksScope reports whether GitHub's scope headers show that the token has none of the scopes
// the endpoint accepts. It is false when the headers are absent, as for fine-grained tokens.
func lacksScope(h http.Header) bool {
	accepted := splitScopes(h.Get("X-Accepted-OAuth-Scopes"))
	if len(accepted) == 0 {
		return false
	}
	if _, present := h["X-Oauth-Scopes"]; !present {
		return false
	}
	have := map[string]bool{}
	for _, s := range splitScopes(h.Get("X-OAuth-Scopes")) {
		have[s] = true
	}
	for _, s := range accepted {
		if have[s] {
			return false
		}
	}
	return true
}

func splitScopes(header string) []string {
	var out []string
	for _, s := range strings.Split(header, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// nextPage returns the page number the Link header points at, or 0 when there is no next page.
// Only the page number is used, so a Link to another host is never followed.
func nextPage(h http.Header, current int) int {
	m := nextLink.FindStringSubmatch(h.Get("Link"))
	if m == nil {
		return 0
	}
	u, err := url.Parse(m[1])
	if err != nil {
		return 0
	}
	page, err := strconv.Atoi(u.Query().Get("page"))
	if err != nil || page <= current {
		return 0
	}
	return page
}
