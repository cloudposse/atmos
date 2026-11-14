// Package roundtriptest publishes artifacts to an in-process OCI registry so that tests of the
// existing consumers (component source provisioning, scaffold generation, and vendoring) can prove
// a published package works through them. It composes the real artifact service and records every
// registry request, so a test can also prove which blobs a consumer actually fetched.
//
// It is test support: it depends on the testing package and must not be imported by production code.
package roundtriptest

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Alias is the repository alias of the registry published to.
const Alias = "registry"

// Prefix is the repository path prefix below the registry host.
const Prefix = "acme/atmos"

// Registry is an in-process registry with a composed artifact service publishing to it.
type Registry struct {
	// Host is the "127.0.0.1:PORT" registry address.
	Host string

	// Service is the real artifact service bound to the registry.
	Service artifact.Service

	mu       sync.Mutex
	requests []string
}

// anonymous resolves every registry anonymously, never reading the developer's Docker configuration.
type anonymous struct{}

func (anonymous) Resolve(authn.Resource) (authn.Authenticator, error) {
	defer perf.Track(nil, "roundtriptest.anonymous.Resolve")()

	return authn.Anonymous, nil
}

// New starts the registry and composes the artifact service with an oci/registry repository named
// [Alias] bound to it. Extra compose options apply after the defaults.
func New(t *testing.T, opts ...compose.Option) *Registry {
	defer perf.Track(nil, "roundtriptest.New")()

	t.Helper()
	reg := &Registry{}
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)), registry.WithReferrersSupport(true))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reg.mu.Lock()
		reg.requests = append(reg.requests, r.Method+" "+r.URL.Path)
		reg.mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	reg.Host = strings.TrimPrefix(server.URL, "http://")

	config := &schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		Alias: {Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": reg.Host, "prefix": Prefix}},
	}}}
	factory := func(context.Context, string, *schema.ArtifactRepositoryConfig, artifact.IdentityResolver) (oci.Transport, error) {
		return oci.NewClient(nil, oci.WithKeychain(anonymous{})), nil
	}
	composition, err := compose.New(config, append([]compose.Option{compose.WithTransportFactory(factory)}, opts...)...)
	require.NoError(t, err)
	reg.Service = composition.Service
	return reg
}

// Publish publishes request to the registry. The repository is always [Alias].
func (r *Registry) Publish(t *testing.T, request *artifact.PublishRequest) *artifact.OperationResult {
	defer perf.Track(nil, "roundtriptest.Registry.Publish")()

	t.Helper()
	request.Repository = Alias
	result, err := r.Service.Publish(context.Background(), *request)
	require.NoError(t, err)
	return result
}

// Image returns the OCI image reference (without a scheme) of a published name and version.
func (r *Registry) Image(name, version string) string {
	defer perf.Track(nil, "roundtriptest.Registry.Image")()

	return r.Host + "/" + Prefix + "/" + name + ":" + version
}

// URI returns the oci:// source URI of a published name and version, as consumers declare it.
func (r *Registry) URI(name, version string) string {
	defer perf.Track(nil, "roundtriptest.Registry.URI")()

	return "oci://" + r.Image(name, version)
}

// Requests returns the "METHOD path" of every request the registry served so far.
func (r *Registry) Requests() []string {
	defer perf.Track(nil, "roundtriptest.Registry.Requests")()

	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// ResetRequests forgets the recorded requests, so a test can observe one consumer in isolation.
func (r *Registry) ResetRequests() {
	defer perf.Track(nil, "roundtriptest.Registry.ResetRequests")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
}
