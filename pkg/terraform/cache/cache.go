// Package cache is the lifecycle facade for the Terraform registry cache. It is
// the only package internal/exec touches: it resolves the cache root, builds the
// storage backend, starts the caching proxy with the provider and module registry
// mirrors, contributes the network_mirror/host directives to the generated
// Terraform CLI config, and prints a per-run savings report on shutdown.
//
// Import as tfcache to avoid colliding with pkg/cache.
package cache

import (
	"context"
	"fmt"
	"time"

	httppkg "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/http/proxy"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/registry"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	defaultMetadataTTL          = 24 * time.Hour
	defaultStaleWhileRevalidate = 168 * time.Hour
)

// publicModuleHosts are the registry hosts whose modules.v1 service the cache
// overrides so module registry traffic routes through the proxy. Provider caching
// needs no host list — the network mirror covers every provider host via the
// protocol path. Private module registries can be added in a follow-up.
var publicModuleHosts = []string{"registry.terraform.io", "registry.opentofu.org"}

// Setup is a started registry cache. Close shuts it down and reports savings.
type Setup struct {
	server   *proxy.Server
	proxyURL string
	certPath string
}

// CertPath returns the on-disk path of the proxy's TLS certificate (PEM). Callers
// use it to wire trust into the terraform/tofu subprocess. Empty on a nil Setup.
func (s *Setup) CertPath() string {
	defer perf.Track(nil, "cache.Setup.CertPath")()

	if s == nil {
		return ""
	}
	return s.certPath
}

// TrustEnv returns environment variables that make the terraform/tofu subprocess
// trust the proxy's certificate (SSL_CERT_FILE bundle). Honored on Linux/BSD; on
// macOS/Windows trust requires installing the cert in the OS trust store. Returns
// nil on a nil Setup.
func (s *Setup) TrustEnv() ([]string, error) {
	defer perf.Track(nil, "cache.Setup.TrustEnv")()

	if s == nil || s.certPath == "" {
		return nil, nil
	}
	return buildTrustBundle(s.certPath)
}

// Start resolves the cache root, builds the backend and mirrors, and starts the
// proxy. It returns (nil, nil) when caching is disabled so callers can no-op.
func Start(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (*Setup, error) {
	defer perf.Track(atmosConfig, "tfcache.Start")()

	cfg := atmosConfig.Components.Terraform.Cache
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	root, err := resolveRoot(cfg)
	if err != nil {
		return nil, err
	}
	if err := ensureLayout(root); err != nil {
		return nil, err
	}

	// Terraform/OpenTofu require provider network mirrors to be https, so the proxy
	// serves TLS with a cached self-signed loopback certificate.
	cert, err := ensureProxyCert(root)
	if err != nil {
		return nil, err
	}
	if cert.Generated {
		ui.Success(fmt.Sprintf("Generated Terraform registry cache certificate (%s)", cert.CertPEMPath))
	}

	client := httppkg.NewDefaultClient(httppkg.WithGitHubToken(httppkg.GetGitHubTokenFromEnv()))
	store := proxy.NewFileStore(root)
	mirrors := []proxy.Mirror{
		registry.NewProviderMirror(client),
		registry.NewModuleMirror(),
	}

	server := proxy.NewServer(proxy.Options{
		Mirrors:              mirrors,
		Store:                store,
		Client:               client,
		MetadataTTL:          parseDuration(cfg.MetadataTTL, defaultMetadataTTL),
		StaleWhileRevalidate: parseDuration(cfg.StaleWhileRevalidate, defaultStaleWhileRevalidate),
		TLSCertificate:       &cert.Certificate,
	})

	proxyURL, err := server.Start(ctx)
	if err != nil {
		return nil, err
	}

	log.Debug("Terraform registry cache started", "proxy", proxyURL, "root", root)
	ui.Success(fmt.Sprintf("Registry cache proxy listening on %s", proxyURL))
	return &Setup{server: server, proxyURL: proxyURL, certPath: cert.CertPEMPath}, nil
}

// Contribute returns the Terraform CLI-config directives the cache injects into the
// generated RC: a provider_installation network_mirror (covering all provider
// hosts) plus a direct{} fallback, and a host override for each public registry that
// routes modules.v1 through the proxy while preserving the upstream providers.v1.
//
// A host block REPLACES the host's whole service-discovery document, so it must also
// declare providers.v1 — otherwise a direct provider lookup (e.g. a mirror miss)
// reports "host does not offer a provider registry".
func (s *Setup) Contribute() map[string]any {
	defer perf.Track(nil, "tfcache.Setup.Contribute")()

	if s == nil {
		return nil
	}

	hosts := map[string]any{}
	for _, host := range publicModuleHosts {
		hosts[host] = map[string]any{
			"services": map[string]any{
				"modules.v1":   s.proxyURL + "modules/" + host + "/",
				"providers.v1": "https://" + host + "/v1/providers/",
			},
		}
	}

	return map[string]any{
		"provider_installation": []any{
			map[string]any{"network_mirror": map[string]any{"url": s.proxyURL + "providers/"}},
			map[string]any{"direct": map[string]any{}},
		},
		"host": hosts,
	}
}

// Close shuts the proxy down and prints the savings report when bytes were served
// from cache. Safe to call on a nil Setup.
func (s *Setup) Close(ctx context.Context) error {
	defer perf.Track(nil, "tfcache.Setup.Close")()

	if s == nil {
		return nil
	}

	printSavingsReport(s.server.Stats())
	return s.server.Shutdown(ctx)
}

// parseDuration parses a Go duration string, falling back to def on empty/invalid.
func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Debug("Invalid Terraform cache duration; using default", "value", s, "default", def)
		return def
	}
	return d
}
