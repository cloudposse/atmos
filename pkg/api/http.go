package api

import (
	"context"
	"errors"
	"io"
	"net/http"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MaxHTTPBody bounds bodies below the plugin protocol's 16 MiB message limit.
const MaxHTTPBody = 8 << 20

// HTTP performs a single request, leaving redirect and retry decisions to the caller.
type HTTP interface {
	Do(context.Context, *http.Request) (*http.Response, error)
}

// HTTPService reuses an injected transport, including its proxy and CA configuration.
// It does not inject authentication, follow redirects, or retry a failed exchange.
type HTTPService struct{ Transport http.RoundTripper }

// Do performs one HTTP exchange under the caller's deadline.
func (s HTTPService) Do(ctx context.Context, request *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "api.HTTPService.Do")()

	if request == nil || request.URL == nil || request.URL.Host == "" || request.URL.User != nil ||
		(request.URL.Scheme != "https" && request.URL.Scheme != "http") {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	transport := s.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	response, err := transport.RoundTrip(request.Clone(ctx))
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginAPI, err)
	}
	return response, nil
}

// ReadHTTPBody reads a bounded body and always closes it.
func ReadHTTPBody(body io.ReadCloser) ([]byte, error) {
	defer perf.Track(nil, "api.ReadHTTPBody")()

	if body == nil {
		return nil, nil
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, MaxHTTPBody+1))
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginAPI, err)
	}
	if len(data) > MaxHTTPBody {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	return data, nil
}
