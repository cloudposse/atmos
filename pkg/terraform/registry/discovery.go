package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/http/proxy"
)

// ErrUpstreamStatus indicates a non-2xx response from an upstream registry call.
var ErrUpstreamStatus = errors.New("upstream registry returned non-success status")

// maxDiscoveryBody bounds the service-discovery / metadata response we read.
const maxDiscoveryBody = 1 << 20 // 1 MiB.

// services holds a registry host's resolved service endpoints (absolute URLs with
// a trailing slash).
type services struct {
	providersV1 string
	modulesV1   string
}

// discovery resolves and caches registry service endpoints via the host's
// /.well-known/terraform.json document, falling back to the standard paths.
type discovery struct {
	mu    sync.Mutex
	cache map[string]services
}

func newDiscovery() *discovery {
	return &discovery{cache: make(map[string]services)}
}

// resolve returns the service endpoints for host, fetching /.well-known/terraform.json
// once (cached) and falling back to the conventional /v1/providers/ and /v1/modules/
// paths when discovery is unavailable.
func (d *discovery) resolve(ctx context.Context, fetch proxy.Fetcher, host string) services {
	d.mu.Lock()
	if s, ok := d.cache[host]; ok {
		d.mu.Unlock()
		return s
	}
	d.mu.Unlock()

	s := d.fetchServices(ctx, fetch, host)

	d.mu.Lock()
	d.cache[host] = s
	d.mu.Unlock()
	return s
}

func (d *discovery) fetchServices(ctx context.Context, fetch proxy.Fetcher, host string) services {
	fallback := services{
		providersV1: fmt.Sprintf("https://%s/v1/providers/", host),
		modulesV1:   fmt.Sprintf("https://%s/v1/modules/", host),
	}

	url := fmt.Sprintf("https://%s/.well-known/terraform.json", host)
	resp, err := fetch(ctx, proxy.UpstreamRequest{URL: url})
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fallback
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryBody))
	if err != nil {
		return fallback
	}

	var doc struct {
		ProvidersV1 string `json:"providers.v1"`
		ModulesV1   string `json:"modules.v1"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fallback
	}

	resolved := fallback
	if doc.ProvidersV1 != "" {
		resolved.providersV1 = absoluteBase(host, doc.ProvidersV1)
	}
	if doc.ModulesV1 != "" {
		resolved.modulesV1 = absoluteBase(host, doc.ModulesV1)
	}
	return resolved
}

// absoluteBase resolves a (possibly relative) service path against the host and
// ensures a trailing slash so endpoint segments can be appended directly.
func absoluteBase(host, p string) string {
	var base string
	switch {
	case strings.HasPrefix(p, "http://"), strings.HasPrefix(p, "https://"):
		base = p
	default:
		base = fmt.Sprintf("https://%s%s", host, p)
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base
}

// fetchJSON performs a propagated GET and decodes the JSON body into T.
func fetchJSON[T any](ctx context.Context, fetch proxy.Fetcher, url string) (T, error) {
	var out T
	resp, err := fetch(ctx, proxy.UpstreamRequest{URL: url})
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return out, fmt.Errorf("%w: %d for %s", ErrUpstreamStatus, resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryBody))
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	return out, nil
}

// httpFetcher adapts a proxy.Doer + inbound request into a propagated Fetcher for
// mirror pre-flight calls made during Route (discovery, archive resolution).
func httpFetcher(client proxy.Doer, inbound *http.Request) proxy.Fetcher {
	return func(ctx context.Context, up proxy.UpstreamRequest) (*http.Response, error) {
		req, err := proxy.BuildUpstreamRequest(ctx, inbound, up)
		if err != nil {
			return nil, err
		}
		return client.Do(req)
	}
}
