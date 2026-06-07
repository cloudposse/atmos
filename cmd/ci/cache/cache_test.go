package cache

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cipkg "github.com/cloudposse/atmos/pkg/ci"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags/global"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// initTestIO initializes the I/O and UI contexts so data.Write / ui.* work in tests.
func initTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
}

// fakeBackend is an in-memory cachepkg.Backend for command tests.
type fakeBackend struct {
	blobs       map[string][]byte
	listEntries []cachepkg.Entry
	listErr     error
	deleteErr   error
	saveErr     error
	deleted     []string
}

func newFakeBackend() *fakeBackend { return &fakeBackend{blobs: map[string][]byte{}} }

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Save(_ context.Context, key string, data io.Reader, _ int64) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	f.blobs[key] = b
	return nil
}

func (f *fakeBackend) Restore(_ context.Context, key string, restoreKeys []string) (string, io.ReadCloser, error) {
	if b, ok := f.blobs[key]; ok {
		return key, io.NopCloser(bytes.NewReader(b)), nil
	}
	for _, rk := range restoreKeys {
		for k, b := range f.blobs {
			if strings.HasPrefix(k, rk) {
				return k, io.NopCloser(bytes.NewReader(b)), nil
			}
		}
	}
	return "", nil, errUtils.ErrCacheNotFound
}

func (f *fakeBackend) List(_ context.Context, _ cachepkg.ListOptions) ([]cachepkg.Entry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listEntries, nil
}

func (f *fakeBackend) Delete(_ context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, key)
	return nil
}

// newStubSetup builds a Manager over a fakeBackend rooted at a temp dir and
// stubs the package-level cacheSetup to return it, restoring the original on
// cleanup. It returns the backend and config so tests can inspect/seed them.
func newStubSetup(t *testing.T, key string) (*fakeBackend, *cachepkg.Config) {
	t.Helper()
	initTestIO(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "toolchain", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "toolchain", "bin", "tool"), []byte("binary"), 0o644))

	cfg := &cachepkg.Config{Enabled: true, Root: root, Key: key, Compression: "gzip"}
	fake := newFakeBackend()
	mgr := cachepkg.NewManager(fake, cfg)

	orig := cacheSetup
	t.Cleanup(func() { cacheSetup = orig })
	cacheSetup = func(_ *cobra.Command, _ cacheOverrides) (*cachepkg.Manager, *cachepkg.Config, error) {
		return mgr, cfg, nil
	}
	return fake, cfg
}

func TestCacheOverrides_Apply(t *testing.T) {
	t.Run("sets fields when provided", func(t *testing.T) {
		cc := &schema.CICacheConfig{}
		o := cacheOverrides{key: "k", root: "/r", paths: []string{"a", "b"}}
		o.apply(cc)
		assert.Equal(t, "k", cc.Key)
		assert.Equal(t, "/r", cc.Root)
		assert.Equal(t, []string{"a", "b"}, cc.Paths)
	})

	t.Run("leaves fields untouched when empty", func(t *testing.T) {
		cc := &schema.CICacheConfig{Key: "orig", Root: "/orig", Paths: []string{"x"}}
		cacheOverrides{}.apply(cc)
		assert.Equal(t, "orig", cc.Key)
		assert.Equal(t, "/orig", cc.Root)
		assert.Equal(t, []string{"x"}, cc.Paths)
	})
}

func TestBuildConfigAndStacksInfo(t *testing.T) {
	assert.Equal(t, schema.ConfigAndStacksInfo{}, buildConfigAndStacksInfo(nil))

	gf := &global.Flags{
		BasePath:   "/base",
		Config:     []string{"a.yaml"},
		ConfigPath: []string{"/dir"},
		Profile:    []string{"prod"},
	}
	info := buildConfigAndStacksInfo(gf)
	assert.Equal(t, "/base", info.AtmosBasePath)
	assert.Equal(t, []string{"a.yaml"}, info.AtmosConfigFilesFromArg)
	assert.Equal(t, []string{"/dir"}, info.AtmosConfigDirsFromArg)
	assert.Equal(t, []string{"prod"}, info.ProfilesFromArg)
}

func TestRunCacheDelete_Success(t *testing.T) {
	fake, _ := newStubSetup(t, "k1")
	require.NoError(t, cacheDeleteCmd.Flags().Set("key", "k1"))
	t.Cleanup(func() { _ = cacheDeleteCmd.Flags().Set("key", "") })

	err := runCacheDelete(cacheDeleteCmd, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"k1"}, fake.deleted)
}

func TestRunCacheDelete_RequiresKey(t *testing.T) {
	newStubSetup(t, "k1")
	require.NoError(t, cacheDeleteCmd.Flags().Set("key", ""))

	err := runCacheDelete(cacheDeleteCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrCacheKeyRequired)
}

func TestRunCacheSave_Success(t *testing.T) {
	fake, cfg := newStubSetup(t, "k1")

	err := runCacheSave(cacheSaveCmd, nil)
	require.NoError(t, err)
	_, ok := fake.blobs[cfg.Key]
	assert.True(t, ok, "save should upload the archive under the key")
}

func TestRunCacheRestore_ExactHit(t *testing.T) {
	fake, cfg := newStubSetup(t, "k1")
	// Seed the backend so restore is an exact hit.
	mgr := cachepkg.NewManager(fake, cfg)
	_, err := mgr.Save(context.Background())
	require.NoError(t, err)
	// Remove the file so restore has work to do.
	require.NoError(t, os.Remove(filepath.Join(cfg.Root, "toolchain", "bin", "tool")))

	err = runCacheRestore(cacheRestoreCmd, nil)
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(cfg.Root, "toolchain", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, "binary", string(got))
}

func TestRunCacheRestore_Miss(t *testing.T) {
	newStubSetup(t, "missing")
	err := runCacheRestore(cacheRestoreCmd, nil)
	require.NoError(t, err)
}

func TestRunCacheList_WithEntries(t *testing.T) {
	fake, _ := newStubSetup(t, "k1")
	fake.listEntries = []cachepkg.Entry{
		{Key: "atmos-a", Size: 10},
		{Key: "atmos-b", Size: 20},
	}
	require.NoError(t, cacheListCmd.Flags().Set("format", "json"))
	t.Cleanup(func() { _ = cacheListCmd.Flags().Set("format", "table") })

	err := runCacheList(cacheListCmd, nil)
	require.NoError(t, err)
}

func TestRunCacheList_Empty(t *testing.T) {
	newStubSetup(t, "k1")
	err := runCacheList(cacheListCmd, nil)
	require.NoError(t, err)
}

func TestCommand(t *testing.T) {
	cmd := Command()
	require.NotNil(t, cmd)
	assert.Equal(t, "cache", cmd.Name())
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"restore", "save", "list", "delete"} {
		assert.True(t, names[want], "expected subcommand %q", want)
	}
}

func TestRunCacheSave_SkippedAlreadySaved(t *testing.T) {
	_, _ = newStubSetup(t, "k1")
	// First save records the entry as saved.
	require.NoError(t, runCacheSave(cacheSaveCmd, nil))
	// Second save is skipped because it was already saved this lifecycle.
	require.NoError(t, runCacheSave(cacheSaveCmd, nil))
}

func TestRunCacheSave_AlreadyExistsRemotely(t *testing.T) {
	fake, _ := newStubSetup(t, "k1")
	fake.saveErr = errUtils.ErrCacheAlreadyExists
	require.NoError(t, runCacheSave(cacheSaveCmd, nil))
}

func TestRunCacheRestore_RestoreKeyMatch(t *testing.T) {
	fake, cfg := newStubSetup(t, "atmos-new")
	// Seed a prefix-matching entry built from the same root under a different key.
	seedCfg := &cachepkg.Config{Enabled: true, Root: cfg.Root, Key: "atmos-old", Compression: "gzip"}
	_, err := cachepkg.NewManager(fake, seedCfg).Save(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(cfg.Root, "toolchain", "bin", "tool")))

	require.NoError(t, cacheRestoreCmd.Flags().Set("restore-key", "atmos-"))
	t.Cleanup(func() { _ = cacheRestoreCmd.Flags().Set("restore-key", "") })

	require.NoError(t, runCacheRestore(cacheRestoreCmd, nil))
	got, err := os.ReadFile(filepath.Join(cfg.Root, "toolchain", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, "binary", string(got))
}

func TestRunCacheRestore_Skipped(t *testing.T) {
	fake, cfg := newStubSetup(t, "k1")
	_, err := cachepkg.NewManager(fake, cfg).Save(context.Background())
	require.NoError(t, err)
	// First restore is a hit; second is skipped (idempotent within lifecycle).
	require.NoError(t, runCacheRestore(cacheRestoreCmd, nil))
	require.NoError(t, runCacheRestore(cacheRestoreCmd, nil))
}

func TestRunCacheDelete_BackendError(t *testing.T) {
	fake, _ := newStubSetup(t, "k1")
	fake.deleteErr = errUtils.ErrCacheDeleteFailed
	require.NoError(t, cacheDeleteCmd.Flags().Set("key", "k1"))
	t.Cleanup(func() { _ = cacheDeleteCmd.Flags().Set("key", "") })

	err := runCacheDelete(cacheDeleteCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrCacheDeleteFailed)
}

func TestRunCacheSave_BackendError(t *testing.T) {
	fake, _ := newStubSetup(t, "k1")
	fake.saveErr = errUtils.ErrCacheSaveFailed
	err := runCacheSave(cacheSaveCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrCacheSaveFailed)
}

func TestRunCacheList_BackendError(t *testing.T) {
	fake, _ := newStubSetup(t, "k1")
	fake.listErr = errUtils.ErrCacheListFailed
	err := runCacheList(cacheListCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrCacheListFailed)
}

func TestRunCacheList_InvalidFormat(t *testing.T) {
	initTestIO(t)
	require.NoError(t, cacheListCmd.Flags().Set("format", "bogus"))
	t.Cleanup(func() { _ = cacheListCmd.Flags().Set("format", "table") })
	err := runCacheList(cacheListCmd, nil)
	require.Error(t, err)
}

func TestRunCacheList_SetupError(t *testing.T) {
	orig := cacheSetup
	t.Cleanup(func() { cacheSetup = orig })
	wantErr := errUtils.ErrCacheUnavailable
	cacheSetup = func(_ *cobra.Command, _ cacheOverrides) (*cachepkg.Manager, *cachepkg.Config, error) {
		return nil, nil, wantErr
	}
	err := runCacheList(cacheListCmd, nil)
	require.ErrorIs(t, err, wantErr)
}

// stubResolveOK stubs the resolveCacheConfig seam to return cfg, restoring the
// original on cleanup. It lets tests drive the real cacheSetup without loading
// Atmos config.
func stubResolveOK(t *testing.T, cfg *cachepkg.Config) {
	t.Helper()
	orig := resolveCacheConfig
	t.Cleanup(func() { resolveCacheConfig = orig })
	resolveCacheConfig = func(_ *cobra.Command, _ cacheOverrides) (*cachepkg.Config, error) {
		return cfg, nil
	}
}

func TestCacheSetup_ResolveError(t *testing.T) {
	// When resolveCacheConfig fails, cacheSetup surfaces that error unchanged.
	orig := resolveCacheConfig
	t.Cleanup(func() { resolveCacheConfig = orig })
	resolveCacheConfig = func(_ *cobra.Command, _ cacheOverrides) (*cachepkg.Config, error) {
		return nil, errUtils.ErrCacheUnavailable
	}

	_, _, err := cacheSetup(&cobra.Command{}, cacheOverrides{})
	require.ErrorIs(t, err, errUtils.ErrCacheUnavailable)
}

func TestCacheSetup_NoBackendDetected(t *testing.T) {
	// Config resolves fine, but with an empty provider registry no CI cache
	// backend is detected, so cacheSetup reports the cache as unavailable.
	stubResolveOK(t, &cachepkg.Config{Enabled: true, Root: t.TempDir(), Key: "k"})
	restore := cipkg.SwapRegistryForTest()
	t.Cleanup(restore)

	_, _, err := cacheSetup(&cobra.Command{}, cacheOverrides{})
	require.ErrorIs(t, err, errUtils.ErrCacheUnavailable)
}

// isolateAtmosConfig points config discovery at an empty temp dir so the real
// resolveCacheConfig does not pick up the repository's atmos.yaml.
func isolateAtmosConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
}

func TestResolveCacheConfig_DisabledByDefault(t *testing.T) {
	isolateAtmosConfig(t)
	// Cache is off by default, so resolveCacheConfig reports it as unavailable.
	_, err := resolveCacheConfig(&cobra.Command{}, cacheOverrides{})
	require.ErrorIs(t, err, errUtils.ErrCacheUnavailable)
}

func TestResolveCacheConfig_EnabledSuccess(t *testing.T) {
	isolateAtmosConfig(t)
	t.Setenv("ATMOS_CI_CACHE_ENABLED", "true")
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())

	cfg, err := resolveCacheConfig(&cobra.Command{}, cacheOverrides{})
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.NotEmpty(t, cfg.Key)
}
