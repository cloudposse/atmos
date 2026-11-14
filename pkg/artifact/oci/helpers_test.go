//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Fixed values shared by the tests.
const (
	testPrefix  = "acme/atmos"
	filePerm    = 0o644
	execPerm    = 0o755
	testOwner   = "test-owner"
	appName     = "team/app"
	appVersion  = "1.0.0"
	signerID    = "release@example.com"
	signerIssue = "https://issuer.example.com"
)

var preparedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// recorder counts the HTTP requests a client makes, by method and path.
type recorder struct {
	mu       sync.Mutex
	requests []string
	fail     error
}

func (r *recorder) RoundTrip(request *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.requests = append(r.requests, request.Method+" "+request.URL.Path)
	fail := r.fail
	r.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	return http.DefaultTransport.RoundTrip(request)
}

// reset forgets the recorded requests.
func (r *recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
}

// count returns how many recorded requests start with the method and contain the path fragment.
func (r *recorder) count(method, fragment string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, request := range r.requests {
		if strings.HasPrefix(request, method+" ") && strings.Contains(request, fragment) {
			total++
		}
	}
	return total
}

// total returns the number of recorded requests.
func (r *recorder) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// failAll makes every later request fail, like a transport with no network.
func (r *recorder) failAll(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fail = err
}

// registry is one in-process registry with its own credentials and recording client.
type registry struct {
	server     *ocitest.Server
	recorder   *recorder
	transport  atmosoci.Transport
	credential *ocitest.Credential
}

// auth returns the authenticator third-party clients use against the registry.
func (r *registry) auth() authn.Authenticator {
	if r.credential == nil {
		return authn.Anonymous
	}
	return &authn.Basic{Username: r.credential.Username, Password: r.credential.Password}
}

// credentialsKeychain resolves only the registries it was given, so one registry's credentials are
// never presented to another and nothing falls back to the developer's Docker configuration.
type credentialsKeychain map[string]authn.Authenticator

func (k credentialsKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	if auth, ok := k[resource.RegistryStr()]; ok {
		return auth, nil
	}
	return authn.Anonymous, nil
}

// newRegistry starts a registry, optionally behind Basic credentials.
func newRegistry(t *testing.T, credential *ocitest.Credential) *registry {
	t.Helper()
	server := ocitest.NewServerWithOptions(t, credential, ocitest.WithTagsFollowManifestDelete())
	rec := &recorder{}
	keychain := credentialsKeychain{}
	if credential != nil {
		keychain[server.Host] = &authn.Basic{Username: credential.Username, Password: credential.Password}
	}
	client := atmosoci.NewClient(nil, atmosoci.WithKeychain(keychain), atmosoci.WithTransport(rec))
	return &registry{server: server, recorder: rec, transport: client, credential: credential}
}

// options returns the repository options for a registry-backed repository.
func (r *registry) options(extra ...Option) []Option {
	return append([]Option{WithTransport(r.transport), WithOwner(testOwner)}, extra...)
}

func registryConfig(host string) *schema.ArtifactRepositoryConfig {
	return &schema.ArtifactRepositoryConfig{
		Kind:    artifact.RepositoryKindOCIRegistry,
		Options: map[string]any{"registry": host, "prefix": testPrefix},
	}
}

func layoutConfig(path string) *schema.ArtifactRepositoryConfig {
	return &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCILayout, Options: map[string]any{"path": path}}
}

// newRegistryRepo builds a registry repository against r.
func newRegistryRepo(t *testing.T, r *registry, extra ...Option) *Repository {
	t.Helper()
	opts := append(r.options(WithLockDir(t.TempDir())), extra...)
	repo, err := NewRepository(context.Background(), "registry", registryConfig(r.server.Host), opts...)
	require.NoError(t, err)
	return repo
}

// newLayoutRepo builds a layout repository in a fresh directory.
func newLayoutRepo(t *testing.T, extra ...Option) *Repository {
	t.Helper()
	return newLayoutRepoAt(t, filepath.Join(t.TempDir(), "layout"), extra...)
}

func newLayoutRepoAt(t *testing.T, path string, extra ...Option) *Repository {
	t.Helper()
	opts := append([]Option{WithOwner(testOwner), WithLockDir(t.TempDir())}, extra...)
	repo, err := NewRepository(context.Background(), "layout", layoutConfig(path), opts...)
	require.NoError(t, err)
	return repo
}

// publishSpec describes one package the tests publish.
type publishSpec struct {
	name      string
	version   string
	content   string
	packaging artifact.Packaging
	labels    map[string]string
	noDocs    bool
	at        time.Time
}

// prepare builds a deterministic prepared artifact.
func prepare(t *testing.T, spec publishSpec) *artifact.PreparedArtifact {
	t.Helper()
	if spec.name == "" {
		spec.name = appName
	}
	if spec.version == "" {
		spec.version = appVersion
	}
	if spec.packaging == "" {
		spec.packaging = artifact.PackagingTgz
	}
	if spec.at.IsZero() {
		spec.at = preparedAt
	}
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", spec.content, filePerm)
	fixtures.WriteFile(t, src, "bin/run.sh", fixtures.HelperScript, execPerm)
	if !spec.noDocs {
		fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "readme for "+spec.name+"\n", filePerm)
		fixtures.WriteFile(t, src, fixtures.ReservedDir+"/CHANGELOG.md", "changes for "+spec.name+"\n", filePerm)
	}
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: spec.name, Version: spec.version, Packaging: spec.packaging,
		Metadata:    artifact.Metadata{Description: "a test package", Labels: spec.labels},
		OperationID: "op-" + spec.name + "-" + spec.version, PreparedAt: spec.at,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	return prepared
}

// publish prepares and publishes a package, replacing nothing.
func publish(t *testing.T, repo *Repository, spec publishSpec) *artifact.PublishResult {
	t.Helper()
	result, err := repo.Publish(context.Background(), prepare(t, spec), artifact.PublishOptions{})
	require.NoError(t, err)
	return result
}

// payloadDigest returns the digest of the single payload layer of a published package.
func payloadDigest(t *testing.T, result *artifact.PublishResult) digest.Digest {
	t.Helper()
	require.Len(t, result.Representation.Payload, 1)
	return result.Representation.Payload[0].Digest
}
