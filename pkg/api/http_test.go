package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

//go:generate go run go.uber.org/mock/mockgen -source=http_test.go -destination=mock_http_test.go -package=api

type (
	httpTransport interface{ http.RoundTripper }
	httpBody      interface{ io.ReadCloser }
)

var (
	_ httpTransport = (*MockhttpTransport)(nil)
	_ httpBody      = (*MockhttpBody)(nil)
)

func TestHTTPServiceRejectsInvalidRequests(t *testing.T) {
	t.Parallel()
	for name, request := range map[string]*http.Request{
		"nil":                nil,
		"missing URL":        {},
		"missing host":       {URL: &url.URL{Scheme: "https"}},
		"credentials":        {URL: &url.URL{Scheme: "https", Host: "issuer.example", User: url.UserPassword("user", "secret")}},
		"unsupported scheme": {URL: &url.URL{Scheme: "file", Host: "issuer.example"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			service := HTTPService{Transport: NewMockhttpTransport(gomock.NewController(t))}
			response, err := service.Do(t.Context(), request)
			if response != nil {
				require.NoError(t, response.Body.Close())
			}
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Nil(t, response)
		})
	}
}

func TestHTTPServicePreservesRedirectAndRequest(t *testing.T) {
	t.Parallel()
	transport := NewMockhttpTransport(gomock.NewController(t))
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://issuer.example/token", bytes.NewBufferString("grant=code"))
	require.NoError(t, err)
	request.Header.Set("X-Request", "original")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	expected := &http.Response{Body: http.NoBody, StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://other.example"}}}
	transport.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(actual *http.Request) (*http.Response, error) {
		require.NotSame(t, request, actual)
		require.Same(t, ctx, actual.Context())
		assert.Equal(t, http.MethodPost, actual.Method)
		assert.Equal(t, request.URL.String(), actual.URL.String())
		assert.Equal(t, "original", actual.Header.Get("X-Request"))
		assert.Empty(t, actual.Header.Get("Authorization"))
		body, err := io.ReadAll(actual.Body)
		require.NoError(t, err)
		assert.Equal(t, "grant=code", string(body))
		actual.Header.Set("X-Request", "changed")
		return expected, nil
	}).Times(1)
	response, err := (HTTPService{Transport: transport}).Do(ctx, request)
	require.NoError(t, err)
	require.Same(t, expected, response, "redirects must be returned without following them")
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "original", request.Header.Get("X-Request"))
	require.Same(t, t.Context(), request.Context())
}

func TestHTTPServiceDoesNotRetryTransportFailure(t *testing.T) {
	t.Parallel()
	transport := NewMockhttpTransport(gomock.NewController(t))
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://issuer.example", nil)
	require.NoError(t, err)
	transport.EXPECT().RoundTrip(gomock.Any()).Return(nil, context.DeadlineExceeded).Times(1)
	response, err := (HTTPService{Transport: transport}).Do(t.Context(), request)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.Nil(t, response)
	require.ErrorIs(t, err, errUtils.ErrPluginAPI)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestHTTPServiceDefaultTransport(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/health", nil)
	require.NoError(t, err)
	response, err := (HTTPService{}).Do(t.Context(), request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
}

func TestReadHTTPBodyBoundsAndCloses(t *testing.T) {
	t.Parallel()
	for name, size := range map[string]int{"empty": 0, "boundary": MaxHTTPBody, "oversized": MaxHTTPBody + 2} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := NewMockhttpBody(gomock.NewController(t))
			input := bytes.Repeat([]byte("x"), size)
			reader := bytes.NewReader(input)
			body.EXPECT().Read(gomock.Any()).DoAndReturn(reader.Read).AnyTimes()
			body.EXPECT().Close().Return(nil).Times(1)
			actual, err := ReadHTTPBody(body)
			if size > MaxHTTPBody {
				require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
				require.Nil(t, actual)
				assert.Equal(t, 1, reader.Len(), "stop reading after detecting the first excess byte")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, input, actual)
		})
	}
}

func TestReadHTTPBodyFailureAndNil(t *testing.T) {
	t.Parallel()
	body := NewMockhttpBody(gomock.NewController(t))
	body.EXPECT().Read(gomock.Any()).Return(0, io.ErrUnexpectedEOF)
	body.EXPECT().Close().Return(nil).Times(1)
	actual, err := ReadHTTPBody(body)
	require.Nil(t, actual)
	require.ErrorIs(t, err, errUtils.ErrPluginAPI)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	actual, err = ReadHTTPBody(nil)
	require.NoError(t, err)
	require.Nil(t, actual)
}
