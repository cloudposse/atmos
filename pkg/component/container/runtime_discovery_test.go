package container

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	cachepkg "github.com/cloudposse/atmos/pkg/cache"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// envSetterMockRuntime combines a MockRuntime with a real SetEnv implementation
// so it satisfies both ctr.Runtime and ctr.EnvSetter, recording every call for
// assertions. Used to test the EnvSetter type-assertion branches in
// invalidatingRuntime.SetEnv and resolved.runtime.
type envSetterMockRuntime struct {
	*MockRuntime
	setEnvCalls [][]string
}

func (m *envSetterMockRuntime) SetEnv(env []string) {
	m.setEnvCalls = append(m.setEnvCalls, env)
}

func isolateRuntimeSelectionCache(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ATMOS_CONTAINER_RUNTIME", "")
}

func TestResolveRuntimeForContainerCommand_CachesAutomaticSelection(t *testing.T) {
	isolateRuntimeSelectionCache(t)
	original := detectRuntime
	t.Cleanup(func() { detectRuntime = original })

	detections := 0
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		detections++
		return ctr.NewPodmanRuntime(), nil
	}

	first, err := resolveRuntimeForContainerCommand(context.Background(), "", true)
	require.NoError(t, err)
	assert.False(t, first.cached)

	second, err := resolveRuntimeForContainerCommand(context.Background(), "", true)
	require.NoError(t, err)
	assert.True(t, second.cached)
	assert.Equal(t, 1, detections)
}

func TestResolveRuntimeForContainerCommand_ExplicitProviderBypassesCache(t *testing.T) {
	isolateRuntimeSelectionCache(t)
	original := detectRuntime
	t.Cleanup(func() { detectRuntime = original })

	detections := 0
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		detections++
		return ctr.NewPodmanRuntime(), nil
	}

	_, err := resolveRuntimeForContainerCommand(context.Background(), "podman", true)
	require.NoError(t, err)
	_, err = resolveRuntimeForContainerCommand(context.Background(), "podman", true)
	require.NoError(t, err)
	assert.Equal(t, 2, detections)
}

func TestResolveRuntimeForContainerCommand_CorruptCacheFallsBackToDetection(t *testing.T) {
	isolateRuntimeSelectionCache(t)
	cache, err := cachepkg.NewFileCache(runtimeCacheSubpath)
	require.NoError(t, err)
	require.NoError(t, cache.Set(runtimeSelectionCacheKey(""), []byte("not json")))

	original := detectRuntime
	t.Cleanup(func() { detectRuntime = original })
	detections := 0
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		detections++
		return ctr.NewDockerRuntime(), nil
	}

	resolution, err := resolveRuntimeForContainerCommand(context.Background(), "", true)
	require.NoError(t, err)
	assert.False(t, resolution.cached)
	assert.Equal(t, 1, detections)
}

func TestRediscoverRuntimeInvalidatesCachedSelection(t *testing.T) {
	isolateRuntimeSelectionCache(t)
	original := detectRuntime
	t.Cleanup(func() { detectRuntime = original })

	detections := 0
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		detections++
		return ctr.NewPodmanRuntime(), nil
	}

	_, err := resolveRuntimeForContainerCommand(context.Background(), "", true)
	require.NoError(t, err)
	cached, err := resolveRuntimeForContainerCommand(context.Background(), "", true)
	require.NoError(t, err)
	require.True(t, cached.cached)

	refreshed, err := rediscoverRuntime(context.Background(), cached)
	require.NoError(t, err)
	assert.False(t, refreshed.cached)
	assert.Equal(t, 2, detections)
}

func TestRuntimeSelectionCacheKeyChangesWithRuntimeEndpoint(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///tmp/docker-a.sock")
	first := runtimeSelectionCacheKey("")
	t.Setenv("DOCKER_HOST", "unix:///tmp/docker-b.sock")
	assert.NotEqual(t, first, runtimeSelectionCacheKey(""))
}

func TestInvalidatingRuntimeInvalidatesOnOperationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("runtime unavailable"))

	invalidated := false
	wrapped := invalidatingRuntime{Runtime: runtime, invalidateCache: func() { invalidated = true }}
	_, err := wrapped.List(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, invalidated)
}

func TestResolveRuntimeForContainerCommand_CachedRuntimeOperationErrorInvalidatesCache(t *testing.T) {
	isolateRuntimeSelectionCache(t)
	original := detectRuntime
	t.Cleanup(func() { detectRuntime = original })
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		return ctr.NewPodmanRuntime(), nil
	}

	_, err := resolveRuntimeForContainerCommand(context.Background(), "", true)
	require.NoError(t, err)
	resolution, err := resolveRuntimeForContainerCommand(context.Background(), "", true)
	require.NoError(t, err)
	require.True(t, resolution.cached)

	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("runtime unavailable"))
	wrapped := resolution.runtime.(invalidatingRuntime)
	wrapped.Runtime = runtime
	_, err = wrapped.List(context.Background(), nil)
	require.Error(t, err)

	_, exists, err := resolution.cache.Get(resolution.cacheKey)
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestLoadCachedRuntime covers every branch of the cached-record decode switch:
// a valid Docker record, a valid Podman record, and a record whose runtime type
// is neither (which must self-heal by deleting the stale entry).
func TestLoadCachedRuntime(t *testing.T) {
	tests := []struct {
		name      string
		record    runtimeCacheRecord
		wantFound bool
		wantType  ctr.Type
	}{
		{
			name:      "docker record resolves to a docker runtime",
			record:    runtimeCacheRecord{Version: runtimeCacheVersion, Runtime: ctr.TypeDocker},
			wantFound: true,
			wantType:  ctr.TypeDocker,
		},
		{
			name:      "podman record resolves to a podman runtime",
			record:    runtimeCacheRecord{Version: runtimeCacheVersion, Runtime: ctr.TypePodman},
			wantFound: true,
			wantType:  ctr.TypePodman,
		},
		{
			name:      "unknown runtime type is treated as a miss and deleted",
			record:    runtimeCacheRecord{Version: runtimeCacheVersion, Runtime: ctr.Type("bogus")},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := cachepkg.NewFileCache(runtimeCacheSubpath, cachepkg.WithBaseDir(t.TempDir()))
			require.NoError(t, err)
			key := "cache-key"

			content, err := json.Marshal(tt.record)
			require.NoError(t, err)
			require.NoError(t, cache.Set(key, content))

			runtime, ok := loadCachedRuntime(cache, key)
			assert.Equal(t, tt.wantFound, ok)

			_, exists, err := cache.Get(key)
			require.NoError(t, err)

			if tt.wantFound {
				require.NotNil(t, runtime)
				assert.Equal(t, tt.wantType, ctr.GetRuntimeType(runtime))
				assert.True(t, exists, "a valid cache entry must not be deleted")
			} else {
				assert.Nil(t, runtime)
				assert.False(t, exists, "an unrecognized runtime type must self-heal by deleting the entry")
			}
		})
	}
}

func TestLoadCachedRuntime_MissingKey(t *testing.T) {
	cache, err := cachepkg.NewFileCache(runtimeCacheSubpath, cachepkg.WithBaseDir(t.TempDir()))
	require.NoError(t, err)

	runtime, ok := loadCachedRuntime(cache, "does-not-exist")
	assert.False(t, ok)
	assert.Nil(t, runtime)
}

// TestInvalidatingRuntime_DelegatesAndInvalidatesOnError exercises every
// invalidatingRuntime delegate method (except List, covered separately above):
// it must forward the call to the underlying runtime unchanged, invalidate the
// cache when the underlying call errors, and leave the cache alone on success.
func TestInvalidatingRuntime_DelegatesAndInvalidatesOnError(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(m *MockRuntime, callErr error)
		invoke    func(ctx context.Context, r invalidatingRuntime) error
	}{
		{
			name: "Build",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Build(gomock.Any(), gomock.Any()).Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Build(ctx, &ctr.BuildConfig{})
			},
		},
		{
			name: "Create",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return("id", callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				_, err := r.Create(ctx, &ctr.CreateConfig{})
				return err
			},
		},
		{
			name: "Start",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Start(gomock.Any(), "id").Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Start(ctx, "id")
			},
		},
		{
			name: "Stop",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Stop(gomock.Any(), "id", time.Second).Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Stop(ctx, "id", time.Second)
			},
		},
		{
			name: "Remove",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Remove(gomock.Any(), "id", true).Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Remove(ctx, "id", true)
			},
		},
		{
			name: "Inspect",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Inspect(gomock.Any(), "id").Return(&ctr.Info{}, callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				_, err := r.Inspect(ctx, "id")
				return err
			},
		},
		{
			name: "Exec",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Exec(gomock.Any(), "id", gomock.Any(), gomock.Any()).Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Exec(ctx, "id", []string{"true"}, &ctr.ExecOptions{})
			},
		},
		{
			name: "Shell",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Shell(gomock.Any(), "id", gomock.Any()).Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Shell(ctx, "id", &ctr.ShellOptions{})
			},
		},
		{
			name: "Attach",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Attach(gomock.Any(), "id", gomock.Any()).Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Attach(ctx, "id", &ctr.AttachOptions{})
			},
		},
		{
			name: "Pull",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Pull(gomock.Any(), "image").Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Pull(ctx, "image")
			},
		},
		{
			name: "Tag",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Tag(gomock.Any(), "source", "target").Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Tag(ctx, "source", "target")
			},
		},
		{
			name: "Push",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Push(gomock.Any(), "image").Return(&ctr.PushResult{}, callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				_, err := r.Push(ctx, "image")
				return err
			},
		},
		{
			name: "ImageInspect",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().ImageInspect(gomock.Any(), "image").Return(&ctr.ImageInfo{}, callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				_, err := r.ImageInspect(ctx, "image")
				return err
			},
		},
		{
			name: "Logs",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Logs(gomock.Any(), "id", true, "10", gomock.Any(), gomock.Any()).Return(callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				return r.Logs(ctx, "id", true, "10", io.Discard, io.Discard)
			},
		},
		{
			name: "Info",
			setupMock: func(m *MockRuntime, callErr error) {
				m.EXPECT().Info(gomock.Any()).Return(&ctr.RuntimeInfo{}, callErr)
			},
			invoke: func(ctx context.Context, r invalidatingRuntime) error {
				_, err := r.Info(ctx)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/success leaves cache intact", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime, nil)

			invalidated := false
			wrapped := invalidatingRuntime{Runtime: mockRuntime, invalidateCache: func() { invalidated = true }}

			err := tt.invoke(context.Background(), wrapped)
			require.NoError(t, err)
			assert.False(t, invalidated)
		})

		t.Run(tt.name+"/error invalidates cache", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockRuntime := NewMockRuntime(ctrl)
			wantErr := errors.New("operation failed")
			tt.setupMock(mockRuntime, wantErr)

			invalidated := false
			wrapped := invalidatingRuntime{Runtime: mockRuntime, invalidateCache: func() { invalidated = true }}

			err := tt.invoke(context.Background(), wrapped)
			require.Error(t, err)
			assert.ErrorIs(t, err, wantErr)
			assert.True(t, invalidated)
		})
	}
}

// TestInvalidatingRuntime_SetEnv verifies SetEnv forwards to the underlying
// runtime only when it implements ctr.EnvSetter, and is a safe no-op otherwise.
func TestInvalidatingRuntime_SetEnv(t *testing.T) {
	t.Run("forwards to an underlying EnvSetter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		underlying := &envSetterMockRuntime{MockRuntime: NewMockRuntime(ctrl)}
		wrapped := invalidatingRuntime{Runtime: underlying}

		wrapped.SetEnv([]string{"FOO=bar"})

		require.Len(t, underlying.setEnvCalls, 1)
		assert.Equal(t, []string{"FOO=bar"}, underlying.setEnvCalls[0])
	})

	t.Run("no-op when the underlying runtime does not implement EnvSetter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		underlying := NewMockRuntime(ctrl)
		wrapped := invalidatingRuntime{Runtime: underlying}

		assert.NotPanics(t, func() { wrapped.SetEnv([]string{"FOO=bar"}) })
	})
}
