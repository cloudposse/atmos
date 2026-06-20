package step

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	webhookDefaultTimeout = 30 * time.Second
	webhookDialTimeout    = 10 * time.Second
	webhookIdleTimeout    = 30 * time.Second
	webhookRespHdrTimeout = 0 // No separate response-header timeout; the per-attempt context governs the whole request.

	contentTypeHeader = "Content-Type"
	contentTypeForm   = "application/x-www-form-urlencoded"
	contentTypeJSON   = "application/json"

	httpServerErrorMin = 500
	httpSuccessMin     = 200
	httpSuccessMax     = 299

	// Bound how much of a response body is read into memory to protect against
	// large/error endpoint responses spiking memory.
	webhookMaxResponseBodyBytes int64 = 4 << 20 // 4 MiB.
)

// Webhook step result metadata keys.
const (
	metaStatusCode      = "status_code"
	metaStatus          = "status"
	metaResponseHeaders = "response_headers"
	metaDuration        = "duration"
	metaAttempts        = "attempts"
)

// webhookMethods lists the HTTP verbs the webhook step accepts.
var webhookMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodPatch:   true,
	http.MethodDelete:  true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// WebhookHandler executes an HTTP request (GET, POST, and other verbs) with query, body,
// headers, timeouts, and retries. It composes with the step's `retry:` configuration.
type WebhookHandler struct {
	BaseHandler
}

func init() {
	Register(&WebhookHandler{
		BaseHandler: NewBaseHandler("webhook", CategoryCommand, false),
	})
}

// webhookRequest is the fully resolved request spec, rebuilt into an *http.Request on
// every attempt so the body reader is fresh for retries.
type webhookRequest struct {
	method  string
	url     string
	headers map[string]string
	body    []byte
}

// webhookExpect holds compiled success criteria.
type webhookExpect struct {
	statuses  []int
	responses []*regexp.Regexp
}

// failureReason distinguishes why a response was rejected, to pick the right error.
type failureReason int

const (
	reasonNone failureReason = iota
	reasonStatus
	reasonResponse
)

// webhookHTTPError carries the outcome of a single attempt so the retry predicate and
// the final error builder can classify it.
type webhookHTTPError struct {
	transport  bool // True when no response was received (network/transport failure).
	statusCode int
	status     string
	body       string
	reason     failureReason
	cause      error
}

func (e *webhookHTTPError) Error() string { //nolint:lintroller // Trivial error-interface method; perf.Track overhead is unwarranted.
	if e.transport {
		return fmt.Sprintf("webhook transport error: %v", e.cause)
	}
	return fmt.Sprintf("webhook returned %s", e.status)
}

func (e *webhookHTTPError) Unwrap() error { //nolint:lintroller // Trivial error-interface method; perf.Track overhead is unwarranted.
	return e.cause
}

// Validate checks the webhook step configuration before execution.
func (h *WebhookHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.WebhookHandler.Validate")()

	if strings.TrimSpace(step.URL) == "" {
		return errUtils.Build(errUtils.ErrWebhookURLRequired).
			WithContext("step", step.Name).
			WithHint("Set 'url' to the endpoint the webhook step should call").
			Err()
	}

	if step.Method != "" {
		method := strings.ToUpper(strings.TrimSpace(step.Method))
		if !webhookMethods[method] {
			return errUtils.Build(errUtils.ErrWebhookInvalidMethod).
				WithContext("step", step.Name).
				WithContext("method", step.Method).
				WithHintf("Use one of: %s", strings.Join(sortedMethods(), ", ")).
				Err()
		}
	}

	if step.Body != "" && len(step.Form) > 0 {
		return errUtils.Build(errUtils.ErrWebhookBodyFormConflict).
			WithContext("step", step.Name).
			WithHint("Set 'body' for a raw payload OR 'form' for key-value params, not both").
			Err()
	}

	if step.Expect != nil {
		for _, pattern := range step.Expect.Response {
			if _, err := regexp.Compile(stripRegexSlashes(pattern)); err != nil {
				return errUtils.Build(errUtils.ErrWebhookInvalidExpectPattern).
					WithCause(err).
					WithContext("step", step.Name).
					WithContext("pattern", pattern).
					WithHint("Provide a valid regular expression, optionally wrapped in /.../").
					Err()
			}
		}
	}

	return nil
}

// Execute performs the HTTP request, applying per-attempt timeouts and retry.
func (h *WebhookHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.WebhookHandler.Execute")()

	req, err := buildWebhookRequest(step, vars)
	if err != nil {
		return nil, err
	}

	expect, err := compileWebhookExpect(step.Expect)
	if err != nil {
		return nil, err
	}

	perAttempt, err := resolveWebhookTimeout(step, vars)
	if err != nil {
		return nil, err
	}

	client := newWebhookHTTPClient()

	var lastResult *StepResult
	attempts := 0
	start := time.Now()

	doRequest := func() error {
		attempts++
		attemptCtx, cancel := context.WithTimeout(ctx, perAttempt)
		defer cancel()

		log.Debug("Executing webhook step", "step", step.Name, "method", req.method, "url", req.url, "attempt", attempts)

		result, reqErr := performWebhookRequest(attemptCtx, client, req, expect)
		if result != nil {
			lastResult = result
		}
		return reqErr
	}

	retryErr := retry.WithPredicate(ctx, step.Retry, doRequest, shouldRetryWebhook(step.Retry))

	if lastResult != nil {
		lastResult.WithMetadata(metaAttempts, attempts).
			WithMetadata(metaDuration, time.Since(start).String())
	}

	if retryErr != nil {
		return lastResult, buildWebhookError(step, req, retryErr)
	}

	return lastResult, nil
}

// performWebhookRequest executes a single attempt and evaluates success criteria.
func performWebhookRequest(ctx context.Context, client *http.Client, req *webhookRequest, expect *webhookExpect) (*StepResult, error) {
	var bodyReader io.Reader
	if len(req.body) > 0 {
		bodyReader = bytes.NewReader(req.body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.method, req.url, bodyReader)
	if err != nil {
		return nil, &webhookHTTPError{transport: true, cause: err}
	}
	for key, value := range req.headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := client.Do(httpReq) //nolint:gosec // G704: the URL is operator-provided workflow configuration; calling it is the step's purpose.
	if err != nil {
		return nil, &webhookHTTPError{transport: true, cause: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, webhookMaxResponseBodyBytes))
	if err != nil {
		return nil, &webhookHTTPError{transport: true, cause: err}
	}

	result := NewStepResult(string(respBody)).
		WithMetadata(metaStatusCode, resp.StatusCode).
		WithMetadata(metaStatus, resp.Status).
		WithMetadata(metaResponseHeaders, flattenHeaders(resp.Header))

	if reason := expect.check(resp.StatusCode, string(respBody)); reason != reasonNone {
		return result, &webhookHTTPError{
			statusCode: resp.StatusCode,
			status:     resp.Status,
			body:       string(respBody),
			reason:     reason,
		}
	}

	return result, nil
}

// check returns the reason a response failed the success criteria, or reasonNone on success.
func (e *webhookExpect) check(statusCode int, body string) failureReason {
	statusOK := statusCode >= httpSuccessMin && statusCode <= httpSuccessMax
	if len(e.statuses) > 0 {
		statusOK = false
		for _, want := range e.statuses {
			if statusCode == want {
				statusOK = true
				break
			}
		}
	}
	if !statusOK {
		return reasonStatus
	}

	if len(e.responses) > 0 {
		for _, re := range e.responses {
			if re.MatchString(body) {
				return reasonNone
			}
		}
		return reasonResponse
	}

	return reasonNone
}

// shouldRetryWebhook builds the retry predicate: retry on transport errors, 5xx, 429,
// or anything matching the configured retry.conditions regexes; never on other failures.
func shouldRetryWebhook(cfg *schema.RetryConfig) func(error) bool {
	return func(err error) bool {
		var httpErr *webhookHTTPError
		if !errors.As(err, &httpErr) {
			return false
		}
		if httpErr.transport {
			return true
		}
		if httpErr.statusCode >= httpServerErrorMin || httpErr.statusCode == http.StatusTooManyRequests {
			return true
		}
		return matchesRetryConditions(cfg, httpErr)
	}
}

// matchesRetryConditions reports whether the response matches any retry.conditions regex.
// The patterns are matched against "<status-code> <body>".
func matchesRetryConditions(cfg *schema.RetryConfig, httpErr *webhookHTTPError) bool {
	if cfg == nil || len(cfg.Conditions) == 0 {
		return false
	}
	text := strconv.Itoa(httpErr.statusCode) + " " + httpErr.body
	for _, condition := range cfg.Conditions {
		re, err := regexp.Compile(condition)
		if err != nil {
			continue
		}
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// buildWebhookError converts the retry loop's final error into a user-facing error.
func buildWebhookError(step *schema.WorkflowStep, req *webhookRequest, retryErr error) error {
	var httpErr *webhookHTTPError
	if !errors.As(retryErr, &httpErr) {
		return retryErr
	}

	if httpErr.transport {
		return errUtils.Build(errUtils.ErrWebhookRequestFailed).
			WithCause(httpErr.cause).
			WithContext("step", step.Name).
			WithContext("url", req.url).
			WithHint("Verify the URL is reachable and the timeout is large enough").
			Err()
	}

	if httpErr.reason == reasonResponse {
		return errUtils.Build(errUtils.ErrWebhookUnexpectedResponse).
			WithContext("step", step.Name).
			WithContext("url", req.url).
			WithContext("status", httpErr.status).
			WithHint("Adjust 'expect.response' or fix the endpoint so the body matches").
			Err()
	}

	return errUtils.Build(errUtils.ErrWebhookUnexpectedStatus).
		WithContext("step", step.Name).
		WithContext("url", req.url).
		WithContext("status", httpErr.status).
		WithHint("Set 'expect.status' to the codes the endpoint returns, or add a retry policy for transient failures").
		Err()
}

// buildWebhookRequest resolves templates and assembles the request spec.
func buildWebhookRequest(step *schema.WorkflowStep, vars *Variables) (*webhookRequest, error) {
	rawURL, err := resolveField(vars, step.Name, "url", step.URL)
	if err != nil {
		return nil, err
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWebhookRequestFailed).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("url", rawURL).
			WithHint("Provide a valid absolute URL (e.g. https://example.com/hook)").
			Err()
	}

	// url.Parse accepts relative URLs; reject them here so they fail fast with clear
	// config feedback instead of surfacing later as retryable transport errors.
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errUtils.Build(errUtils.ErrWebhookRequestFailed).
			WithContext("step", step.Name).
			WithContext("url", rawURL).
			WithHint("Provide a valid absolute URL (e.g. https://example.com/hook)").
			Err()
	}

	if err := applyQueryParams(parsedURL, step, vars); err != nil {
		return nil, err
	}

	headers, err := vars.ResolveEnvMap(step.Headers)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "headers").
			Err()
	}
	if headers == nil {
		headers = make(map[string]string)
	}

	body, err := buildWebhookBody(step, vars, headers)
	if err != nil {
		return nil, err
	}

	method := http.MethodGet
	if step.Method != "" {
		method = strings.ToUpper(strings.TrimSpace(step.Method))
	}

	return &webhookRequest{
		method:  method,
		url:     parsedURL.String(),
		headers: headers,
		body:    body,
	}, nil
}

// applyQueryParams resolves and appends query-string parameters to the URL.
func applyQueryParams(parsedURL *url.URL, step *schema.WorkflowStep, vars *Variables) error {
	if len(step.Query) == 0 {
		return nil
	}
	query := parsedURL.Query()
	for key, value := range step.Query {
		resolved, err := vars.Resolve(value)
		if err != nil {
			return errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "query").
				WithContext("param", key).
				Err()
		}
		query.Set(key, resolved)
	}
	parsedURL.RawQuery = query.Encode()
	return nil
}

// buildWebhookBody resolves the raw body or form params, setting a default Content-Type
// header for form payloads when none was provided.
func buildWebhookBody(step *schema.WorkflowStep, vars *Variables, headers map[string]string) ([]byte, error) {
	if step.Body != "" {
		resolved, err := vars.Resolve(step.Body)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "body").
				Err()
		}
		return []byte(resolved), nil
	}

	if len(step.Form) == 0 {
		return nil, nil
	}

	resolvedForm, err := vars.ResolveEnvMap(step.Form)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "form").
			Err()
	}

	existingCT := findHeader(headers, contentTypeHeader)
	if strings.Contains(strings.ToLower(existingCT), contentTypeJSON) {
		encoded, err := json.Marshal(resolvedForm)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrWebhookRequestFailed).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "form").
				Err()
		}
		return encoded, nil
	}

	values := url.Values{}
	for key, value := range resolvedForm {
		values.Set(key, value)
	}
	if existingCT == "" {
		headers[contentTypeHeader] = contentTypeForm
	}
	return []byte(values.Encode()), nil
}

// compileWebhookExpect compiles the success criteria. Patterns are validated in Validate,
// so a compile error here is unexpected but still surfaced.
func compileWebhookExpect(expect *schema.WebhookExpect) (*webhookExpect, error) {
	compiled := &webhookExpect{}
	if expect == nil {
		return compiled, nil
	}
	compiled.statuses = expect.Status
	for _, pattern := range expect.Response {
		re, err := regexp.Compile(stripRegexSlashes(pattern))
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrWebhookInvalidExpectPattern).
				WithCause(err).
				WithContext("pattern", pattern).
				Err()
		}
		compiled.responses = append(compiled.responses, re)
	}
	return compiled, nil
}

// resolveWebhookTimeout resolves the per-attempt timeout from the step's timeout field.
func resolveWebhookTimeout(step *schema.WorkflowStep, vars *Variables) (time.Duration, error) {
	if step.Timeout == "" {
		return webhookDefaultTimeout, nil
	}
	resolved, err := vars.Resolve(step.Timeout)
	if err != nil {
		return 0, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "timeout").
			Err()
	}
	parsed, err := time.ParseDuration(resolved)
	if err != nil {
		return 0, errUtils.Build(errUtils.ErrInvalidDuration).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("value", resolved).
			Err()
	}
	return parsed, nil
}

// newWebhookHTTPClient builds an HTTP client with hardened transport timeouts. The
// per-request deadline is enforced via context, so the client itself has no overall
// Timeout (which would otherwise race the context).
func newWebhookHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: webhookDialTimeout,
			}).DialContext,
			IdleConnTimeout:       webhookIdleTimeout,
			ResponseHeaderTimeout: webhookRespHdrTimeout,
		},
	}
}

// resolveField resolves a single templated field, wrapping template errors consistently.
func resolveField(vars *Variables, stepName, field, value string) (string, error) {
	resolved, err := vars.Resolve(value)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", stepName).
			WithContext("field", field).
			Err()
	}
	return resolved, nil
}

// findHeader returns the value of a header by canonical name, case-insensitively.
func findHeader(headers map[string]string, name string) string {
	canonical := http.CanonicalHeaderKey(name)
	for key, value := range headers {
		if http.CanonicalHeaderKey(key) == canonical {
			return value
		}
	}
	return ""
}

// flattenHeaders converts response headers into a single-valued map for step metadata.
func flattenHeaders(header http.Header) map[string]string {
	flat := make(map[string]string, len(header))
	for key, values := range header {
		flat[key] = strings.Join(values, ", ")
	}
	return flat
}

// stripRegexSlashes removes surrounding /.../ delimiters from a pattern, if present.
func stripRegexSlashes(pattern string) string {
	if len(pattern) >= 2 && strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		return pattern[1 : len(pattern)-1]
	}
	return pattern
}

// sortedMethods returns the accepted HTTP methods in a stable order for error hints.
func sortedMethods() []string {
	methods := make([]string, 0, len(webhookMethods))
	for method := range webhookMethods {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}
