package imports

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields the tests set.
var (
	_ = schema.ImportsSettings{Identity: "x"}
	_ = schema.StackImport{Identity: "x"}
)

// contextRecorder is a downloader that also implements ContextFileDownloader, recording the
// per-fetch environment each contextual fetch ran with. Plain Fetch calls go to the generated mock.
type contextRecorder struct {
	*downloader.MockFileDownloader
	mu   sync.Mutex
	envs [][]string
}

func (c *contextRecorder) FetchWithMetadataContext(ctx context.Context, _, dest string, _ downloader.ClientMode, _ time.Duration) (downloader.FetchMetadata, error) {
	c.mu.Lock()
	c.envs = append(c.envs, downloader.EnvironmentFromContext(ctx))
	c.mu.Unlock()
	// The content names the identity that fetched it, so a test can tell whose copy it was served.
	content := "vars:\n  fetched_as: " + strconv.Quote(downloader.IdentityFromContext(ctx)) + "\n"
	return downloader.FetchMetadata{}, os.WriteFile(dest, []byte(content), testFilePerm)
}

func newRecorderImporter(t *testing.T, atmosConfig *schema.AtmosConfiguration) *contextRecorder {
	t.Helper()
	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(t.TempDir()))
	require.NoError(t, err)
	recorder := &contextRecorder{MockFileDownloader: downloader.NewMockFileDownloader(gomock.NewController(t))}
	importer, err := NewRemoteImporter(atmosConfig, WithCache(testCache), WithDownloader(recorder))
	require.NoError(t, err)
	setGlobalImporter(t, importer)
	return recorder
}

func registerImportResolver(t *testing.T) {
	t.Helper()
	downloader.RegisterIdentityEnvironmentResolver(func(_ context.Context, _ *schema.AtmosConfiguration, identity string) ([]string, error) {
		return []string{"IMPORT_IDENTITY=" + identity}, nil
	})
	t.Cleanup(func() { downloader.RegisterIdentityEnvironmentResolver(nil) })
}

func TestResolveRemoteImportNestedIdentity_UsesImportOrSectionIdentity(t *testing.T) {
	registerImportResolver(t)

	tests := []struct {
		name    string
		entry   string
		section string
		want    []string
	}{
		{name: "the import's own identity", entry: "ops", section: "default", want: []string{"IMPORT_IDENTITY=ops"}},
		{name: "imports.identity when the import names none", section: "default", want: []string{"IMPORT_IDENTITY=default"}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newRecorderImporter(t, &schema.AtmosConfiguration{Imports: schema.ImportsSettings{Identity: tt.section}})
			uri := fmt.Sprintf("https://example.invalid/stacks/catalog-%d.yaml", i)

			matches, err := ResolveRemoteImportNestedIdentity(&schema.AtmosConfiguration{Imports: schema.ImportsSettings{Identity: tt.section}}, uri, "", "", tt.entry)

			require.NoError(t, err)
			require.Len(t, matches, 1)
			assert.Equal(t, [][]string{tt.want}, recorder.envs)
		})
	}
}

func TestResolveRemoteImportNestedIdentity_NoIdentityUsesPlainFetch(t *testing.T) {
	registerImportResolver(t)
	recorder := newRecorderImporter(t, &schema.AtmosConfiguration{})
	const uri = "https://example.invalid/stacks/plain.yaml"
	recorder.MockFileDownloader.EXPECT().
		Fetch(uri, gomock.Any(), downloader.ClientModeFile, gomock.Any()).
		DoAndReturn(func(_, dest string, _ downloader.ClientMode, _ time.Duration) error {
			return os.WriteFile(dest, []byte("vars:\n  a: true\n"), testFilePerm)
		}).Times(1)

	matches, err := ResolveRemoteImportNestedIdentity(&schema.AtmosConfiguration{}, uri, "", "", "")

	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Empty(t, recorder.envs, "an import without an identity keeps using the plain downloader call")
}

func TestResolveRemoteImportNestedIdentity_LoginRequiredFailsWithoutFetching(t *testing.T) {
	downloader.RegisterIdentityEnvironmentResolver(func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		return nil, errUtils.ErrRemoteIdentityLoginRequired
	})
	t.Cleanup(func() { downloader.RegisterIdentityEnvironmentResolver(nil) })
	recorder := newRecorderImporter(t, &schema.AtmosConfiguration{})

	_, err := ResolveRemoteImportNestedIdentity(&schema.AtmosConfiguration{}, "https://example.invalid/stacks/private.yaml", "", "", "ops")

	require.ErrorIs(t, err, errUtils.ErrRemoteIdentityLoginRequired)
	require.ErrorIs(t, err, errUtils.ErrRemoteImportIdentity)
	assert.False(t, SkippableWhenMissing(err), "skip_if_missing must not hide an identity that is not logged in")
	assert.Empty(t, recorder.envs)
}

// skip_if_missing covers a source that is not there, and nothing else.
func TestSkippableWhenMissing(t *testing.T) {
	missing := fmt.Errorf("%w: 404", errUtils.ErrDownloadRemoteImport)
	identity := fmt.Errorf("%w: %w", errUtils.ErrRemoteImportIdentity, errUtils.ErrIdentityNotFound)

	assert.True(t, SkippableWhenMissing(missing))
	assert.False(t, SkippableWhenMissing(identity))
	assert.False(t, SkippableWhenMissing(nil), "nothing failed, so there is nothing to skip")
}

// What one identity may read, another may not. The same URI fetched as two identities must be two
// fetches with two results, and neither may be served to a fetch that names no identity.
func TestResolveRemoteImportNestedIdentity_CachesAreSeparatedByIdentity(t *testing.T) {
	registerImportResolver(t)
	atmosConfig := &schema.AtmosConfiguration{}
	recorder := newRecorderImporter(t, atmosConfig)
	const uri = "https://example.invalid/stacks/shared.yaml"
	recorder.MockFileDownloader.EXPECT().
		Fetch(uri, gomock.Any(), downloader.ClientModeFile, gomock.Any()).
		DoAndReturn(func(_, dest string, _ downloader.ClientMode, _ time.Duration) error {
			return os.WriteFile(dest, []byte("vars:\n  fetched_as: \"\"\n"), testFilePerm)
		}).Times(1)
	read := func(identity string) (path, content string) {
		t.Helper()
		matches, err := ResolveRemoteImportNestedIdentity(atmosConfig, uri, "", "", identity)
		require.NoError(t, err)
		require.Len(t, matches, 1)
		assert.Equal(t, uri, matches[0].Key, "the reported import key is the URI, never the scoped cache key")
		data, err := os.ReadFile(matches[0].Path)
		require.NoError(t, err)
		return matches[0].Path, string(data)
	}

	opsPath, opsContent := read("ops")
	deployPath, deployContent := read("deploy")
	plainPath, plainContent := read("")
	opsAgainPath, opsAgainContent := read("ops")

	assert.Contains(t, opsContent, `fetched_as: "ops"`)
	assert.Contains(t, deployContent, `fetched_as: "deploy"`, "a second identity must not receive the first identity's content")
	assert.Contains(t, plainContent, `fetched_as: ""`, "a fetch that names no identity must not receive an identity's content")
	assert.NotEqual(t, opsPath, deployPath)
	assert.NotEqual(t, opsPath, plainPath)
	assert.Equal(t, opsPath, opsAgainPath, "the same identity reuses its own cached copy")
	assert.Equal(t, opsContent, opsAgainContent)
	assert.Equal(t, [][]string{{"IMPORT_IDENTITY=ops"}, {"IMPORT_IDENTITY=deploy"}}, recorder.envs,
		"one fetch per identity: the repeat for ops is served from its cache")
}

// An oci:// import authenticates through the registry keychain. An identity it would never use
// must not be resolved, so a missing session cannot stop the import.
func TestResolveRemoteImportNestedIdentity_OCIImportDoesNotResolveTheIdentity(t *testing.T) {
	resolved := 0
	downloader.RegisterIdentityEnvironmentResolver(func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		resolved++
		return nil, errUtils.ErrRemoteIdentityLoginRequired
	})
	t.Cleanup(func() { downloader.RegisterIdentityEnvironmentResolver(nil) })
	atmosConfig := &schema.AtmosConfiguration{Imports: schema.ImportsSettings{Identity: "ops"}}
	recorder := newRecorderImporter(t, atmosConfig)
	registryDown := errors.New("registry unreachable")
	recorder.MockFileDownloader.EXPECT().
		Fetch(gomock.Any(), gomock.Any(), downloader.ClientModeDir, gomock.Any()).
		Return(registryDown).Times(1)

	_, err := ResolveRemoteImportNestedIdentity(atmosConfig, "oci://ghcr.io/acme/stacks:v1//catalog/vpc.yaml", "", "", "")

	require.ErrorIs(t, err, registryDown, "the import reached the registry fetch")
	assert.NotErrorIs(t, err, errUtils.ErrRemoteImportIdentity)
	assert.Zero(t, resolved, "the identity is not resolved for an OCI import")
	assert.Empty(t, recorder.envs, "the fetch ran without an identity environment")
}
