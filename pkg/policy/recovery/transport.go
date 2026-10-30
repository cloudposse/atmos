package recovery

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// Transport retries individual SDK HTTP requests, including polling, without
// replaying an enclosing install, upgrade, or other completed mutation.
type Transport struct {
	Base   http.RoundTripper
	Runner *Runner
}

// RoundTrip preserves the final SDK response and its body when retries are exhausted.
func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "recovery.Transport.RoundTrip")()

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.Runner == nil {
		return base.RoundTrip(request)
	}
	// Streaming request bodies cannot be replayed. Do not buffer credentials or uploads.
	if request.Body != nil && request.Body != http.NoBody && request.GetBody == nil {
		return base.RoundTrip(request)
	}
	attempt := httpAttempt{base: base, request: request}
	err := t.Runner.Run(request.Context(), request.Method, nil, attempt.invoke)
	return attempt.result(err)
}

func (a *httpAttempt) result(err error) (*http.Response, error) {
	if !a.attempted && a.request.Body != nil {
		_ = a.request.Body.Close()
	}
	if err != nil && (guard.IsPolicyError(err) || a.request.Context().Err() != nil) {
		a.closeResponse()
		return nil, err
	}
	if a.transportErr != nil {
		a.closeResponse()
		return nil, a.transportErr
	}
	return a.response, nil
}

type httpAttempt struct {
	base         http.RoundTripper
	request      *http.Request
	response     *http.Response
	transportErr error
	attempted    bool
}

func (a *httpAttempt) closeResponse() {
	if a.response != nil && a.response.Body != nil {
		_ = a.response.Body.Close()
	}
}

func (a *httpAttempt) nextRequest() (*http.Request, error) {
	current := a.request.Clone(a.request.Context())
	if !a.attempted {
		a.attempted = true
		return current, nil
	}
	a.closeResponse()
	if a.request.GetBody != nil {
		body, err := a.request.GetBody()
		if err != nil {
			return nil, err
		}
		current.Body = body
	}
	return current, nil
}

func (a *httpAttempt) invoke() Result {
	current, err := a.nextRequest()
	if err != nil {
		a.transportErr = err
		return Result{Err: err}
	}
	// Ownership passes to the SDK caller, or closeResponse before another attempt.
	a.response, a.transportErr = a.base.RoundTrip(current) //nolint:bodyclose // Closed before retry or returned to the SDK caller.
	if a.transportErr != nil {
		return Result{Err: a.transportErr}
	}
	if a.response.StatusCode < http.StatusBadRequest {
		return Result{}
	}
	// Retain at most 64 KiB for predicates, restoring every byte for the SDK.
	const maxErrorBodyBytes = 64 * 1024
	var prefix []byte
	if a.response.Body != nil {
		prefix, _ = io.ReadAll(io.LimitReader(a.response.Body, maxErrorBodyBytes))
		a.response.Body = &replayResponseBody{Reader: io.MultiReader(bytes.NewReader(prefix), a.response.Body), Closer: a.response.Body}
	}
	return Result{Err: httpFailure{status: a.response.StatusCode, body: string(prefix)}}
}

type replayResponseBody struct {
	io.Reader
	io.Closer
}

type httpFailure struct {
	status int
	body   string
}

//nolint:lintroller // Pure error formatting.
func (e httpFailure) Error() string {
	return fmt.Sprintf("%s: HTTP %d %s: %s", errUtils.ErrComponentExecutionFailed, e.status, http.StatusText(e.status), e.body)
}

//nolint:lintroller // Pure error code accessor.
func (e httpFailure) ErrorCode() string { return strconv.Itoa(e.status) }
