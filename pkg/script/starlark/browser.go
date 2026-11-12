package starlark

import (
	"net/url"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/browser"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WithBrowserOpener supplies the browser launch boundary. The opener must be
// concurrency safe; tests can inject an opener without launching a real browser.
func WithBrowserOpener(opener browser.Opener) Option {
	defer perf.Track(nil, "starlark.WithBrowserOpener")()

	return func(e *Engine) { e.browser = opener }
}

func (s *session) browserModule() starlark.Value {
	return module("browser", starlark.StringDict{
		"open": starlark.NewBuiltin("browser.open", s.openBrowser),
	})
}

// openBrowser launches an HTTP(S) URL with Atmos's native system-browser opener.
// It does not wait for the page to load or assume that a graphical session exists.
func (s *session) openBrowser(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.openBrowser")()

	var target string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "url", &target); err != nil {
		return nil, err
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, invalidArg("browser.open: expected an absolute HTTP(S) URL without embedded credentials")
	}
	if err := s.engine.browser.Open(target); err != nil {
		return nil, failWith(errUtils.ErrStarlarkProcessFailed, err, "browser.open: could not launch the default browser: %s", err)
	}
	return starlark.None, nil
}
