package ci

import (
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/cache"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	providersMu sync.RWMutex
	providers   = make(map[string]provider.Provider)
)

// Register registers a CI provider.
// Providers should call this in their init() function.
func Register(p provider.Provider) {
	defer perf.Track(nil, "provider.Register")()

	providersMu.Lock()
	defer providersMu.Unlock()
	providers[p.Name()] = p
}

// Get returns a provider by name.
func Get(name string) (provider.Provider, error) {
	defer perf.Track(nil, "provider.Get")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	p, ok := providers[name]
	if !ok {
		return nil, errUtils.ErrCIProviderNotFound
	}
	return p, nil
}

// Detect returns a provider that detects it is active in the current environment.
func Detect() provider.Provider {
	defer perf.Track(nil, "provider.Detect")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	for _, p := range providers {
		if p.Detect() {
			log.Debug("CI provider detected", "provider", p.Name())
			return p
		} else {
			log.Debug("CI provider not detected", "provider", p.Name())
		}
	}
	return nil
}

// DetectOrError returns the detected provider or an error if none is detected.
func DetectOrError() (provider.Provider, error) {
	defer perf.Track(nil, "provider.DetectOrError")()

	p := Detect()
	if p == nil {
		return nil, errUtils.ErrCIProviderNotDetected
	}
	return p, nil
}

// List returns all registered provider names.
func List() []string {
	defer perf.Track(nil, "provider.List")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

// Reset clears all registered providers. Intended for use in tests to ensure
// clean state between subtests that register providers.
func Reset() {
	defer perf.Track(nil, "ci.Reset")()

	providersMu.Lock()
	defer providersMu.Unlock()
	providers = make(map[string]provider.Provider)
}

// SwapRegistryForTest clears the provider registry and returns a restore function
// that puts the previous registry back in place when invoked. Intended for use in
// tests (including tests in other packages) that need to isolate themselves from
// the set of CI providers that are auto-registered via init() in the hosting
// binary — for example, tests asserting that ci.IsCI() returns false even when
// running under GitHub Actions.
func SwapRegistryForTest() func() {
	defer perf.Track(nil, "ci.SwapRegistryForTest")()

	providersMu.Lock()
	prev := providers
	providers = make(map[string]provider.Provider)
	providersMu.Unlock()

	return func() {
		providersMu.Lock()
		providers = prev
		providersMu.Unlock()
	}
}

// IsCI returns true if any CI provider is detected.
func IsCI() bool {
	defer perf.Track(nil, "provider.IsCI")()

	return Detect() != nil
}

// DebugModeInfo describes the detected CI provider's debug-mode state.
// Returned by DetectDebugMode to keep callers provider-agnostic.
type DebugModeInfo struct {
	// Active is true when the detected provider reports that CI debug
	// logging is enabled for the current run.
	Active bool

	// Provider is the name of the detected CI provider (e.g.
	// "github-actions"). Empty when no CI provider is detected.
	Provider string
}

// DetectDebugMode inspects the active CI provider for a debug-mode signal.
// Returns a zero-value DebugModeInfo when no provider is detected or the
// detected provider does not implement provider.DebugModeDetector.
//
// Callers (e.g., the CLI startup path) use this to auto-promote their own
// log level when the user has opted into CI-side debug logging, without
// hard-coding any provider-specific environment variables.
func DetectDebugMode() DebugModeInfo {
	defer perf.Track(nil, "ci.DetectDebugMode")()

	p := Detect()
	if p == nil {
		return DebugModeInfo{}
	}
	info := DebugModeInfo{Provider: p.Name()}
	if d, ok := p.(provider.DebugModeDetector); ok {
		info.Active = d.IsDebugMode()
	}
	return info
}

// DetectCache returns the cache backend exposed by the active CI provider.
// It returns errUtils.ErrCacheUnavailable when no CI provider is detected or the
// detected provider does not implement the cache capability. This keeps callers
// (CLI subcommands and lifecycle hooks) provider-agnostic.
//
// DetectCache is the in-runner path: it requires the provider to be actively
// detected (e.g. GITHUB_ACTIONS=true), so the automatic restore/save lifecycle
// safely no-ops outside CI. For cache administration that should work locally,
// use ResolveAdminCache.
func DetectCache() (cache.Backend, error) {
	defer perf.Track(nil, "ci.DetectCache")()

	p := Detect()
	if p == nil {
		return nil, errUtils.ErrCacheUnavailable
	}
	cp, ok := p.(provider.CacheProvider)
	if !ok {
		return nil, errUtils.ErrCacheUnavailable
	}
	return cp.Cache()
}

// ResolveAdminCache returns a cache backend for administering the cache (list and
// delete) without requiring an active CI runtime. Cache administration uses the
// provider's public API and a token, so it must work locally — outside a runner —
// which DetectCache deliberately does not allow.
//
// It prefers the actively-detected provider (when running inside CI) and
// otherwise falls back to any registered cache-capable provider so a repo admin
// can manage the cache from their workstation. The resulting backend's
// save/restore may still be unavailable outside a runner; that is enforced by the
// backend itself (see the github backend's Save/Restore).
func ResolveAdminCache() (cache.Backend, error) {
	defer perf.Track(nil, "ci.ResolveAdminCache")()

	if p := Detect(); p != nil {
		if cp, ok := p.(provider.CacheProvider); ok {
			return cp.Cache()
		}
	}

	providersMu.RLock()
	defer providersMu.RUnlock()
	for _, p := range providers {
		cp, ok := p.(provider.CacheProvider)
		if !ok {
			continue
		}
		log.Debug("CI cache: resolved cache-capable provider for administration", "provider", p.Name())
		return cp.Cache()
	}
	return nil, errUtils.ErrCacheUnavailable
}
