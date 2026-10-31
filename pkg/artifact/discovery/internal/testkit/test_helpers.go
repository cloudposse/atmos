// Package testkit provides in-process registries and publication helpers for discovery and
// catalog tests: an OCI registry that counts every request, repositories bound to it, and a
// deterministic way to publish small packages.
package testkit

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/oci"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Prefix is the collection prefix of every test repository.
const Prefix = "acme/atmos"

// Owner is the lock owner of test repositories.
const Owner = "test-owner"

const filePerm = 0o644

// PreparedAt is the fixed preparation time of published packages.
var PreparedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// recorder counts the HTTP requests a registry client makes, by method and path.
type recorder struct {
	mu        sync.Mutex
	requests  []string
	fail      error
	failAfter int
}

func (r *recorder) RoundTrip(request *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.requests = append(r.requests, request.Method+" "+request.URL.Path)
	fail := r.fail
	if fail != nil && len(r.requests) <= r.failAfter {
		fail = nil
	}
	r.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	return http.DefaultTransport.RoundTrip(request)
}

// Registry is one in-process OCI registry with a request-counting client.
type Registry struct {
	// Host is the registry address ("127.0.0.1:PORT").
	Host string

	// Transport is the recording registry client.
	Transport atmosoci.Transport

	server   *ocitest.Server
	recorder *recorder
}

// NewRegistry starts a registry. A non-nil credential makes the registry require it; the client
// then presents it, like a repository with a bound identity.
func NewRegistry(t *testing.T) *Registry {
	t.Helper()
	server := ocitest.NewServerWithOptions(t, nil, ocitest.WithTagsFollowManifestDelete())
	rec := &recorder{}
	client := atmosoci.NewClient(nil, atmosoci.WithTransport(rec))
	return &Registry{Host: server.Host, Transport: client, server: server, recorder: rec}
}

// Reset forgets the recorded requests.
func (r *Registry) Reset() {
	r.recorder.mu.Lock()
	defer r.recorder.mu.Unlock()
	r.recorder.requests = nil
}

// Total returns how many requests were recorded since the last Reset.
func (r *Registry) Total() int {
	r.recorder.mu.Lock()
	defer r.recorder.mu.Unlock()
	return len(r.recorder.requests)
}

// Count returns how many recorded requests used the method and contain the path fragment.
func (r *Registry) Count(method, fragment string) int {
	r.recorder.mu.Lock()
	defer r.recorder.mu.Unlock()
	total := 0
	for _, request := range r.recorder.requests {
		if strings.HasPrefix(request, method+" ") && strings.Contains(request, fragment) {
			total++
		}
	}
	return total
}

// Requests returns a copy of the recorded requests.
func (r *Registry) Requests() []string {
	r.recorder.mu.Lock()
	defer r.recorder.mu.Unlock()
	return append([]string(nil), r.recorder.requests...)
}

// FailAll makes every later request fail, like a client with no network; nil restores it.
func (r *Registry) FailAll(err error) {
	r.FailAfter(0, err)
}

// FailAfter lets the next n requests through, then fails every later one; nil restores the client.
// It simulates a refresh that is interrupted part way through.
func (r *Registry) FailAfter(n int, err error) {
	r.recorder.mu.Lock()
	defer r.recorder.mu.Unlock()
	r.recorder.fail, r.recorder.failAfter = err, len(r.recorder.requests)+n
}

// Config returns the repository configuration of a registry-backed repository.
func (r *Registry) Config(catalog string) *schema.ArtifactRepositoryConfig {
	return &schema.ArtifactRepositoryConfig{
		Kind:    artifact.RepositoryKindOCIRegistry,
		Options: map[string]any{"registry": r.Host, "prefix": Prefix},
		Catalog: catalog,
	}
}

// Location is the credential-free repository location ("registry/prefix").
func (r *Registry) Location() string { return r.Host + "/" + Prefix }

// CatalogURI returns the catalog reference of a tag below the repository path.
func (r *Registry) CatalogURI(name, tag string) string {
	return "oci://" + r.Location() + "/" + name + ":" + tag
}

// Repo builds a repository bound to the registry. Extra options follow the defaults.
func (r *Registry) Repo(t *testing.T, alias string, catalog string, extra ...oci.Option) *oci.Repository {
	t.Helper()
	options := append([]oci.Option{oci.WithTransport(r.Transport), oci.WithOwner(Owner), oci.WithLockDir(t.TempDir())}, extra...)
	repo, err := oci.NewRepository(context.Background(), alias, r.Config(catalog), options...)
	require.NoError(t, err)
	return repo
}

// Info returns the service description of a registry-backed repository.
func (r *Registry) Info(repo artifact.Repository, catalog string) service.RepositoryInfo {
	return service.RepositoryInfo{
		Capabilities: repo.Capabilities(),
		Discovery: service.DiscoveryInfo{
			Kind: artifact.RepositoryKindOCIRegistry, Location: r.Location(), Catalog: catalog,
			Options: map[string]any{"registry": r.Host, "prefix": Prefix},
		},
	}
}

// Spec describes one package to publish.
type Spec struct {
	Name    string
	Version string
	Content string
	Labels  map[string]string
	NoDocs  bool

	// Packaging defaults to tgz, which registries and archives store; local directories need none.
	Packaging artifact.Packaging
}

// Prepare builds a deterministic prepared artifact.
func Prepare(t *testing.T, spec *Spec) *artifact.PreparedArtifact {
	t.Helper()
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", spec.Content, filePerm)
	if !spec.NoDocs {
		fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "readme for "+spec.Name+"\n", filePerm)
	}
	packaging := spec.Packaging
	if packaging == "" {
		packaging = artifact.PackagingTgz
	}
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: spec.Name, Version: spec.Version, Packaging: packaging,
		Metadata:    artifact.Metadata{Description: "package " + spec.Name, Labels: spec.Labels},
		OperationID: "op-" + spec.Name + "-" + spec.Version, PreparedAt: PreparedAt,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	return prepared
}

// Publish publishes a package to a repository and returns the result.
//
//nolint:gocritic // Test helper; the spec is a small literal.
func Publish(t *testing.T, repo artifact.Repository, spec Spec) *artifact.PublishResult {
	t.Helper()
	result, err := repo.Publish(context.Background(), Prepare(t, &spec), artifact.PublishOptions{})
	require.NoError(t, err)
	return result
}

// LocalDir returns a fresh directory for a local repository.
func LocalDir(t *testing.T) string { return filepath.Join(t.TempDir(), "repo") }
