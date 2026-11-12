package imports

import (
	"context"
	"fmt"
	"os"
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
	return downloader.FetchMetadata{}, os.WriteFile(dest, []byte("vars:\n  a: true\n"), testFilePerm)
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
		{name: "settings.imports.identity when the import names none", section: "default", want: []string{"IMPORT_IDENTITY=default"}},
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
	assert.Empty(t, recorder.envs)
}
