package artifact

import (
	"context"
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

// testStore is a minimal Store implementation for testing.
type testStore struct {
	name string
}

func (s *testStore) Name() string { return s.name }
func (s *testStore) Upload(_ context.Context, _ string, _ []FileEntry, _ *Metadata) error {
	return nil
}
func (s *testStore) Download(_ context.Context, _ string) ([]FileResult, *Metadata, error) {
	return nil, nil, nil
}
func (s *testStore) Delete(_ context.Context, _ string) error                     { return nil }
func (s *testStore) List(_ context.Context, _ Query) ([]ArtifactInfo, error)      { return nil, nil }
func (s *testStore) Exists(_ context.Context, _ string) (bool, error)             { return false, nil }
func (s *testStore) GetMetadata(_ context.Context, _ string) (*Metadata, error)   { return nil, nil }

func setupTestRegistry(t *testing.T, storeNames ...string) func() {
	t.Helper()
	registryMu.Lock()
	oldFactories := factories
	factories = make(map[string]StoreFactory)
	registryMu.Unlock()

	for _, name := range storeNames {
		n := name
		Register(n, func(opts StoreOptions) (Store, error) {
			return &testStore{name: n}, nil
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
