package artifact

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// testChecker is a simple EnvironmentChecker for testing.
type testChecker struct {
	available bool
}

func (c *testChecker) IsAvailable(_ context.Context) bool {
	return c.available
}

// testBackend is a minimal Backend implementation for testing.
type testBackend struct {
	name string
}

func (s *testBackend) Name() string { return s.name }
func (s *testBackend) Upload(_ context.Context, _ string, _ io.Reader, _ int64, _ *Metadata) error {
	return nil
}
func (s *testBackend) Download(_ context.Context, _ string) (io.ReadCloser, *Metadata, error) {
	return nil, nil, nil
}
func (s *testBackend) Delete(_ context.Context, _ string) error                     { return nil }
func (s *testBackend) List(_ context.Context, _ Query) ([]ArtifactInfo, error)      { return nil, nil }
func (s *testBackend) Exists(_ context.Context, _ string) (bool, error)             { return false, nil }
func (s *testBackend) GetMetadata(_ context.Context, _ string) (*Metadata, error)   { return nil, nil }

func setupTestRegistry(t *testing.T, storeNames ...string) func() {
	t.Helper()
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]BackendFactory)
	registryMu.Unlock()

	for _, name := range storeNames {
		n := name
		Register(n, func(opts StoreOptions) (Backend, error) {
			return &testBackend{name: n}, nil
		})
	}

	return func() {
		registryMu.Lock()
		factories = oldFactories
		registryMu.Unlock()
	}
}

func TestSelectStoreExplicitOverride(t *testing.T) {
	cleanup := setupTestRegistry(t, "s3", "gcs")
	defer cleanup()

	stores := map[string]StoreOptions{
		"my-s3":  {Type: "s3"},
		"my-gcs": {Type: "gcs"},
	}

	store, err := SelectStore(context.Background(), []string{"my-gcs", "my-s3"}, stores, nil, "my-s3", nil)
	require.NoError(t, err)
	assert.Equal(t, "s3", store.Name())
}

func TestSelectStorePrioritySelection(t *testing.T) {
	cleanup := setupTestRegistry(t, "s3", "gcs")
	defer cleanup()

	stores := map[string]StoreOptions{
		"primary":  {Type: "s3"},
		"fallback": {Type: "gcs"},
	}
	checkers := map[string]EnvironmentChecker{
		"primary":  &testChecker{available: false},
		"fallback": &testChecker{available: true},
	}

	store, err := SelectStore(context.Background(), []string{"primary", "fallback"}, stores, checkers, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "gcs", store.Name())
}

func TestSelectStorePriorityFirstAvailable(t *testing.T) {
	cleanup := setupTestRegistry(t, "s3", "gcs")
	defer cleanup()

	stores := map[string]StoreOptions{
		"primary":  {Type: "s3"},
		"fallback": {Type: "gcs"},
	}
	checkers := map[string]EnvironmentChecker{
		"primary":  &testChecker{available: true},
		"fallback": &testChecker{available: true},
	}

	store, err := SelectStore(context.Background(), []string{"primary", "fallback"}, stores, checkers, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "s3", store.Name())
}

func TestSelectStoreNoAvailableStore(t *testing.T) {
	cleanup := setupTestRegistry(t, "s3")
	defer cleanup()

	stores := map[string]StoreOptions{
		"primary": {Type: "s3"},
	}
	checkers := map[string]EnvironmentChecker{
		"primary": &testChecker{available: false},
	}

	_, err := SelectStore(context.Background(), []string{"primary"}, stores, checkers, "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactStoreNotFound)
}

func TestSelectStoreExplicitNotConfigured(t *testing.T) {
	stores := map[string]StoreOptions{}

	_, err := SelectStore(context.Background(), nil, stores, nil, "nonexistent", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactStoreNotFound)
}

func TestSelectStoreNoCheckerMeansAvailable(t *testing.T) {
	cleanup := setupTestRegistry(t, "s3")
	defer cleanup()

	stores := map[string]StoreOptions{
		"primary": {Type: "s3"},
	}

	// No checkers — store should be considered available.
	store, err := SelectStore(context.Background(), []string{"primary"}, stores, nil, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "s3", store.Name())
}
