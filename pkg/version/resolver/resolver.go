// Package resolver defines the datasource resolver registry for the Atmos
// Version Tracker. A resolver lists candidate versions for a package from a
// concrete datasource (GitHub tags/releases, OCI registries, the Atmos
// toolchain, ...) and can pin a version to an immutable digest.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// ErrResolverUnsupported is returned when a datasource has no registered resolver.
	ErrResolverUnsupported = errors.New("version resolver unsupported")
	// ErrNoVersionMatch is returned when no candidate satisfies the desired expression.
	ErrNoVersionMatch = errors.New("no version satisfies constraint")
	// ErrPinUnsupported is returned when a datasource has no immutable digest concept.
	ErrPinUnsupported = errors.New("datasource does not support digest pinning")
	// ErrDuplicateResolver is returned when two resolvers register the same datasource name.
	ErrDuplicateResolver = errors.New("duplicate resolver registration")
	// ErrVersionListingUnsupported is returned when a datasource cannot enumerate versions.
	ErrVersionListingUnsupported = errors.New("datasource cannot list versions")
)

// Candidate is one version a datasource offers for a package.
type Candidate struct {
	// Version is the version string as published by the datasource.
	Version string
	// Digest is the immutable identifier for the version when known up front
	// (git commit SHA for tags, sha256 digest for OCI images).
	Digest string
	// ReleasedAt is the upstream release timestamp when the datasource
	// provides one; nil otherwise. Used for update cooldown checks.
	ReleasedAt *time.Time
	// Prerelease marks versions the datasource labels as prereleases.
	Prerelease bool
}

// Request identifies the package a resolver operates on.
type Request struct {
	// Package is the datasource-specific package coordinate (e.g. owner/repo).
	Package string
	// Datasource is the canonical datasource key (e.g. github-tags).
	Datasource string
	// Provider carries backend endpoint/auth configuration from version.providers.
	Provider schema.VersionProvider
	// Config is the Atmos configuration.
	Config *schema.AtmosConfiguration
}

// Resolver lists and pins versions for one or more datasources.
type Resolver interface {
	// Names returns the datasource names this resolver serves.
	Names() []string
	// Versions returns candidate versions for the requested package.
	Versions(ctx context.Context, req *Request) ([]Candidate, error)
	// Pin resolves a version to its immutable digest. Implementations return
	// ErrPinUnsupported when the datasource has no digest concept.
	Pin(ctx context.Context, req *Request, version string) (string, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Resolver{}
)

// datasourceAliases maps ecosystem-style names to canonical datasource names
// so entries can rely on the ecosystem when the datasource is omitted.
var datasourceAliases = map[string]string{
	"github":         "github-tags",
	"github/actions": "github-tags",
	"github-actions": "github-tags",
	"docker":         "docker-tags",
	"oci":            "oci-tags",
}

// Register adds a resolver under each of its datasource names. It panics on a
// duplicate name: registration happens in init() and a duplicate is a
// programming error.
func Register(r Resolver) {
	defer perf.Track(nil, "resolver.Register")()

	registryMu.Lock()
	defer registryMu.Unlock()
	for _, name := range r.Names() {
		if _, exists := registry[name]; exists {
			panic(fmt.Errorf("%w: %s", ErrDuplicateResolver, name))
		}
		registry[name] = r
	}
}

// Lookup returns the resolver serving the datasource along with the canonical
// datasource name, resolving ecosystem aliases (e.g. github -> github-tags).
func Lookup(datasource string) (Resolver, string, bool) {
	defer perf.Track(nil, "resolver.Lookup")()

	registryMu.RLock()
	defer registryMu.RUnlock()
	if r, ok := registry[datasource]; ok {
		return r, datasource, true
	}
	if alias, ok := datasourceAliases[datasource]; ok {
		if r, ok := registry[alias]; ok {
			return r, alias, true
		}
	}
	return nil, "", false
}

// Names returns the sorted registered datasource names.
func Names() []string {
	defer perf.Track(nil, "resolver.Names")()

	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
