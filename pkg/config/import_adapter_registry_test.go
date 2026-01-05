package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockTestAdapter is a simple mock adapter for testing the registry.
type mockTestAdapter struct {
	schemes []string
}

func (m *mockTestAdapter) Schemes() []string {
	return m.schemes
}

func (m *mockTestAdapter) Resolve(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ int,
	_ int,
) ([]ResolvedPaths, error) {
	return []ResolvedPaths{{FilePath: "mock", ImportType: ADAPTER}}, nil
}

func TestRegisterImportAdapter_NilAdapter(t *testing.T) {
	ResetImportAdapterRegistry()

	// Should not panic and should be a no-op.
	RegisterImportAdapter(nil)

	adapters := GetRegisteredAdapters()
	assert.Empty(t, adapters)
}

func TestRegisterImportAdapter_WithSchemes(t *testing.T) {
	ResetImportAdapterRegistry()

	adapter := &mockTestAdapter{schemes: []string{"test://"}}
	RegisterImportAdapter(adapter)

	adapters := GetRegisteredAdapters()
	assert.Len(t, adapters, 1)
	assert.Equal(t, []string{"test://"}, adapters[0].Schemes())
}

func TestRegisterImportAdapter_DefaultAdapter(t *testing.T) {
	ResetImportAdapterRegistry()

	// Adapter with no schemes should become the default adapter.
	adapter := &mockTestAdapter{schemes: nil}
	RegisterImportAdapter(adapter)

	adapters := GetRegisteredAdapters()
	assert.Empty(t, adapters) // Should not be in the adapters list.

	defaultAdapter := GetDefaultAdapter()
	assert.NotNil(t, defaultAdapter)
	assert.Nil(t, defaultAdapter.Schemes())
}

func TestSetDefaultAdapter(t *testing.T) {
	ResetImportAdapterRegistry()

	adapter := &mockTestAdapter{schemes: nil}
	SetDefaultAdapter(adapter)

	result := GetDefaultAdapter()
	assert.Equal(t, adapter, result)
}

func TestGetRegisteredAdapters(t *testing.T) {
	ResetImportAdapterRegistry()

	adapter1 := &mockTestAdapter{schemes: []string{"one://"}}
	adapter2 := &mockTestAdapter{schemes: []string{"two://"}}

	RegisterImportAdapter(adapter1)
	RegisterImportAdapter(adapter2)

	adapters := GetRegisteredAdapters()
	assert.Len(t, adapters, 2)
}

func TestGetDefaultAdapter_WhenNotSet(t *testing.T) {
	ResetImportAdapterRegistry()

	result := GetDefaultAdapter()
	assert.Nil(t, result)
}

func TestResetImportAdapterRegistry(t *testing.T) {
	// Register some adapters first.
	adapter := &mockTestAdapter{schemes: []string{"test://"}}
	RegisterImportAdapter(adapter)
	SetDefaultAdapter(&mockTestAdapter{schemes: nil})

	// Reset should clear everything.
	ResetImportAdapterRegistry()

	assert.Empty(t, GetRegisteredAdapters())
	assert.Nil(t, GetDefaultAdapter())
}

func TestFindImportAdapter_NoDefaultAdapter(t *testing.T) {
	ResetImportAdapterRegistry()

	// When no default adapter is set, should return noopAdapter.
	adapter := FindImportAdapter("local/path.yaml")
	assert.NotNil(t, adapter)

	// noopAdapter returns nil for schemes.
	assert.Nil(t, adapter.Schemes())

	// noopAdapter.Resolve returns nil, nil.
	paths, err := adapter.Resolve(context.Background(), "", "", "", 0, 0)
	assert.NoError(t, err)
	assert.Nil(t, paths)
}

func TestFindImportAdapter_MatchesScheme(t *testing.T) {
	ResetImportAdapterRegistry()

	testAdapter := &mockTestAdapter{schemes: []string{"test://"}}
	RegisterImportAdapter(testAdapter)

	adapter := FindImportAdapter("test://something")
	assert.Equal(t, testAdapter, adapter)
}

func TestFindImportAdapter_CaseInsensitive(t *testing.T) {
	ResetImportAdapterRegistry()

	testAdapter := &mockTestAdapter{schemes: []string{"HTTP://"}}
	RegisterImportAdapter(testAdapter)

	// Should match regardless of case.
	adapter := FindImportAdapter("http://example.com")
	assert.Equal(t, testAdapter, adapter)
}

func TestFindImportAdapter_FallsBackToDefault(t *testing.T) {
	ResetImportAdapterRegistry()

	testAdapter := &mockTestAdapter{schemes: []string{"test://"}}
	defaultAdapter := &mockTestAdapter{schemes: nil}

	RegisterImportAdapter(testAdapter)
	SetDefaultAdapter(defaultAdapter)

	// Path without matching scheme should return default adapter.
	adapter := FindImportAdapter("local/path.yaml")
	assert.Equal(t, defaultAdapter, adapter)
}

func TestFindImportAdapter_MultipleSchemes(t *testing.T) {
	ResetImportAdapterRegistry()

	multiSchemeAdapter := &mockTestAdapter{schemes: []string{"http://", "https://"}}
	RegisterImportAdapter(multiSchemeAdapter)

	// Both schemes should match.
	adapter1 := FindImportAdapter("http://example.com")
	adapter2 := FindImportAdapter("https://example.com")

	assert.Equal(t, multiSchemeAdapter, adapter1)
	assert.Equal(t, multiSchemeAdapter, adapter2)
}

func TestSetBuiltinAdaptersInitializer(t *testing.T) {
	ResetImportAdapterRegistry()

	called := false
	testInitFunc := func() {
		called = true
	}

	SetBuiltinAdaptersInitializer(testInitFunc)
	EnsureAdaptersInitialized()

	assert.True(t, called)
}

func TestEnsureAdaptersInitialized_NilInitializer(t *testing.T) {
	ResetImportAdapterRegistry()

	// Set initializer to nil.
	SetBuiltinAdaptersInitializer(nil)

	// Should not panic.
	EnsureAdaptersInitialized()
}

func TestEnsureAdaptersInitialized_CalledOnce(t *testing.T) {
	ResetImportAdapterRegistry()

	callCount := 0
	testInitFunc := func() {
		callCount++
	}

	SetBuiltinAdaptersInitializer(testInitFunc)

	// Call multiple times.
	EnsureAdaptersInitialized()
	EnsureAdaptersInitialized()
	EnsureAdaptersInitialized()

	// Should only be called once due to sync.Once.
	assert.Equal(t, 1, callCount)
}

func TestNoopAdapter_Schemes(t *testing.T) {
	noop := &noopAdapter{}
	assert.Nil(t, noop.Schemes())
}

func TestNoopAdapter_Resolve(t *testing.T) {
	noop := &noopAdapter{}
	paths, err := noop.Resolve(context.Background(), "path", "base", "temp", 1, 10)
	assert.NoError(t, err)
	assert.Nil(t, paths)
}

func TestFindImportAdapter_RegistrationOrder(t *testing.T) {
	ResetImportAdapterRegistry()

	// Register adapters - more specific first.
	specificAdapter := &mockTestAdapter{schemes: []string{"github.com/"}}
	generalAdapter := &mockTestAdapter{schemes: []string{"http://", "https://"}}

	RegisterImportAdapter(specificAdapter)
	RegisterImportAdapter(generalAdapter)

	// GitHub URL should match specific adapter first.
	adapter := FindImportAdapter("github.com/user/repo")
	assert.Equal(t, specificAdapter, adapter)
}
