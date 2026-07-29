package container

import (
	"context"
	"errors"
	"testing"

	cachepkg "github.com/cloudposse/atmos/pkg/cache"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

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
