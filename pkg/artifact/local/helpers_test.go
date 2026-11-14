package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields these tests configure.
var _ = schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir, Packaging: "none", Permissions: "0600"}

const (
	testName    = "team/app"
	testVersion = "1.0.0"
)

// capturedAt is the fixed preparation time of the test artifacts.
var capturedAt = time.Date(2026, 10, 30, 23, 59, 59, 0, time.UTC)

// repoSpec configures a repository under test.
type repoSpec struct {
	kind        string
	packaging   artifact.Packaging
	permissions string
	keyPattern  string
	root        string
	opts        []Option
}

func (s *repoSpec) config(t *testing.T) *schema.ArtifactRepositoryConfig {
	t.Helper()
	options := map[string]any{"path": s.root}
	if s.keyPattern != "" {
		options["key_pattern"] = s.keyPattern
	}
	cfg := &schema.ArtifactRepositoryConfig{Kind: s.kind, Options: options, Permissions: s.permissions}
	if s.packaging != "" {
		cfg.Packaging = string(s.packaging)
	}
	return cfg
}

// newRepo builds a repository under a fresh temporary root unless the spec names one.
func newRepo(t *testing.T, spec *repoSpec) *Repository {
	t.Helper()
	spec = spec.clone()
	if spec.root == "" {
		spec.root = filepath.Join(t.TempDir(), "repo")
	}
	if spec.kind == "" {
		spec.kind = artifact.RepositoryKindLocalDir
	}
	repo, err := newRepository("test", spec.kind, spec.config(t), spec.opts)
	require.NoError(t, err)
	return repo
}

func dirSpec() *repoSpec { return &repoSpec{kind: artifact.RepositoryKindLocalDir} }

func archiveSpec(packaging artifact.Packaging) *repoSpec {
	return &repoSpec{kind: artifact.RepositoryKindLocalArchive, packaging: packaging}
}

// clone returns an independent copy, so a case can adjust its spec without leaking into others.
func (s *repoSpec) clone() *repoSpec {
	c := *s
	c.opts = append([]Option(nil), s.opts...)
	return &c
}

func (s *repoSpec) withOpts(opts ...Option) *repoSpec {
	c := s.clone()
	c.opts = append(c.opts, opts...)
	return c
}

func (s *repoSpec) withRoot(root string) *repoSpec {
	c := s.clone()
	c.root = root
	return c
}

func (s *repoSpec) withPattern(pattern string) *repoSpec {
	c := s.clone()
	c.keyPattern = pattern
	return c
}

func (s *repoSpec) withPermissions(permissions string) *repoSpec {
	c := s.clone()
	c.permissions = permissions
	return c
}

// prepareSpec describes the artifact a test prepares with the production files producer.
type prepareSpec struct {
	name      string
	version   string
	packaging artifact.Packaging
	files     map[string]string
	at        time.Time
	opID      string
	labels    map[string]string
}

func (p *prepareSpec) clone() *prepareSpec {
	c := *p
	return &c
}

func (p *prepareSpec) withDefaults(repo *Repository) *prepareSpec {
	p = p.clone()
	if p.name == "" {
		p.name = testName
	}
	if p.version == "" {
		p.version = testVersion
	}
	if p.packaging == "" {
		p.packaging = repo.packaging
	}
	if p.at.IsZero() {
		p.at = capturedAt
	}
	if p.opID == "" {
		p.opID = "op-" + p.name + "-" + p.version
	}
	if p.files == nil {
		p.files = map[string]string{"data.txt": "hello\n"}
	}
	return p
}

func prepare(t *testing.T, repo *Repository, spec *prepareSpec) *artifact.PreparedArtifact {
	t.Helper()
	spec = spec.withDefaults(repo)
	src := t.TempDir()
	for name, content := range spec.files {
		fixtures.WriteFile(t, src, name, content, 0o644)
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
	prepared := prepare(t, repo, spec)
	result, err := repo.Publish(context.Background(), prepared, options)
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

// listFiles returns the sorted slash-separated relative paths of every file below dir.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, relErr := filepath.Rel(dir, p)
			require.NoError(t, relErr)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	require.NoError(t, err)
	sort.Strings(out)
	return out
}

// payloadFiles lists the files outside the managed namespace.
func payloadFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, p := range listFiles(t, root) {
		if !layout.InManagedNamespace(p) {
			out = append(out, p)
		}
	}
	return out
}

func newSigner() *artifacttest.FakeSigner {
	return &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
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

// helperConfig is the configuration the lock helper process builds its repository from.
func helperConfig(root string) *schema.ArtifactRepositoryConfig {
	return &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": root}}
}
