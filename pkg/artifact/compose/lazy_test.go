package compose_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/schema"
)

// lazyFixture configures one repository of every kind, three of them with an identity, and counts
// every identity resolution and transport construction.
type lazyFixture struct {
	composition *compose.Composition
	resolved    atomic.Int64
	transports  atomic.Int64
	localRoot   string
	archiveRoot string
	layoutRoot  string
}

func newLazyFixture(t *testing.T) *lazyFixture {
	t.Helper()
	base := t.TempDir()
	f := &lazyFixture{
		localRoot:   filepath.Join(base, "dir-repository"),
		archiveRoot: filepath.Join(base, "archive-repository"),
		layoutRoot:  filepath.Join(base, "layout"),
	}
	repos := map[string]schema.ArtifactRepositoryConfig{
		"dir":     localRepo(artifact.RepositoryKindLocalDir, f.localRoot),
		"archive": localRepo(artifact.RepositoryKindLocalArchive, f.archiveRoot),
		"layout":  {Kind: artifact.RepositoryKindOCILayout, Options: map[string]any{"path": f.layoutRoot}},
		"bucket":  {Kind: artifact.RepositoryKindS3, Identity: "writer", Options: map[string]any{"bucket": "acme-artifacts"}},
		"registry": {
			Kind: artifact.RepositoryKindOCIRegistry, Identity: "registry-writer",
			Options: map[string]any{"registry": "ghcr.io", "prefix": "acme/atmos"},
		},
	}
	f.composition = build(
		t, repos, nil,
		compose.WithIdentityResolver(func(context.Context, string) (any, error) {
			f.resolved.Add(1)
			return nil, os.ErrPermission
		}),
		compose.WithTransportFactory(newCountingFactory(&f.transports)),
	)
	return f
}

func (f *lazyFixture) assertUntouched(t *testing.T, message string) {
	t.Helper()
	assert.Zero(t, f.resolved.Load(), "%s: no identity may be resolved", message)
	assert.Zero(t, f.transports.Load(), "%s: no registry transport may be built", message)
	for _, root := range []string{f.localRoot, f.archiveRoot, f.layoutRoot} {
		_, err := os.Stat(root)
		assert.True(t, os.IsNotExist(err), "%s: %s must not be created", message, root)
	}
}

func TestNew_BuildsNothingEagerly(t *testing.T) {
	f := newLazyFixture(t)
	f.assertUntouched(t, "building the composition")

	for _, alias := range f.composition.Repositories.Aliases() {
		info, err := f.composition.Repositories.Describe(alias)
		require.NoError(t, err)
		assert.Equal(t, alias, info.Alias)
	}
	f.assertUntouched(t, "describing every repository")
}

func TestDryRun_NeverAuthenticatesCreatesDirectoriesOrContactsRegistries(t *testing.T) {
	f := newLazyFixture(t)
	src := source(t, "hello")

	for _, alias := range []string{"dir", "archive", "layout", "bucket", "registry"} {
		t.Run(alias, func(t *testing.T) {
			request := publishRequest(alias, src)
			request.DryRun = true
			result, err := f.composition.Service.Publish(context.Background(), request)
			require.NoError(t, err)
			assert.Equal(t, artifact.OutcomeDryRun, result.Outcome)
			assert.NotEmpty(t, result.RecordDigest, "a dry run still prepares and identifies the artifact")
			f.assertUntouched(t, "dry-run publish to "+alias)
		})
	}
}

func TestOperationsAuthenticateOnlyWhenARepositoryIsActuallyUsed(t *testing.T) {
	f := newLazyFixture(t)

	_, err := f.composition.Service.Publish(context.Background(), publishRequest("bucket", source(t, "hello")))
	require.Error(t, err, "the injected resolver refuses, so the real operation fails closed")
	assert.Equal(t, int64(1), f.resolved.Load(), "the repository's own identity is resolved once, lazily, when the operation needs credentials")
	assert.Zero(t, f.transports.Load(), "another repository's transport is never built")
}
