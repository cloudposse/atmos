package emulator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// persistDriverName/persistDataDir back an in-test AWS-target driver that DOES
// declare a data dir, so the persistence-mount injection path is exercised
// (the package's other test drivers declare none).
const (
	persistDriverName = "test/persist"
	persistDataDir    = "/var/lib/persist"
)

type persistTestDriver struct{}

func (persistTestDriver) Name() string   { return persistDriverName }
func (persistTestDriver) Target() string { return TargetAWS }

func (persistTestDriver) Defaults() ContainerDefaults {
	return ContainerDefaults{Image: "test/persist:latest", Ports: []int{1234}, DataDir: persistDataDir}
}

func (persistTestDriver) Profile(_ *Endpoint) Profile { return Profile{} }

func init() { RegisterDriver(persistTestDriver{}) }

// findMount returns the mount targeting the given container path, or nil.
func findMount(mounts []container.Mount, target string) *container.Mount {
	for i := range mounts {
		if mounts[i].Target == target {
			return &mounts[i]
		}
	}
	return nil
}

func TestNamedConfig_InjectsPersistenceMount(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())
	m := newManagerWithRuntime(nil)

	cfg, err := m.namedConfig(&Spec{Driver: persistDriverName}, "dev", "aws", nil, false)
	require.NoError(t, err)

	mount := findMount(cfg.Mounts, persistDataDir)
	require.NotNil(t, mount, "persistence bind mount must be injected onto the driver data dir")
	assert.Equal(t, "bind", mount.Type)
	assert.False(t, mount.ReadOnly)

	wantHost, err := InstanceDataDir("dev", "aws")
	require.NoError(t, err)
	assert.Equal(t, wantHost, mount.Source)
	// The host dir is created under the configured XDG cache, named by the
	// sanitized canonical runtime name.
	assert.Equal(t, container.RuntimeName("dev", "emulator", "aws"), filepath.Base(mount.Source))
	info, statErr := os.Stat(mount.Source)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestNamedConfig_EphemeralSkipsPersistenceMount(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv(xdg.EnvAtmosXDGCacheHome, cacheHome)
	m := newManagerWithRuntime(nil)

	ephemeral := true
	cfg, err := m.namedConfig(&Spec{Driver: persistDriverName, Ephemeral: &ephemeral}, "dev", "aws", nil, false)
	require.NoError(t, err)

	assert.Nil(t, findMount(cfg.Mounts, persistDataDir), "ephemeral instance must not get a persistence mount")
	// Ephemeral must not create the host state directory either.
	_, statErr := os.Stat(xdg.LookupXDGCacheDir(instanceCacheSubpath("dev", "aws")))
	assert.True(t, os.IsNotExist(statErr), "ephemeral instance must not create a host state dir")
}

func TestNamedConfig_PlumbsUserMounts(t *testing.T) {
	// testDriver has no DataDir, so the only mount must be the user's — proving the
	// previously-dropped container.mounts are now plumbed through.
	m := newManagerWithRuntime(nil)
	spec := &Spec{
		Driver: testDriverName,
		Container: &schema.ContainerRunStep{
			Mounts: []schema.ContainerMount{
				{Type: "bind", Source: "/host/data", Target: "/in/container", ReadOnly: true},
				{Source: "/host/two", Target: "/two"}, // no type -> defaults to bind.
			},
		},
	}

	cfg, err := m.namedConfig(spec, "dev", "aws", nil, false)
	require.NoError(t, err)
	require.Len(t, cfg.Mounts, 2)
	assert.Equal(t, container.Mount{Type: "bind", Source: "/host/data", Target: "/in/container", ReadOnly: true}, cfg.Mounts[0])
	assert.Equal(t, container.Mount{Type: "bind", Source: "/host/two", Target: "/two"}, cfg.Mounts[1])
}

func TestNamedConfig_UserMountWinsOverPersistence(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())
	m := newManagerWithRuntime(nil)
	spec := &Spec{
		Driver: persistDriverName,
		Container: &schema.ContainerRunStep{
			Mounts: []schema.ContainerMount{{Type: "volume", Source: "myvol", Target: persistDataDir}},
		},
	}

	cfg, err := m.namedConfig(spec, "dev", "aws", nil, false)
	require.NoError(t, err)

	// Exactly one mount targets the data dir: the user's, not the auto-injected bind.
	require.Len(t, cfg.Mounts, 1)
	assert.Equal(t, "volume", cfg.Mounts[0].Type)
	assert.Equal(t, "myvol", cfg.Mounts[0].Source)
	assert.Equal(t, persistDataDir, cfg.Mounts[0].Target)
}

func TestNamedConfig_NoDataDirNoMount(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())
	m := newManagerWithRuntime(nil)

	// testDriver has no DataDir and no user mounts -> no mounts at all.
	cfg, err := m.namedConfig(&Spec{Driver: testDriverName}, "dev", "aws", nil, false)
	require.NoError(t, err)
	assert.Empty(t, cfg.Mounts)
}

func TestInstanceDataDir_HonorsEnvSanitizesAndCreates(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv(xdg.EnvAtmosXDGCacheHome, cacheHome)

	// A stack name with a slash must be sanitized into a single flat dir name.
	dir, err := InstanceDataDir("dev/ue1", "aws")
	require.NoError(t, err)

	assert.Equal(t, container.RuntimeName("dev/ue1", "emulator", "aws"), filepath.Base(dir))
	assert.Equal(t, filepath.Join(cacheHome, "atmos", "emulator", filepath.Base(dir)), dir)
	info, statErr := os.Stat(dir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestDefaultEmulatorCacheHomeForBindMounts(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "Users", "alice")
	got, ok := defaultEmulatorCacheHomeForBindMounts("darwin", home)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(home, "Library", "Caches"), got)

	got, ok = defaultEmulatorCacheHomeForBindMounts("linux", "/home/alice")
	assert.False(t, ok)
	assert.Empty(t, got)

	got, ok = defaultEmulatorCacheHomeForBindMounts("darwin", "")
	assert.False(t, ok)
	assert.Empty(t, got)
}

func TestInstanceDataDir_CollisionFreeAcrossInstances(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())

	a, err := InstanceDataDir("dev", "aws")
	require.NoError(t, err)
	b, err := InstanceDataDir("dev", "gcp")
	require.NoError(t, err)
	c, err := InstanceDataDir("prod", "aws")
	require.NoError(t, err)

	assert.NotEqual(t, a, b)
	assert.NotEqual(t, a, c)
}

func TestLookupInstanceDataDir_DoesNotCreate(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())

	lookup := LookupInstanceDataDir("dev", "aws")
	_, statErr := os.Stat(lookup)
	require.True(t, os.IsNotExist(statErr), "lookup must not create the directory")

	created, err := InstanceDataDir("dev", "aws")
	require.NoError(t, err)
	assert.Equal(t, created, lookup, "lookup and create must resolve the same path")
}

func TestManager_Reset_RemovesPersistedState(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())

	// Seed a persisted state dir with a file.
	dir, err := InstanceDataDir("dev", "aws")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.db"), []byte("data"), 0o600))

	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	// Down discovers no container (List empty) -> no-op; Reset then wipes the dir.
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)

	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Reset(context.Background(), nil, "dev", "aws"))

	_, statErr := os.Stat(dir)
	assert.True(t, os.IsNotExist(statErr), "reset must delete the persisted state dir")
}

func TestManager_Reset_NoStateIsNoOp(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())

	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)

	// No state dir was ever created (e.g. an ephemeral instance): reset is a clean
	// no-op rather than an error.
	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Reset(context.Background(), nil, "dev", "aws"))
}

func TestManager_Reset_DownErrorPreservesState(t *testing.T) {
	t.Setenv(xdg.EnvAtmosXDGCacheHome, t.TempDir())

	dir, err := InstanceDataDir("dev", "aws")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.db"), []byte("data"), 0o600))

	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errRuntimeBoom)

	m := newManagerWithRuntime(runtime)
	require.Error(t, m.Reset(context.Background(), nil, "dev", "aws"))

	// A failed Down must not wipe state.
	_, statErr := os.Stat(dir)
	assert.NoError(t, statErr, "state must survive when Down fails")
}

func TestWipePersistedStateInContainer_ExecsRmInRunningContainer(t *testing.T) {
	// On a rootful runtime the container writes root-owned files the host can't
	// delete; Reset wipes them from inside the running container first. Verify the
	// in-container `rm -rf` of the driver's data dir is issued.
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).
		Return([]container.Info{runningEmulatorInfo(4566)}, nil)
	runtime.EXPECT().Exec(gomock.Any(), "container-abc", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, cmd []string, _ *container.ExecOptions) error {
			require.Len(t, cmd, 3)
			assert.Equal(t, "sh", cmd[0])
			assert.Equal(t, "-c", cmd[1])
			assert.Contains(t, cmd[2], persistDataDir, "must rm the driver's data dir")
			return nil
		})

	m := newManagerWithRuntime(runtime)
	m.wipePersistedStateInContainer(context.Background(), &Spec{Driver: persistDriverName}, "dev", "aws")
}

func TestWipePersistedStateInContainer_SkipsWhenEphemeral(t *testing.T) {
	// An ephemeral instance has no persisted state: no runtime calls at all.
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl) // any call fails the test (no EXPECT).

	m := newManagerWithRuntime(runtime)
	ephemeral := true
	m.wipePersistedStateInContainer(context.Background(),
		&Spec{Driver: persistDriverName, Ephemeral: &ephemeral}, "dev", "aws")
}
