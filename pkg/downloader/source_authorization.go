package downloader

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

type sourceAuthorizationKey struct{}

// WithSourceAuthorization binds a registry archive header to one invocation.
func WithSourceAuthorization(ctx context.Context, auth *schema.SourceAuthorization) context.Context {
	if auth == nil {
		return ctx
	}
	copy := *auth
	return context.WithValue(ctx, sourceAuthorizationKey{}, &copy)
}

func authorizedHTTPClient(ctx context.Context, client *http.Client) *http.Client {
	auth, ok := ctx.Value(sourceAuthorizationKey{}).(*schema.SourceAuthorization)
	if !ok || auth == nil {
		return client
	}
	cloned := *client
	cloned.Transport = &sourceAuthorizationTransport{base: cloned.Transport, origin: auth.Origin, header: auth.Header}
	return &cloned
}

type sourceAuthorizationTransport struct {
	base   http.RoundTripper
	origin string
	header string
}

func (t *sourceAuthorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	origin := copy.URL.Scheme + "://" + copy.URL.Host
	if strings.EqualFold(origin, t.origin) {
		copy.Header.Set("Authorization", t.header)
	} else if copy.Header.Get("Authorization") == t.header {
		// http.Client may copy Authorization onto a redirect request. Never let
		// the registry header cross the exact origin boundary.
		copy.Header.Del("Authorization")
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(copy)
}
