package component

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// testProvider is a minimal provider implementation for testing.
type testProvider struct {
	componentType  string
	group          string
	commands       []string
	components     []string
	listError      error
	listByStackMap map[string][]string // stack -> components for per-stack testing.
}

func (t *testProvider) GetType() string {
	return t.componentType
}

func (t *testProvider) GetGroup() string {
	return t.group
}

func (t *testProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	return "components/" + t.componentType
}

func (t *testProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
	if t.listError != nil {
		return nil, t.listError
	}
	if t.listByStackMap != nil {
		if comps, ok := t.listByStackMap[stack]; ok {
			return comps, nil
		}
		return []string{}, nil
	}
	return t.components, nil
}

func (t *testProvider) ValidateComponent(config map[string]any) error {
	return nil
}

func (t *testProvider) Execute(ctx *ExecutionContext) error {
	return nil
}

func (t *testProvider) GenerateArtifacts(ctx *ExecutionContext) error {
	return nil
}

func (t *testProvider) GetAvailableCommands() []string {
	return t.commands
}

func TestRegister(t *testing.T) {
	Reset()

	provider := &testProvider{
		componentType: "test",
		group:         "Testing",
		commands:      []string{"plan", "apply"},
	}

	err := Register(provider)
	require.NoError(t, err)

	assert.Equal(t, 1, Count())

	retrieved, ok := GetProvider("test")
	assert.True(t, ok)
	assert.Equal(t, "test", retrieved.GetType())
}

func TestRegisterErrors(t *testing.T) {
	Reset()

	tests := []struct {
		name        string
		provider    ComponentProvider
		expectError error
	}{
		{
			name:        "nil provider",
			provider:    nil,
			expectError: errUtils.ErrComponentProviderNil,
		},
		{
			name: "empty component type",
			provider: &testProvider{
				componentType: "",
				group:         "Testing",
			},
			expectError: errUtils.ErrComponentTypeEmpty,
		},
		{
			name: "valid provider",
			provider: &testProvider{
				componentType: "valid",
				group:         "Testing",
			},
			expectError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Register(tt.provider)
			if tt.expectError != nil {
				assert.ErrorIs(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetProvider(t *testing.T) {
	Reset()

	provider := &testProvider{
		componentType: "test",
		group:         "Testing",
	}

	err := Register(provider)
	require.NoError(t, err)

	// Found case.
	retrieved, ok := GetProvider("test")
	assert.True(t, ok)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test", retrieved.GetType())

	// Not found case.
	_, ok = GetProvider("nonexistent")
	assert.False(t, ok)
}

func TestListTypes(t *testing.T) {
	Reset()

	// Empty registry.
	types := ListTypes()
	assert.Empty(t, types)

	// Register multiple providers.
	require.NoError(t, Register(&testProvider{componentType: "zulu", group: "Group A"}))
	require.NoError(t, Register(&testProvider{componentType: "alpha", group: "Group B"}))
	require.NoError(t, Register(&testProvider{componentType: "bravo", group: "Group A"}))

	types = ListTypes()
	assert.Equal(t, []string{"alpha", "bravo", "zulu"}, types) // Sorted.
}

func TestListProviders(t *testing.T) {
	Reset()

	// Register providers in different groups.
	require.NoError(t, Register(&testProvider{componentType: "terraform", group: "Infrastructure as Code"}))
	require.NoError(t, Register(&testProvider{componentType: "helmfile", group: "Kubernetes"}))
	require.NoError(t, Register(&testProvider{componentType: "mock", group: "Testing"}))
	require.NoError(t, Register(&testProvider{componentType: "packer", group: "Infrastructure as Code"}))

	grouped := ListProviders()

	assert.Len(t, grouped, 3) // Three groups.
	assert.Len(t, grouped["Infrastructure as Code"], 2)
	assert.Len(t, grouped["Kubernetes"], 1)
	assert.Len(t, grouped["Testing"], 1)
}

func TestCount(t *testing.T) {
	Reset()

	assert.Equal(t, 0, Count())

	require.NoError(t, Register(&testProvider{componentType: "test1", group: "Group A"}))
	assert.Equal(t, 1, Count())

	require.NoError(t, Register(&testProvider{componentType: "test2", group: "Group B"}))
	assert.Equal(t, 2, Count())

	require.NoError(t, Register(&testProvider{componentType: "test3", group: "Group A"}))
	assert.Equal(t, 3, Count())
}

func TestGetInfo(t *testing.T) {
	Reset()

	require.NoError(t, Register(&testProvider{
		componentType: "test1",
		group:         "Group A",
		commands:      []string{"plan", "apply"},
	}))
	require.NoError(t, Register(&testProvider{
		componentType: "test2",
		group:         "Group B",
		commands:      []string{"deploy"},
	}))

	infos := GetInfo()

	assert.Len(t, infos, 2)

	// Verify sorting by type.
	assert.Equal(t, "test1", infos[0].Type)
	assert.Equal(t, "test2", infos[1].Type)

	// Verify metadata.
	assert.Equal(t, "Group A", infos[0].Group)
	assert.Equal(t, []string{"plan", "apply"}, infos[0].Commands)
}

func TestReset(t *testing.T) {
	Reset()

	require.NoError(t, Register(&testProvider{componentType: "test1", group: "Group A"}))
	require.NoError(t, Register(&testProvider{componentType: "test2", group: "Group B"}))

	assert.Equal(t, 2, Count())

	Reset()

	assert.Equal(t, 0, Count())
	assert.Empty(t, ListTypes())
	assert.Empty(t, ListProviders())
}

func TestMustGetProvider(t *testing.T) {
	Reset()

	provider := &testProvider{componentType: "test", group: "Testing"}
	require.NoError(t, Register(provider))

	// Found case - should not panic.
	retrieved := MustGetProvider("test")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test", retrieved.GetType())
}

func TestMustGetProviderPanic(t *testing.T) {
	Reset()

	defer func() {
		r := recover()
		require.NotNil(t, r)
		err, ok := r.(error)
		require.True(t, ok)
		assert.ErrorIs(t, err, errUtils.ErrComponentProviderNotFound)
	}()

	MustGetProvider("nonexistent")
	t.Fatal("should have panicked")
}

func TestDuplicateRegistration(t *testing.T) {
	Reset()

	provider1 := &testProvider{componentType: "test", group: "Group A"}
	provider2 := &testProvider{componentType: "test", group: "Group B"}

	require.NoError(t, Register(provider1))
	assert.Equal(t, 1, Count())

	// Re-registration should not error (allows override).
	require.NoError(t, Register(provider2))
	assert.Equal(t, 1, Count())

	// Latest registration wins.
	retrieved, ok := GetProvider("test")
	assert.True(t, ok)
	assert.Equal(t, "Group B", retrieved.GetGroup())
}

// TestConcurrentRegistration verifies thread safety during concurrent registration.
func TestConcurrentRegistration(t *testing.T) {
	Reset()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Test concurrent registration doesn't cause race conditions.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			provider := &testProvider{
				componentType: fmt.Sprintf("test-%d", id),
				group:         "Testing",
			}
			_ = Register(provider) // Ignore error in goroutine.
		}(i)
	}

	wg.Wait()
	assert.Equal(t, numGoroutines, Count())
}

// TestConcurrentReadWrite verifies thread safety during concurrent reads and writes.
func TestConcurrentReadWrite(t *testing.T) {
	Reset()
	require.NoError(t, Register(&testProvider{componentType: "test", group: "Testing"}))

	var wg sync.WaitGroup
	numReaders := 50
	numWriters := 10
	errCh := make(chan error, numReaders)

	// Concurrent reads.
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := GetProvider("test"); !ok {
				errCh <- fmt.Errorf("provider 'test' not found")
			}
		}()
	}

	// Concurrent writes.
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			provider := &testProvider{
				componentType: fmt.Sprintf("concurrent-%d", id),
				group:         "Testing",
			}
			_ = Register(provider) // Ignore error in goroutine.
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	// Should have original + 10 new providers.
	assert.Equal(t, 1+numWriters, Count())
}

// TestEmptyRegistry verifies operations on empty registry don't panic.
func TestEmptyRegistry(t *testing.T) {
	Reset()

	assert.Equal(t, 0, Count())
	assert.Empty(t, ListTypes())
	assert.Empty(t, ListProviders())
	assert.Empty(t, GetInfo())

	_, ok := GetProvider("nonexistent")
	assert.False(t, ok)
}

// TestNilChecks verifies nil checks are handled properly.
func TestNilChecks(t *testing.T) {
	Reset()

	// Register with nil provider should error.
	err := Register(nil)
	assert.ErrorIs(t, err, errUtils.ErrComponentProviderNil)

	// Register with empty type should error.
	provider := &testProvider{
		componentType: "",
		group:         "",
		commands:      nil,
	}
	err = Register(provider)
	assert.ErrorIs(t, err, errUtils.ErrComponentTypeEmpty)
}

func TestListAllComponents(t *testing.T) {
	tests := []struct {
		name          string
		componentType string
		provider      *testProvider
		stacksMap     map[string]any
		want          []string
		wantErr       error
	}{
		{
			name:          "returns sorted components from single stack",
			componentType: "script",
			provider: &testProvider{
				componentType: "script",
				group:         "Custom",
				components:    []string{"deploy-app", "build-image"},
			},
			stacksMap: map[string]any{
				"dev": map[string]any{},
			},
			want: []string{"build-image", "deploy-app"},
		},
		{
			name:          "deduplicates components across stacks",
			componentType: "script",
			provider: &testProvider{
				componentType: "script",
				group:         "Custom",
				listByStackMap: map[string][]string{
					"dev":     {"deploy-app", "build-image"},
					"staging": {"deploy-app", "run-tests"},
					"prod":    {"deploy-app"},
				},
			},
			stacksMap: map[string]any{
				"dev":     map[string]any{},
				"staging": map[string]any{},
				"prod":    map[string]any{},
			},
			want: []string{"build-image", "deploy-app", "run-tests"},
		},
		{
			name:          "unknown provider returns error",
			componentType: "unknown",
			provider:      nil,
			stacksMap:     map[string]any{},
			wantErr:       errUtils.ErrComponentProviderNotFound,
		},
		{
			name:          "empty stacks map returns empty list",
			componentType: "script",
			provider: &testProvider{
				componentType: "script",
				group:         "Custom",
				components:    []string{"comp1"},
			},
			stacksMap: map[string]any{},
			want:      []string{},
		},
		{
			name:          "skips stacks with invalid config type",
			componentType: "script",
			provider: &testProvider{
				componentType: "script",
				group:         "Custom",
				listByStackMap: map[string][]string{
					"dev": {"valid-component"},
				},
			},
			stacksMap: map[string]any{
				"dev":     map[string]any{}, // Valid.
				"invalid": "not-a-map",      // Invalid type.
				"also":    123,              // Also invalid.
			},
			want: []string{"valid-component"},
		},
		{
			name:          "gracefully handles ListComponents errors",
			componentType: "script",
			provider: &testProvider{
				componentType: "script",
				group:         "Custom",
				listError:     fmt.Errorf("mock error"),
			},
			stacksMap: map[string]any{
				"dev": map[string]any{},
			},
			want: []string{}, // Graceful degradation.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Reset()

			if tt.provider != nil {
				require.NoError(t, Register(tt.provider))
			}

			got, err := ListAllComponents(context.Background(), tt.componentType, tt.stacksMap)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
