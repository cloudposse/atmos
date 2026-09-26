package pro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

const (
	exceptionDeliveryTimeout  = 2 * time.Second
	maxExceptionEnvelopeBytes = 1 << 20
	exceptionBufferSize       = 32
	// The SDK needs a DSN for serialization; only exceptionEndpoint is contacted.
	exceptionDSN = "https://public@atmos.invalid/1"
)

// ExceptionTransportOptions supplies injectable HTTP and token dependencies.
type ExceptionTransportOptions struct {
	HTTPClient *http.Client
	Token      func(context.Context) (string, error)
}

// ExceptionTransport sends buffered Sentry events to the repository's Pro endpoint.
type ExceptionTransport struct {
	inner      *sentry.HTTPTransport
	endpoint   *url.URL
	client     *http.Client
	token      func(context.Context) (string, error)
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	configured atomic.Bool
}

var _ sentry.Transport = (*ExceptionTransport)(nil)

// NewExceptionTransport creates a transport without fetching credentials or sending requests.
func NewExceptionTransport(settings *schema.ProSettings, repositoryID string, options ExceptionTransportOptions) (*ExceptionTransport, error) {
	defer perf.Track(nil, "pro.NewExceptionTransport")()

	if settings == nil {
		settings = &schema.ProSettings{}
	}
	endpoint, err := exceptionEndpoint(settings.BaseURL, repositoryID)
	if err != nil {
		return nil, err
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: exceptionDeliveryTimeout}
	}
	// OIDC request credentials and Sentry auth must never follow redirects.
	boundedClient := *client
	boundedClient.Timeout = exceptionDeliveryTimeout
	boundedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	token := options.Token
	if token == nil {
		oidc := settings.GithubOIDC
		token = func(ctx context.Context) (string, error) {
			return GetGitHubOIDCTokenContext(ctx, oidc, DefaultProAudience, &boundedClient)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &ExceptionTransport{inner: sentry.NewHTTPTransport(), endpoint: endpoint, client: &boundedClient, token: token, ctx: ctx, cancel: cancel}
	t.inner.BufferSize = exceptionBufferSize
	return t, nil
}

func exceptionEndpoint(baseURL, repositoryID string) (*url.URL, error) {
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(repositoryID) {
		return nil, errUtils.ErrInvalidURL
	}
	if baseURL == "" {
		baseURL = cfg.AtmosProDefaultBaseUrl
	}
	u, err := url.Parse(baseURL)
	if err != nil || !validExceptionBaseURL(u) {
		return nil, errUtils.ErrInvalidURL
	}
	local := u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return nil, errUtils.ErrInvalidURL
	}
	return u.JoinPath("api/v1/events", repositoryID, "exceptions"), nil
}

func validExceptionBaseURL(u *url.URL) bool {
	return u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// Configure installs Pro routing and retains the SDK's envelope serializer and queue.
//
//nolint:gocritic // The SDK Transport interface requires ClientOptions by value.
func (t *ExceptionTransport) Configure(_ sentry.ClientOptions) {
	defer perf.Track(nil, "pro.ExceptionTransport.Configure")()

	t.once.Do(func() {
		client := *t.client
		base := client.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		client.Transport = &exceptionRoundTripper{owner: t, base: base}
		t.inner.Configure(sentry.ClientOptions{Dsn: exceptionDSN, HTTPClient: &client})
		t.configured.Store(true)
	})
}

// SendEvent snapshots and masks each event before returning it to the SDK queue.
func (t *ExceptionTransport) SendEvent(event *sentry.Event) {
	defer perf.Track(nil, "pro.ExceptionTransport.SendEvent")()

	if event == nil || !t.configured.Load() || t.ctx.Err() != nil {
		return
	}
	copy, err := maskedExceptionEvent(event)
	if err != nil {
		log.Debug("Pro exception skipped: invalid or oversized event")
		return
	}
	t.inner.SendEventWithContext(t.ctx, copy)
}

// Flush waits for delivery attempts, including rejected requests.
func (t *ExceptionTransport) Flush(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return t.FlushWithContext(ctx)
}

// FlushWithContext cancels pending delivery when the shutdown deadline expires.
func (t *ExceptionTransport) FlushWithContext(ctx context.Context) bool {
	defer perf.Track(nil, "pro.ExceptionTransport.FlushWithContext")()

	if !t.configured.Load() {
		return true
	}
	if !t.inner.FlushWithContext(ctx) {
		t.Close()
		return false
	}
	return true
}

// Close cancels outstanding requests and stops the worker; it is idempotent.
func (t *ExceptionTransport) Close() { t.cancel(); t.inner.Close() }

type exceptionRoundTripper struct {
	owner *ExceptionTransport
	base  http.RoundTripper
}

func (t *exceptionRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	defer request.Body.Close()
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	body, err := exceptionEnvelope(request.Body)
	if err != nil {
		return nil, err
	}
	token, err := t.owner.token(request.Context())
	if err != nil || token == "" {
		return nil, errUtils.ErrFailedToGetOIDCToken
	}
	req, err := http.NewRequestWithContext(request.Context(), http.MethodPost, t.owner.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errUtils.ErrFailedToCreateRequest
	}
	req.Header.Set("Content-Type", "application/x-sentry-envelope")
	req.Header.Set("X-Sentry-Auth", fmt.Sprintf("Sentry sentry_version=7, sentry_client=atmos-cli/%s, sentry_key=%s", version.Version, token))
	req.Header.Set("User-Agent", "atmos-cli/"+version.Version)
	response, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, errUtils.ErrFailedToMakeRequest
	}
	log.Debug("Pro exception delivery completed", "status", response.StatusCode)
	return response, nil
}

// exceptionEnvelope emits a minimal header; routing and credentials belong in HTTP.
// SDK item framing and event timestamps are preserved. Optional sent_at is omitted.
func exceptionEnvelope(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxExceptionEnvelopeBytes+1))
	if err != nil {
		return nil, errUtils.ErrFailedToReadResponseBody
	}
	if len(body) > maxExceptionEnvelopeBytes {
		return nil, errUtils.ErrProExceptionEnvelopeTooLarge
	}
	header, items, found := bytes.Cut(body, []byte("\n"))
	var identity struct {
		EventID sentry.EventID `json:"event_id"`
	}
	if !found || json.Unmarshal(header, &identity) != nil || identity.EventID == "" {
		return nil, errUtils.ErrFailedToMarshalPayload
	}
	header, err = json.Marshal(identity)
	if err != nil {
		return nil, errUtils.ErrFailedToMarshalPayload
	}
	return append(append(header, '\n'), items...), nil
}
