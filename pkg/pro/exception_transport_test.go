package pro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// exceptionTestEvent is deliberately independent of the SDK's client enrichment.
func exceptionTestEvent() *sentry.Event {
	return &sentry.Event{
		EventID:     "0123456789abcdef0123456789abcdef",
		Timestamp:   time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
		Level:       sentry.LevelError,
		Message:     "component failed",
		Fingerprint: []string{"terraform", "plan", "invalid configuration"},
		Tags:        map[string]string{"team": "platform", "production": "true", "atmos.component": "vpc", "atmos.stack": "prod", "atmos.execution_id": "12345678-1234-4234-8234-123456789abc"},
		Contexts:    map[string]sentry.Context{"atmos_execution": {"execution_id": "12345678-1234-4234-8234-123456789abc", "command": "terraform plan"}},
	}
}

func decodeExceptionEnvelope(t *testing.T, data []byte) *sentry.Event {
	t.Helper()
	parts := bytes.SplitN(data, []byte("\n"), 3)
	require.Len(t, parts, 3)
	var header map[string]any
	require.NoError(t, json.Unmarshal(parts[0], &header))
	var item struct {
		Type   string `json:"type"`
		Length int    `json:"length"`
	}
	require.NoError(t, json.Unmarshal(parts[1], &item))
	require.Equal(t, "event", item.Type)
	require.Equal(t, item.Length, len(bytes.TrimSuffix(parts[2], []byte("\n"))))
	var event sentry.Event
	require.NoError(t, json.Unmarshal(parts[2], &event))
	require.Equal(t, string(event.EventID), header["event_id"])
	return &event
}

func TestExceptionTransportFreshOIDCAndEnvelope(t *testing.T) {
	var tokens atomic.Int32
	oidc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, DefaultProAudience, r.URL.Query().Get("audience"))
		assert.Equal(t, "Bearer request-credential", r.Header.Get("Authorization"))
		_, _ = fmt.Fprintf(w, `{"value":"token-%d"}`, tokens.Add(1))
	}))
	defer oidc.Close()
	events := make(chan *sentry.Event, 2)
	auth := make(chan string, 2)
	endpoint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/events/123/exceptions", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-sentry-envelope", r.Header.Get("Content-Type"))
		data, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		events <- decodeExceptionEnvelope(t, data)
		auth <- r.Header.Get("X-Sentry-Auth")
		w.WriteHeader(http.StatusOK)
	}))
	defer endpoint.Close()
	transport, err := NewExceptionTransport(&schema.ProSettings{BaseURL: endpoint.URL, GithubOIDC: schema.GithubOIDCSettings{RequestURL: oidc.URL + "?existing=1", RequestToken: "request-credential"}}, "123", ExceptionTransportOptions{HTTPClient: oidc.Client()})
	require.NoError(t, err)
	defer transport.Close()
	transport.Configure(sentry.ClientOptions{})
	event := exceptionTestEvent()
	transport.SendEvent(event)
	transport.SendEvent(event)
	require.True(t, transport.Flush(3*time.Second))
	require.EqualValues(t, 2, tokens.Load())
	require.Len(t, events, 2)
	for i := 1; i <= 2; i++ {
		delivered := <-events
		assert.Equal(t, event.EventID, delivered.EventID)
		assert.Equal(t, event.Fingerprint, delivered.Fingerprint)
		assert.Equal(t, event.Tags, delivered.Tags)
		assert.Contains(t, <-auth, fmt.Sprintf("sentry_key=token-%d", i))
	}
}

func TestExceptionTransportFailureAndShutdown(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(status) }))
			defer server.Close()
			transport, err := NewExceptionTransport(&schema.ProSettings{BaseURL: server.URL}, "123", ExceptionTransportOptions{Token: func(context.Context) (string, error) { return "test-token", nil }})
			require.NoError(t, err)
			defer transport.Close()
			transport.Configure(sentry.ClientOptions{})
			transport.SendEvent(exceptionTestEvent())
			require.True(t, transport.Flush(time.Second))
			assert.EqualValues(t, 1, calls.Load(), "rejections complete without retries or changing the command result")
		})
	}
	t.Run("cancel blocked token and queued events", func(t *testing.T) {
		started, canceled := make(chan struct{}), make(chan struct{})
		var startOnce, cancelOnce sync.Once
		transport, err := NewExceptionTransport(&schema.ProSettings{}, "123", ExceptionTransportOptions{Token: func(ctx context.Context) (string, error) {
			startOnce.Do(func() { close(started) })
			<-ctx.Done()
			cancelOnce.Do(func() { close(canceled) })
			return "", ctx.Err()
		}})
		require.NoError(t, err)
		defer transport.Close()
		transport.Configure(sentry.ClientOptions{})
		transport.SendEvent(exceptionTestEvent())
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("token acquisition did not start")
		}
		for i := 0; i < 100; i++ {
			transport.SendEvent(exceptionTestEvent())
		}
		start := time.Now()
		require.False(t, transport.Flush(20*time.Millisecond))
		require.Less(t, time.Since(start), time.Second)
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("token acquisition was not canceled")
		}
		transport.Close()
		transport.SendEvent(exceptionTestEvent())
	})
	t.Run("cancel blocked HTTP request", func(t *testing.T) {
		started, canceled := make(chan struct{}), make(chan struct{})
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			close(started)
			select {
			case <-r.Context().Done():
				close(canceled)
			case <-release:
			}
		}))
		defer server.Close()
		defer close(release)
		transport, err := NewExceptionTransport(&schema.ProSettings{BaseURL: server.URL}, "123", ExceptionTransportOptions{Token: func(ctx context.Context) (string, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "the delivery budget includes token acquisition")
			assert.LessOrEqual(t, time.Until(deadline), exceptionDeliveryTimeout)
			return "test-token", nil
		}})
		require.NoError(t, err)
		defer transport.Close()
		transport.Configure(sentry.ClientOptions{})
		transport.SendEvent(exceptionTestEvent())
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("HTTP delivery did not start")
		}
		require.False(t, transport.Flush(20*time.Millisecond))
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("HTTP delivery was not canceled")
		}
	})
	t.Run("oversized event never requests a token", func(t *testing.T) {
		var tokens atomic.Int32
		transport, err := NewExceptionTransport(&schema.ProSettings{}, "123", ExceptionTransportOptions{Token: func(context.Context) (string, error) { tokens.Add(1); return "test-token", nil }})
		require.NoError(t, err)
		defer transport.Close()
		transport.Configure(sentry.ClientOptions{})
		event := exceptionTestEvent()
		event.Message = strings.Repeat("x", maxExceptionEnvelopeBytes)
		transport.SendEvent(event)
		require.True(t, transport.Flush(time.Second))
		assert.Zero(t, tokens.Load())
		_, err = exceptionEnvelope(strings.NewReader(event.Message))
		assert.ErrorIs(t, err, errUtils.ErrFailedToMarshalPayload)
	})
}

func TestExceptionEndpoint(t *testing.T) {
	for _, base := range []string{"https://qa-2.atmos-pro.com", "https://atmos-pro.com/prefix/"} {
		endpoint, err := exceptionEndpoint(base, "123")
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSuffix(base, "/")+"/api/v1/events/123/exceptions", endpoint.String())
	}
	for _, base := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com?secret=1", "ftp://example.com", ":bad"} {
		_, err := exceptionEndpoint(base, "123")
		require.Error(t, err)
	}
	for _, id := range []string{"", "0", "../123", "org/repo"} {
		_, err := exceptionEndpoint("", id)
		require.Error(t, err)
	}
}
