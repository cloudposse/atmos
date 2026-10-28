package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Browser supplies browser interaction and loopback callback transport.
type Browser interface {
	Open(context.Context, string) error
	Listen(context.Context, string, string) (CallbackListener, error)
}

// Callback contains authorization query parameters, never a rendered web response.
type Callback struct{ Code, State, Error string }

// CallbackListener is ready before it is returned. Closing it releases the socket.
type CallbackListener interface {
	RedirectURI() string
	Receive(context.Context) (Callback, error)
	Close() error
}

// LocalBrowser uses an injected browser opener and loopback-only callback listener.
// Interactive must be set by the invocation, never by the plugin.
type LocalBrowser struct {
	Interactive bool
	OpenURL     func(string) error
}

// Open opens an HTTP(S) authorization URL without logging its query parameters.
func (b LocalBrowser) Open(ctx context.Context, rawURL string) error {
	defer perf.Track(nil, "api.LocalBrowser.Open")()

	if !b.Interactive {
		return errUtils.ErrPluginInteractionRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return errUtils.ErrPluginInvalidConfig
	}
	if b.OpenURL == nil {
		return errUtils.ErrPluginInteractionRequired
	}
	return b.OpenURL(rawURL)
}

// Listen binds before returning. Wrong-state or unrelated requests leave the callback usable.
func (b LocalBrowser) Listen(ctx context.Context, path, expectedState string) (CallbackListener, error) {
	defer perf.Track(nil, "api.LocalBrowser.Listen")()

	if !b.Interactive {
		return nil, errUtils.ErrPluginInteractionRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#\\\r\n") || expectedState == "" {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginAPI, err)
	}
	lifetime, cancel := context.WithCancel(ctx)
	result := &loopbackCallback{uri: (&url.URL{Scheme: "http", Host: listener.Addr().String(), Path: path}).String(), ctx: lifetime, cancel: cancel, callbacks: make(chan Callback, 1)}
	result.server = &http.Server{Handler: callbackHandler(path, expectedState, result.callbacks), ReadHeaderTimeout: callbackHeaderTimeout}
	go func() { defer cancel(); _ = result.server.Serve(listener) }()
	go func() { <-lifetime.Done(); _ = result.server.Close() }()
	return result, nil
}

const callbackHeaderTimeout = 5 * time.Second

func callbackHandler(path, expectedState string, results chan<- Callback) http.Handler {
	var consumed atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		callback, valid := parseCallback(r, path, expectedState)
		if !valid {
			http.Error(w, "Invalid authorization callback.", http.StatusBadRequest)
			return
		}
		if !consumed.CompareAndSwap(false, true) {
			http.Error(w, "Authorization callback already received.", http.StatusConflict)
			return
		}
		// The bridge closes this server as soon as it receives the callback.
		// Send a complete, fixed-length response first so that close cannot
		// truncate the browser's response or its chunked transfer terminator.
		const response = "Authorization response received. You can close this tab."
		w.Header().Set("Content-Length", strconv.Itoa(len(response)))
		_, _ = io.WriteString(w, response)
		_ = http.NewResponseController(w).Flush()
		// The channel has room for exactly one result and only the CAS winner writes it.
		results <- callback
		// Closing after the sole result wakes every concurrent and future receiver.
		close(results)
	})
}

type loopbackCallback struct {
	uri       string
	ctx       context.Context
	cancel    context.CancelFunc
	server    *http.Server
	callbacks chan Callback
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (l *loopbackCallback) RedirectURI() string { return l.uri }

func (l *loopbackCallback) Receive(ctx context.Context) (Callback, error) {
	defer perf.Track(nil, "api.loopbackCallback.Receive")()

	select {
	case callback, ok := <-l.callbacks:
		if !ok {
			return Callback{}, io.EOF
		}
		return callback, nil
	case <-ctx.Done():
		return Callback{}, ctx.Err()
	case <-l.ctx.Done():
		return Callback{}, l.ctx.Err()
	}
}

func (l *loopbackCallback) Close() error {
	defer perf.Track(nil, "api.loopbackCallback.Close")()

	l.cancel()
	return l.server.Close()
}

func parseCallback(r *http.Request, path, expectedState string) (Callback, bool) {
	query := r.URL.Query()
	state := query.Get("state")
	if r.Method != http.MethodGet || r.URL.Path != path || len(query["state"]) != 1 {
		return Callback{}, false
	}
	valid := subtle.ConstantTimeCompare([]byte(state), []byte(expectedState)) == 1
	code, failure := query.Get("code"), query.Get("error")
	valid = valid && ((code != "" && failure == "" && len(query["code"]) == 1) || (failure != "" && code == "" && len(query["error"]) == 1))
	return Callback{Code: code, State: state, Error: failure}, valid
}
