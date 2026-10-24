package release

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestRunOwnsLifecycleAndCleansTemporaryLayout(t *testing.T) {
	backend := NewMockBackend(gomock.NewController(t))
	parent := t.TempDir()
	expected := &pluginoci.Release{Name: "example/test", Version: "1.0.0"}
	var staged string
	gomock.InOrder(
		backend.EXPECT().Validate(gomock.Any(), "source"),
		backend.EXPECT().Build(gomock.Any(), "source"),
		backend.EXPECT().Test(gomock.Any(), "source"),
		backend.EXPECT().Pack(gomock.Any(), "source", gomock.Any()).DoAndReturn(func(_ context.Context, _, layout string) error {
			staged = layout
			require.DirExists(t, filepath.Dir(layout))
			require.NoDirExists(t, layout)
			return os.Mkdir(layout, 0o700)
		}),
		backend.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, layout string) (*pluginoci.Release, error) {
			require.Equal(t, staged, layout)
			require.DirExists(t, layout)
			return expected, nil
		}),
	)
	actual, err := Run(t.Context(), "source", backend, WithTemporaryDirectory(parent))
	require.NoError(t, err)
	require.Same(t, expected, actual)
	require.NoDirExists(t, filepath.Dir(staged))
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func releaseExpectations(backend *MockBackend) []func() *gomock.Call {
	return []func() *gomock.Call{
		func() *gomock.Call { return backend.EXPECT().Validate(gomock.Any(), "source") },
		func() *gomock.Call { return backend.EXPECT().Build(gomock.Any(), "source") },
		func() *gomock.Call { return backend.EXPECT().Test(gomock.Any(), "source") },
		func() *gomock.Call { return backend.EXPECT().Pack(gomock.Any(), "source", gomock.Any()) },
		func() *gomock.Call { return backend.EXPECT().Publish(gomock.Any(), gomock.Any()) },
	}
}

func TestRunStopsAtFailureWithoutPublishing(t *testing.T) {
	for index, phase := range []string{"validate", "build", "test", "pack", "publish"} {
		t.Run(phase, func(t *testing.T) {
			backend := NewMockBackend(gomock.NewController(t))
			parent := t.TempDir()
			failure := errUtils.ErrPluginBuild
			expectations := releaseExpectations(backend)
			for _, expect := range expectations[:index] {
				expect()
			}
			failed := expectations[index]()
			if phase == "publish" {
				failed.Return(nil, failure)
			} else {
				failed.Return(failure)
			}
			actual, err := Run(t.Context(), "source", backend, WithTemporaryDirectory(parent))
			require.Nil(t, actual)
			require.ErrorIs(t, err, failure)
			entries, err := os.ReadDir(parent)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestRunCancellationPreventsFollowingOperations(t *testing.T) {
	for index, phase := range []string{"before", "validate", "build", "test", "pack"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			backend := NewMockBackend(gomock.NewController(t))
			parent := t.TempDir()
			expectations := releaseExpectations(backend)
			for step, expect := range expectations[:index] {
				call := expect()
				if step != index-1 {
					continue
				}
				if phase == "pack" {
					call.Do(func(context.Context, string, string) { cancel() })
				} else {
					call.Do(func(context.Context, string) { cancel() })
				}
			}
			if index == 0 {
				cancel()
			}
			_, err := Run(ctx, "source", backend, WithTemporaryDirectory(parent))
			require.ErrorIs(t, err, context.Canceled)
			entries, err := os.ReadDir(parent)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestRunRejectsMissingBackendAndUnavailableStagingDirectory(t *testing.T) {
	_, err := Run(t.Context(), ".", nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	backend := NewMockBackend(gomock.NewController(t))
	backend.EXPECT().Validate(gomock.Any(), ".")
	backend.EXPECT().Build(gomock.Any(), ".")
	backend.EXPECT().Test(gomock.Any(), ".")
	_, err = Run(t.Context(), ".", backend, WithTemporaryDirectory(filepath.Join(t.TempDir(), "absent")))
	require.ErrorIs(t, err, os.ErrNotExist)
}
