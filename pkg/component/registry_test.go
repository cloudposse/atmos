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
	componentType string
	group         string
	commands      []string
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
	return []string{}, nil
}

func (t *testProvider) ValidateComponent(config map[string]any) error {
	return nil
}

func (t *testProvider) Execute(ctx ExecutionContext) error {
	return nil
}

func (t *testProvider) GenerateArtifacts(ctx ExecutionContext) error {
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

	// Concurrent reads.
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok := GetProvider("test")
			assert.True(t, ok)
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
