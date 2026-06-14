package proxy

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// fallbackUserAgent is used only when the inbound request carries no User-Agent
// (Terraform always sends one). Normal operation forwards Terraform's User-Agent
// verbatim — and since Atmos injects TF_APPEND_USER_AGENT into Terraform's own
// User-Agent, the Atmos identity rides along inside it and is never lost.
var fallbackUserAgent = "Atmos/" + version.Version

// nonForwardedHeaders are inbound headers NOT copied to the upstream fetch:
// hop-by-hop headers (RFC 7230 §6.1), plus Host (set from the upstream URL),
// Accept-Encoding and Content-Length (managed by the Go transport so cached bytes
// stay canonical/identity-encoded). Every other inbound header is forwarded so
// private registries that rely on custom headers keep working. Keys are compared
// case-insensitively via http.Header canonicalization.
var nonForwardedHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
	"Host":                true,
	"Accept-Encoding":     true,
	"Content-Length":      true,
}

// BuildUpstreamRequest builds a propagated upstream *http.Request from an inbound
// request. It is exported so Mirror adapters can make their own propagated
// pre-flight calls (e.g. resolving a provider download URL) using the same
// credential/header/User-Agent rules the proxy applies on the main fetch path.
func BuildUpstreamRequest(ctx context.Context, inbound *http.Request, up UpstreamRequest) (*http.Request, error) {
	defer perf.Track(nil, "proxy.BuildUpstreamRequest")()

	return buildUpstreamRequest(ctx, inbound, up)
}

// buildUpstreamRequest constructs the upstream *http.Request for a route, applying
// credential, header, and User-Agent propagation centrally:
//
//   - Mirror-supplied headers (up.Header) are applied first.
//   - All inbound headers are forwarded except hop-by-hop/connection-specific ones
//     (see nonForwardedHeaders), so private registries that rely on Authorization
//     or any custom header keep working.
//   - When no Authorization is forwarded, a Terraform-style TF_TOKEN_<host> bearer
//     token is resolved for the upstream host (Terraform's native credential env).
//   - The inbound (Terraform/OpenTofu) User-Agent is forwarded verbatim, so the
//     agent identity Terraform presents — including any TF_APPEND_USER_AGENT Atmos
//     injected — reaches upstream unchanged.
func buildUpstreamRequest(ctx context.Context, inbound *http.Request, up UpstreamRequest) (*http.Request, error) {
	method := up.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, up.URL, nil)
	if err != nil {
		return nil, err
	}

	// 1) Mirror-supplied headers.
	for k, vals := range up.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	// 2) Forward all sensible inbound headers (credentials + custom headers for
	// private registries). Mirror-supplied headers take precedence.
	forwardInboundHeaders(req, inbound)

	// 3) Fall back to a Terraform-style host token when no Authorization is present.
	if req.Header.Get("Authorization") == "" {
		if token := terraformHostToken(req.URL.Hostname()); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	// 4) Forward Terraform's User-Agent verbatim (fallback only when absent).
	req.Header.Set("User-Agent", inboundUserAgent(inbound))

	return req, nil
}

// forwardInboundHeaders copies all non-hop-by-hop inbound headers onto req that
// are not already set (mirror-supplied headers take precedence).
func forwardInboundHeaders(req, inbound *http.Request) {
	if inbound == nil {
		return
	}
	for h, vals := range inbound.Header {
		if nonForwardedHeaders[http.CanonicalHeaderKey(h)] || req.Header.Get(h) != "" {
			continue
		}
		for _, v := range vals {
			req.Header.Add(h, v)
		}
	}
}

// inboundUserAgent returns the inbound request's User-Agent verbatim, or a
// fallback Atmos User-Agent when the inbound request carries none.
func inboundUserAgent(inbound *http.Request) string {
	if inbound != nil {
		if ua := inbound.Header.Get("User-Agent"); ua != "" {
			return ua
		}
	}
	return fallbackUserAgent
}

// terraformHostToken resolves a bearer token for host from the Terraform/OpenTofu
// host-token environment convention, where dots in the host are replaced with
// underscores (e.g. app.terraform.io → TF_TOKEN_app_terraform_io) and hyphens map
// to double underscores. Both TF_TOKEN_<host> (Terraform, also honored by OpenTofu
// for compatibility) and TOFU_TOKEN_<host> (OpenTofu) are checked, so the proxy
// authenticates to private registries for either tool.
func terraformHostToken(host string) string {
	if host == "" {
		return ""
	}
	suffix := strings.NewReplacer(".", "_", "-", "__").Replace(host)
	for _, prefix := range []string{"TF_TOKEN_", "TOFU_TOKEN_"} {
		//nolint:forbidigo // Terraform/OpenTofu-native credential env var lookup.
		if token := os.Getenv(prefix + suffix); token != "" {
			return token
		}
	}
	return ""
}
