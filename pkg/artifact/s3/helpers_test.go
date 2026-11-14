package s3

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	blobs3 "github.com/cloudposse/atmos/pkg/artifact/blob/s3"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields these tests configure.
var _ = schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindS3, Packaging: "tgz", Identity: "id", Options: nil}

const (
	testName    = "team/app"
	testVersion = "1.0.0"

	// A part size that lets tests exercise multipart uploads with kilobytes.
	smallPart = 16 << 10
)

// errCrash is the error the fault hooks simulate a crash with.
var errCrash = errors.New("simulated crash")

// capturedAt is the fixed preparation time of the test artifacts.
var capturedAt = time.Date(2026, 10, 30, 23, 59, 59, 0, time.UTC)

// rspec configures a repository under test.
type rspec struct {
	packaging  artifact.Packaging
	prefix     string
	tags       map[string]string
	keyPattern string
	opts       []Option
	fake       *s3test.Server
	storeOpts  []blobs3.Option
	probe      bool
}

func (s *rspec) clone() *rspec {
	c := *s
	c.opts = append([]Option(nil), s.opts...)
	c.storeOpts = append([]blobs3.Option(nil), s.storeOpts...)
	return &c
}

func (s *rspec) with(fn func(*rspec)) *rspec {
	c := s.clone()
	fn(c)
	return c
}

// fresh returns a copy of the spec bound to a new fake S3 endpoint, so the test can inspect it.
func (s *rspec) fresh(t *testing.T) *rspec {
	t.Helper()
	return s.with(func(c *rspec) { c.fake = s3test.NewServer(t) })
}

func (s *rspec) withOpts(opts ...Option) *rspec {
	return s.with(func(c *rspec) { c.opts = append(c.opts, opts...) })
}

func (s *rspec) config(bucket string) *schema.ArtifactRepositoryConfig {
	options := map[string]any{"bucket": bucket}
	if s.prefix != "" {
		options["prefix"] = s.prefix
	}
	if s.keyPattern != "" {
		options["key_pattern"] = s.keyPattern
	}
	if len(s.tags) > 0 {
		tags := map[string]any{}
		for k, v := range s.tags {
			tags[k] = v
		}
		options["tags"] = tags
	}
	cfg := &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindS3, Options: options}
	if s.packaging != "" {
		cfg.Packaging = string(s.packaging)
	}
	return cfg
}

// newStoreFor builds the blob store the repository runs over, rooted at the spec's prefix.
func newStoreFor(t *testing.T, spec *rspec) *blobs3.Store {
	t.Helper()
	opts := []blobs3.Option{blobs3.WithPartSize(smallPart), blobs3.WithPrefix(spec.prefix)}
	if spec.probe {
		opts = append(opts, blobs3.WithConditionalMode(blobs3.ConditionalProbe))
	}
	opts = append(opts, spec.storeOpts...)
	store, err := blobs3.NewFromClient(spec.fake.Client(), spec.fake.Bucket(), opts...)
	require.NoError(t, err)
	return store
}

// newRepo builds a repository over a fresh fake unless the spec names one. Lock polling is fast so
// contention tests stay quick.
func newRepo(t *testing.T, spec *rspec) *Repository {
	t.Helper()
	spec = spec.clone()
	if spec.fake == nil {
		spec.fake = s3test.NewServer(t)
	}
	opts := append([]Option{WithStore(newStoreFor(t, spec)), withLockPoll(5 * time.Millisecond)}, spec.opts...)
	repo, err := newRepository("test", spec.config(spec.fake.Bucket()), opts, &artifact.RepositoryOptions{})
	require.NoError(t, err)
	return repo
}

func withLockPoll(poll time.Duration) Option {
	return func(s *settings) { s.lockPoll = poll }
}

func withFaults(hook faultHook) Option {
	return func(s *settings) { s.faults = hook }
}

// crashAt returns a fault hook that fails once at the point.
func crashAt(point faultPoint) faultHook {
	fired := false
	return func(p faultPoint) error {
		if p == point && !fired {
			fired = true
			return errCrash
		}
		return nil
	}
}

// armedFault is a fault hook that does nothing until a test arms it.
type armedFault struct{ hook faultHook }

func (a *armedFault) option() Option {
	return withFaults(func(point faultPoint) error {
		if a.hook == nil {
			return nil
		}
		return a.hook(point)
	})
}

func noneSpec() *rspec { return &rspec{packaging: artifact.PackagingNone} }

func archiveSpec(packaging artifact.Packaging) *rspec { return &rspec{packaging: packaging} }

var allSpecs = []*rspec{noneSpec(), archiveSpec(artifact.PackagingTar), archiveSpec(artifact.PackagingTgz), archiveSpec(artifact.PackagingZip)}

func specName(spec *rspec) string { return string(spec.packaging) }

// prepareSpec describes the artifact a test prepares with the production files producer.
type prepareSpec struct {
	name      string
	version   string
	packaging artifact.Packaging
	files     map[string]string
	at        time.Time
	opID      string
	labels    map[string]string
	source    string
}

func (p *prepareSpec) withDefaults(packaging artifact.Packaging) *prepareSpec {
	c := *p
	if c.name == "" {
		c.name = testName
	}
	if c.version == "" {
		c.version = testVersion
	}
	if c.packaging == "" {
		c.packaging = packaging
	}
	if c.at.IsZero() {
		c.at = capturedAt
	}
	if c.opID == "" {
		c.opID = "op-" + c.name + "-" + c.version
	}
	if c.files == nil {
		c.files = map[string]string{"data.txt": "hello\n"}
	}
	return &c
}

func prepare(t *testing.T, repo *Repository, spec *prepareSpec) *artifact.PreparedArtifact {
	t.Helper()
	spec = spec.withDefaults(repo.packaging)
	src := spec.source
	if src == "" {
		src = t.TempDir()
		for name, content := range spec.files {
			fixtures.WriteFile(t, src, name, content, 0o644)
		}
	}
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: spec.name, Version: spec.version, Packaging: spec.packaging,
		Metadata:    artifact.Metadata{Labels: spec.labels},
		OperationID: spec.opID, PreparedAt: spec.at,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	return prepared
}

// publish prepares and publishes, failing the test on error.
func publish(t *testing.T, repo *Repository, spec *prepareSpec, options artifact.PublishOptions) *artifact.PublishResult {
	t.Helper()
	result, err := repo.Publish(context.Background(), prepare(t, repo, spec), options)
	require.NoError(t, err)
	return result
}

func ref(name, version string) artifact.Ref { return artifact.Ref{Name: name, Version: version} }

func defaultRef() artifact.Ref { return ref(testName, testVersion) }

func resolve(t *testing.T, repo *Repository, r artifact.Ref) artifact.PinnedReference {
	t.Helper()
	pinned, err := repo.Resolve(context.Background(), r)
	require.NoError(t, err)
	return pinned
}

// readAll drains every stream of the content, returning the first read error.
func readAll(t *testing.T, content *artifact.Content) error {
	t.Helper()
	for index := range content.Streams {
		if _, err := io.Copy(io.Discard, content.Streams[index].Reader); err != nil {
			return err
		}
	}
	return nil
}

// assertIntact reads the artifact end to end and checks that every stream verifies.
func assertIntact(t *testing.T, repo *Repository, r artifact.Ref) {
	t.Helper()
	pinned := resolve(t, repo, r)
	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err)
	defer content.Close()
	require.NoError(t, readAll(t, content))
}

func listNames(t *testing.T, repo *Repository) []string {
	t.Helper()
	page, err := repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	names := make([]string, 0, len(page.Entries))
	for index := range page.Entries {
		names = append(names, page.Entries[index].Name+":"+page.Entries[index].Version)
	}
	return names
}

// bucketKeys lists the keys of the fake bucket, sorted.
func bucketKeys(fake *s3test.Server) []string {
	keys := fake.Keys()
	sort.Strings(keys)
	return keys
}

// payloadKeys lists the bucket keys outside the managed namespace.
func payloadKeys(fake *s3test.Server, prefix string) []string {
	var out []string
	for _, key := range bucketKeys(fake) {
		rel := strings.TrimPrefix(strings.TrimPrefix(key, prefix), "/")
		if !layout.InManagedNamespace(rel) {
			out = append(out, rel)
		}
	}
	return out
}

// managedKeys lists the bucket keys below a managed directory, relative to the prefix.
func managedKeys(fake *s3test.Server, prefix, dir string) []string {
	var out []string
	for _, key := range bucketKeys(fake) {
		rel := strings.TrimPrefix(strings.TrimPrefix(key, prefix), "/")
		if strings.HasPrefix(rel, dir+"/") {
			out = append(out, rel)
		}
	}
	return out
}

func newSigner() *artifacttest.FakeSigner {
	return &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
}

// storeRequests returns the recorded requests that mutate objects, in arrival order.
func mutations(fake *s3test.Server) []s3test.Request {
	var out []s3test.Request
	for _, request := range fake.Requests() {
		switch request.Op {
		case s3test.OpPutObject, s3test.OpDeleteObject, s3test.OpCompleteMultipartUpload:
			out = append(out, request)
		}
	}
	return out
}

var _ blob.Store = (*blobs3.Store)(nil)

// hookStore decorates a store so a test can run code around writes: before delegating a Put (to
// stage a concurrent writer) or after it (to lose the response).
type hookStore struct {
	blob.Store
	beforePut func(key string, opts blob.PutOptions)
	afterPut  func(key string, info *blob.ObjectInfo, err error) (*blob.ObjectInfo, error)
}

func (h *hookStore) Put(ctx context.Context, key string, body io.Reader, opts blob.PutOptions) (*blob.ObjectInfo, error) {
	if h.beforePut != nil {
		h.beforePut(key, opts)
	}
	info, err := h.Store.Put(ctx, key, body, opts)
	if h.afterPut != nil {
		return h.afterPut(key, info, err)
	}
	return info, err
}

// refKey is the bucket key of the default artifact's reference object.
func refKey(prefix string) string {
	key := layout.ReferencePath(layout.ReferenceKey(testName, testVersion))
	if prefix == "" {
		return key
	}
	return prefix + "/" + key
}

// dummyLock is a lock handle that lets a test call Publish without taking the reference lock.
type dummyLock struct{}

func (dummyLock) Resource() string                           { return "dummy" }
func (dummyLock) Owner() string                              { return "test" }
func (dummyLock) Scope() artifact.LockScope                  { return artifact.LockScopeRepository }
func (dummyLock) Token() string                              { return "dummy-token" }
func (dummyLock) ExpiresAt() (time.Time, bool)               { return time.Time{}, false }
func (dummyLock) Renew(context.Context, time.Duration) error { return nil }
func (dummyLock) Release(context.Context) error              { return nil }

// recordSubject builds the signature subject of a pinned artifact's canonical Record.
func recordSubject(t *testing.T, repo *Repository, pinned *artifact.PinnedReference) artifact.SignatureSubject {
	t.Helper()
	inspection, err := repo.Inspect(context.Background(), *pinned)
	require.NoError(t, err)
	canonical, err := inspection.Record.Canonical()
	require.NoError(t, err)
	return artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromBytes(canonical), MediaType: artifact.MediaTypeConfig, Size: int64(len(canonical))}
}

// incompressible returns n deterministic pseudo-random bytes as a string, so archives of it stay
// large enough to need multipart uploads however they are compressed.
func incompressible(n int) string {
	data := make([]byte, n)
	_, _ = rand.New(rand.NewSource(1)).Read(data)
	return string(data)
}

// packageSpec prepares the shared "File package" fixture: nested files, an executable, an empty
// file and directory, a confined symlink, a Unicode name, and distinct README and CHANGELOG.
func packageSpec(t *testing.T) (*prepareSpec, fixtures.FilePackage) {
	t.Helper()
	src := t.TempDir()
	pkg := fixtures.WriteFilePackage(t, src)
	return &prepareSpec{source: src}, pkg
}

// refKeyOnly is the hashed reference key of a name and version.
func refKeyOnly(name, version string) string { return layout.ReferenceKey(name, version) }
