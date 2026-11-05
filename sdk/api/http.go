package api

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	apiv1 "github.com/cloudposse/atmos/sdk/proto/atmos/api/v1"
)

// Transport lets standard Go OIDC and OAuth libraries use the Atmos HTTP API.
type Transport struct{ API *Client }

// RoundTrip performs exactly one exchange, preserving request cancellation.
func (t Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	conn, err := t.API.conn()
	if err != nil {
		return nil, err
	}
	var body []byte
	if request.Body != nil {
		defer request.Body.Close()
		body, err = io.ReadAll(io.LimitReader(request.Body, MaxBody+1))
		if err != nil {
			return nil, err
		}
		if len(body) > MaxBody {
			return nil, ErrPayloadTooLarge
		}
	}
	input := &apiv1.HTTPRequest{Method: request.Method, Url: request.URL.String(), Body: body, Headers: make(map[string]*apiv1.HeaderValues)}
	for key, values := range request.Header {
		input.Headers[key] = &apiv1.HeaderValues{Values: append([]string(nil), values...)}
	}
	result, err := apiv1.NewHTTPClient(conn).Do(request.Context(), input)
	if err != nil {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		return nil, err
	}
	if result.GetStatusCode() < 100 || result.GetStatusCode() > 599 || len(result.GetBody()) > MaxBody {
		return nil, ErrProtocol
	}
	response := &http.Response{StatusCode: int(result.GetStatusCode()), Status: strconv.Itoa(int(result.GetStatusCode())) + " " + http.StatusText(int(result.GetStatusCode())), Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(result.GetBody())), ContentLength: int64(len(result.GetBody())), Request: request}
	for key, values := range result.GetHeaders() {
		response.Header[key] = append([]string(nil), values.GetValues()...)
	}
	return response, nil
}

// HTTPClient returns a bounded client with no retry loop and safe redirect handling.
func (c *Client) HTTPClient() *http.Client {
	return &http.Client{Transport: Transport{API: c}, Timeout: 30 * time.Second, CheckRedirect: safeRedirect}
}

func safeRedirect(next *http.Request, previous []*http.Request) error {
	if len(previous) >= 10 {
		return ErrRedirect
	}
	last := previous[len(previous)-1]
	if last.URL.Scheme == "https" && next.URL.Scheme != "https" {
		return ErrRedirect
	}
	// Code exchange, refresh, and revocation bodies must not be replayed to a
	// redirected endpoint, even if the HTTP library would preserve the method.
	if last.Method != http.MethodGet && last.Method != http.MethodHead {
		return ErrRedirect
	}
	if !strings.EqualFold(last.URL.Scheme+"://"+last.URL.Host, next.URL.Scheme+"://"+next.URL.Host) {
		next.Header.Del("Authorization")
		next.Header.Del("Cookie")
		next.Header.Del("Proxy-Authorization")
	}
	return nil
}
