package provider

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/avast/retry-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/publicsuffix"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cookiejar"
)

// HTTPClient saml2aws http client which extends the existing client
type HTTPClient struct {
	http.Client
	CheckResponseStatus func(*http.Request, *http.Response) error
	Options             *HTTPClientOptions
}

const (
	DefaultAttemptsCount = 1
	DefaultRetryDelay    = time.Duration(1) * time.Second
	// MaxRetryDelay caps the configured delay between HTTP retries.
	MaxRetryDelay = 5 * time.Minute
	// DefaultRequestTimeout bounds each IdP request, including reading the response body.
	// Atmos fork: upstream left http.Client.Timeout at zero, so an IdP that accepted a request
	// and never answered blocked authentication forever. Two minutes is generous for interactive
	// IdP pages; the MFA push/poll flows repeat short requests, so each one stays well inside it.
	DefaultRequestTimeout = 2 * time.Minute
)

type HTTPClientOptions struct {
	IsWithRetries bool // http retry feature switch
	AttemptsCount uint
	RetryDelay    time.Duration
}

// MinTLSVersion is the minimum TLS version for connections to identity providers.
// Atmos fork: upstream left MinVersion unset. TLS 1.2 (not 1.3) is the floor because many
// enterprise IdPs, notably ADFS, still serve TLS 1.2 only, and ADFS also relies on TLS
// renegotiation, which TLS 1.3 removed.
const MinTLSVersion = tls.VersionTLS12

// NewDefaultTransport configure a transport with the TLS skip verify option
func NewDefaultTransport(skipVerify bool) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: MinTLSVersion, InsecureSkipVerify: skipVerify}, //nolint:gosec // Skipping verification is an explicit, user-configured opt-in (skip_verify).
	}
}

func BuildHttpClientOpts(account *cfg.IDPAccount) *HTTPClientOptions {
	opts := &HTTPClientOptions{}
	atmt, atmtErr := strconv.ParseUint(account.HttpAttemptsCount, 10, 0)
	if opts.IsWithRetries = atmtErr == nil; opts.IsWithRetries {
		// Atmos fork: upstream used the value as is, so "0" made retry-go never run the
		// request and return an empty error. At least one attempt is always made.
		opts.AttemptsCount = atLeastOneAttempt(uint(atmt))
	} else {
		if account.HttpAttemptsCount != "" {
			logrus.WithField("http_attempts_count", account.HttpAttemptsCount).
				Debug("Invalid HTTP attempts count, sending each request once")
		}
		opts.AttemptsCount = DefaultAttemptsCount
	}

	// Atmos fork: upstream parsed an unbounded integer and multiplied it into a time.Duration,
	// which overflows for large values. Parse at most 32 bits (no overflow when multiplied by
	// time.Second) and cap the delay at MaxRetryDelay.
	delay, delayErr := strconv.ParseUint(account.HttpRetryDelay, 10, 32)
	if delayErr != nil {
		if account.HttpRetryDelay != "" {
			logrus.WithField("http_retry_delay", account.HttpRetryDelay).
				Debug("Invalid HTTP retry delay, using the default")
		}
		opts.RetryDelay = DefaultRetryDelay
	} else {
		opts.RetryDelay = min(time.Duration(delay)*time.Second, MaxRetryDelay)
	}

	return opts
}

// atLeastOneAttempt clamps an attempt count to at least one: retry-go v3 runs nothing for zero attempts.
func atLeastOneAttempt(attempts uint) uint {
	return max(attempts, DefaultAttemptsCount)
}

// NewHTTPClient configure the default http client used by the providers
func NewHTTPClient(tr http.RoundTripper, opts *HTTPClientOptions) (*HTTPClient, error) {
	options := &cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}

	jar, err := cookiejar.New(options)
	if err != nil {
		return nil, err
	}

	client := http.Client{Transport: tr, Jar: jar, Timeout: DefaultRequestTimeout}

	return &HTTPClient{client, nil, opts}, nil
}

// Do do the request
func (hc *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", fmt.Sprintf("saml2aws/1.0 (%s %s) Versent", runtime.GOOS, runtime.GOARCH))

	var resp *http.Response
	var err error

	// Atmos fork: a client created with nil options sends the request once instead of panicking.
	if hc.Options != nil && hc.Options.IsWithRetries {
		resp, err = hc.doWithRetry(req)
	} else {
		hc.logHTTPRequest(req)
		resp, err = hc.Client.Do(req)
	}
	if err != nil {
		return resp, err
	}

	// if a response check has been configured
	if hc.CheckResponseStatus != nil {
		err = hc.CheckResponseStatus(req, resp)
		if err != nil {
			// Atmos fork: callers discard the response when the status check fails, so its body
			// was never closed. The status line stays available on the returned response.
			_ = resp.Body.Close()
			return resp, err
		}
	}

	hc.logHTTPResponse(resp)

	return resp, err
}

func (hc *HTTPClient) doWithRetry(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	attempted := false
	err := retry.Do(
		func() error {
			// Atmos fork: a failed attempt may have consumed the request body, so later attempts
			// send a fresh copy. Upstream resent the consumed body.
			if attempted && req.GetBody != nil {
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return retry.Unrecoverable(bodyErr)
				}
				req.Body = body
			}
			attempted = true
			hc.logHTTPRequest(req)
			clientResp, err := hc.Client.Do(req) //nolint:bodyclose // The response is returned to the caller, which closes the body.
			if err != nil {
				return err
			}
			resp = clientResp
			return nil
		},
		// Atmos fork: options built without BuildHttpClientOpts are clamped too.
		retry.Attempts(atLeastOneAttempt(hc.Options.AttemptsCount)),
		retry.Delay(hc.Options.RetryDelay),
		// Atmos fork: cancelling the request also stops the retry waits.
		retry.Context(req.Context()),
		retry.OnRetry(
			func(n uint, err error) {
				logrus.
					WithField("Attempt #", n).
					WithField("Caused by", err).
					Debug("Retry")
			}),
	)
	return resp, err
}

// DisableFollowRedirect disable redirects
func (hc *HTTPClient) DisableFollowRedirect() {
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
}

// EnableFollowRedirect enable redirects
func (hc *HTTPClient) EnableFollowRedirect() {
	hc.CheckRedirect = nil
}

// SuccessOrRedirectResponseValidator this validates the response code is within range of 200 - 399
func SuccessOrRedirectResponseValidator(req *http.Request, resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}

	return errors.Errorf("request for url: %s failed status: %s", req.URL.String(), resp.Status)
}

// SuccessOrRedirectOrUnauthorizedResponseValidator also allows 401
func SuccessOrRedirectOrUnauthorizedResponseValidator(req *http.Request, resp *http.Response) error {
	validatorResponse := SuccessOrRedirectResponseValidator(req, resp)

	if validatorResponse == nil || resp.StatusCode == 401 {
		return nil
	}

	return validatorResponse
}

// Atmos fork: upstream's DUMP_CONTENT switch dumped full requests and responses, including
// password form bodies, session cookies and SAML assertions, to the terminal. It was removed,
// and request URLs are logged without their query string, which can carry session tokens.
func (hc *HTTPClient) logHTTPRequest(req *http.Request) {
	logrus.WithField("http", "client").WithFields(logrus.Fields{
		"URL":    RedactURL(req.URL),
		"method": req.Method,
	}).Debug("HTTP Req")
}

func (hc *HTTPClient) logHTTPResponse(resp *http.Response) {
	logrus.WithField("http", "client").WithFields(logrus.Fields{
		"Status": resp.Status,
	}).Debug("HTTP Res")
}
