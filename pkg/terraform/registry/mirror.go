package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/http/proxy"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrInvalidPlatform indicates a platform string that is not of the form os_arch.
var ErrInvalidPlatform = errors.New("invalid provider platform")

// ErrInvalidProviderSource indicates a provider source address that is not of the
// form host/namespace/type.
var ErrInvalidProviderSource = errors.New("invalid provider source address")

// Platform is a Terraform provider target platform (operating system + CPU
// architecture), e.g. {OS: "linux", Arch: "amd64"}.
type Platform struct {
	OS   string
	Arch string
}

// String renders the platform as Terraform's canonical os_arch token.
func (p Platform) String() string {
	defer perf.Track(nil, "registry.Platform.String")()

	return p.OS + "_" + p.Arch
}

// HostPlatform returns the platform Atmos is currently running on.
func HostPlatform() Platform {
	defer perf.Track(nil, "registry.HostPlatform")()

	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// ParsePlatform parses a Terraform os_arch platform token (e.g. "linux_amd64").
func ParsePlatform(s string) (Platform, error) {
	defer perf.Track(nil, "registry.ParsePlatform")()

	parts := strings.SplitN(strings.TrimSpace(s), "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Platform{}, fmt.Errorf("%w: %q (want os_arch, e.g. linux_amd64)", ErrInvalidPlatform, s)
	}
	return Platform{OS: parts[0], Arch: parts[1]}, nil
}

// ProviderRef identifies a single provider version to mirror: a registry host,
// namespace, type, and exact version.
type ProviderRef struct {
	Host      string
	Namespace string
	Type      string
	Version   string
}

// Source renders the host/namespace/type source address (without version).
func (r ProviderRef) Source() string {
	defer perf.Track(nil, "registry.ProviderRef.Source")()

	return r.Host + "/" + r.Namespace + "/" + r.Type
}

// ParseSource builds a ProviderRef from a fully-qualified source address
// (host/namespace/type, as recorded in .terraform.lock.hcl) and an exact version.
func ParseSource(source, version string) (ProviderRef, error) {
	defer perf.Track(nil, "registry.ParseSource")()

	parts := strings.Split(source, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return ProviderRef{}, fmt.Errorf("%w: %q (want host/namespace/type)", ErrInvalidProviderSource, source)
	}
	return ProviderRef{Host: parts[0], Namespace: parts[1], Type: parts[2], Version: version}, nil
}

// fetcher returns a propagated Fetcher with no inbound request. Credential
// propagation still applies via the TF_TOKEN_<host> / TOFU_TOKEN_<host> fallback,
// and the Atmos User-Agent is used since there is no Terraform UA to forward.
func (m *ProviderMirror) fetcher() proxy.Fetcher {
	return func(ctx context.Context, up proxy.UpstreamRequest) (*http.Response, error) {
		req, err := proxy.BuildUpstreamRequest(ctx, nil, up)
		if err != nil {
			return nil, err
		}
		return m.client.Do(req)
	}
}

// DownloadInto resolves a provider version's download for a single platform and
// stores the zip in the canonical filesystem_mirror layout under the store root,
// verifying the registry-provided zh: hash. It returns the object metadata and
// whether the object was already cached (in which case nothing is downloaded).
//
// Unlike the proxy's request-driven Route path, DownloadInto is callable directly
// so commands (e.g. `atmos terraform cache warm`) can eagerly hydrate the cache —
// including for platforms other than the host's — without running Terraform.
func (m *ProviderMirror) DownloadInto(ctx context.Context, store proxy.Store, ref ProviderRef, platform Platform) (proxy.Meta, bool, error) {
	defer perf.Track(nil, "registry.ProviderMirror.DownloadInto")()

	fetch := m.fetcher()
	svc := m.disc.resolve(ctx, fetch, ref.Host)
	dlURL := fmt.Sprintf("%s%s/%s/%s/download/%s/%s", svc.providersV1, ref.Namespace, ref.Type, ref.Version, platform.OS, platform.Arch)
	dl, err := fetchJSON[registryDownload](ctx, fetch, dlURL)
	if err != nil {
		return proxy.Meta{}, false, err
	}
	if dl.DownloadURL == "" {
		return proxy.Meta{}, false, fmt.Errorf("%w: no download_url for %s %s %s", ErrInvalidProviderPath, ref.Source(), ref.Version, platform)
	}

	filename := dl.Filename
	if filename == "" {
		filename = fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", ref.Type, ref.Version, platform.OS, platform.Arch)
	}
	key := providerArchiveKey(ref.Host, ref.Namespace, ref.Type, filename)

	var (
		meta   proxy.Meta
		cached bool
	)
	lockErr := store.Lock(key).WithLock(func() error {
		if existing, ok, serr := store.Stat(key); serr == nil && ok {
			meta, cached = existing, true
			return nil
		}
		committed, cerr := m.fetchAndCommit(ctx, store, fetch, key, dl)
		if cerr != nil {
			return cerr
		}
		meta = committed
		return nil
	})
	if lockErr != nil {
		return proxy.Meta{}, false, lockErr
	}
	return meta, cached, nil
}

// fetchAndCommit downloads the provider zip and commits it to the store, verifying
// the zh: hash. It is called under the per-key lock held by DownloadInto.
func (m *ProviderMirror) fetchAndCommit(ctx context.Context, store proxy.Store, fetch proxy.Fetcher, key string, dl registryDownload) (proxy.Meta, error) {
	resp, err := fetch(ctx, proxy.UpstreamRequest{URL: dl.DownloadURL})
	if err != nil {
		return proxy.Meta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return proxy.Meta{}, fmt.Errorf("%w: %d for %s", ErrUpstreamStatus, resp.StatusCode, dl.DownloadURL)
	}
	return store.Commit(ctx, proxy.CommitRequest{
		Key:         key,
		Data:        resp.Body,
		Kind:        proxy.KindArtifact,
		ContentType: contentTypeZip,
		Verify:      zhVerify(dl.Shasum),
	})
}
