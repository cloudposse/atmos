package registry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockClient implements the Client interface for testing.
type mockClient struct {
	model     string
	maxTokens int
}

func (m *mockClient) SendMessage(_ context.Context, _ string) (string, error) {
	return "mock response", nil
}

func (m *mockClient) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return nil, nil
}

func (m *mockClient) SendMessageWithHistory(_ context.Context, _ []types.Message) (string, error) {
	return "mock response", nil
}

func (m *mockClient) SendMessageWithToolsAndHistory(_ context.Context, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return nil, nil
}

func (m *mockClient) SendMessageWithSystemPromptAndTools(_ context.Context, _, _ string, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return nil, nil
}

func (m *mockClient) GetModel() string {
	return m.model
}

func (m *mockClient) GetMaxTokens() int {
	return m.maxTokens
}

// Helper to create a test factory.
func createTestFactory(model string, maxTokens int) ClientFactory {
	return func(_ context.Context, _ *schema.AtmosConfiguration) (Client, error) {
		return &mockClient{model: model, maxTokens: maxTokens}, nil
	}
}

// Helper to create a test factory that returns an error.
func createErrorFactory(err error) ClientFactory {
	return func(_ context.Context, _ *schema.AtmosConfiguration) (Client, error) {
		return nil, err
	}
}

// clearRegistry resets the global registry for tests.
// This is needed because the global registry persists between tests.
func clearRegistry() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.providers = make(map[string]ClientFactory)
}

func TestRegister_NewProvider(t *testing.T) {
	clearRegistry()

	factory := createTestFactory("test-model", 4096)
	Register("test-provider", factory)

	assert.True(t, IsProviderRegistered("test-provider"))
	assert.Equal(t, 1, ProviderCount())
}

func TestRegister_MultipleProviders(t *testing.T) {
	clearRegistry()

	Register("provider-a", createTestFactory("model-a", 1024))
	Register("provider-b", createTestFactory("model-b", 2048))
	Register("provider-c", createTestFactory("model-c", 4096))

	assert.True(t, IsProviderRegistered("provider-a"))
	assert.True(t, IsProviderRegistered("provider-b"))
	assert.True(t, IsProviderRegistered("provider-c"))
	assert.Equal(t, 3, ProviderCount())
}

func TestRegister_OverwriteExisting(t *testing.T) {
	clearRegistry()

	Register("test", createTestFactory("model-v1", 1024))
	Register("test", createTestFactory("model-v2", 2048)) // Overwrite

	// Verify the provider was overwritten.
	factory, err := GetFactory("test")
	require.NoError(t, err)

	client, err := factory(context.Background(), &schema.AtmosConfiguration{})
	require.NoError(t, err)

	assert.Equal(t, "model-v2", client.GetModel())
	assert.Equal(t, 2048, client.GetMaxTokens())
	assert.Equal(t, 1, ProviderCount())
}

func TestGetFactory_ExistingProvider(t *testing.T) {
	clearRegistry()

	expectedModel := "test-model-xyz"
	Register("test", createTestFactory(expectedModel, 4096))

	factory, err := GetFactory("test")
	require.NoError(t, err)
	require.NotNil(t, factory)

	client, err := factory(context.Background(), &schema.AtmosConfiguration{})
	require.NoError(t, err)

	assert.Equal(t, expectedModel, client.GetModel())
}

func TestGetFactory_NonExistentProvider(t *testing.T) {
	clearRegistry()

	factory, err := GetFactory("nonexistent")

	assert.Nil(t, factory)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIUnsupportedProvider))
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestGetFactory_ErrorFromFactory(t *testing.T) {
	clearRegistry()

	expectedErr := errors.New("factory error")
	Register("error-provider", createErrorFactory(expectedErr))

	factory, err := GetFactory("error-provider")
	require.NoError(t, err)
	require.NotNil(t, factory)

	client, err := factory(context.Background(), &schema.AtmosConfiguration{})
	assert.Nil(t, client)
	assert.Equal(t, expectedErr, err)
}

func TestListProviders_Empty(t *testing.T) {
	clearRegistry()

	result := ListProviders()
	assert.Empty(t, result)
}

func TestListProviders_SingleProvider(t *testing.T) {
	clearRegistry()

	Register("anthropic", createTestFactory("claude", 4096))

	result := ListProviders()
	assert.Equal(t, "anthropic", result)
}

func TestListProviders_MultipleProviders_Sorted(t *testing.T) {
	clearRegistry()

	// Register in non-alphabetical order.
	Register("openai", createTestFactory("gpt-4", 4096))
	Register("anthropic", createTestFactory("claude", 4096))
	Register("gemini", createTestFactory("gemini-pro", 8192))

	result := ListProviders()

	// Should be sorted alphabetically.
	assert.Equal(t, "anthropic, gemini, openai", result)
}

func TestListProviders_AllBuiltInProviders(t *testing.T) {
	clearRegistry()

	// Register all built-in providers.
	providers := []string{"anthropic", "azureopenai", "bedrock", "gemini", "grok", "ollama", "openai"}
	for _, p := range providers {
		Register(p, createTestFactory(p+"-model", 4096))
	}

	result := ListProviders()
	parts := strings.Split(result, ", ")

	assert.Len(t, parts, 7)
	// Verify alphabetical order.
	assert.Equal(t, []string{"anthropic", "azureopenai", "bedrock", "gemini", "grok", "ollama", "openai"}, parts)
}

func TestIsProviderRegistered_True(t *testing.T) {
	clearRegistry()

	Register("test", createTestFactory("model", 4096))

	assert.True(t, IsProviderRegistered("test"))
}

func TestIsProviderRegistered_False(t *testing.T) {
	clearRegistry()

	assert.False(t, IsProviderRegistered("nonexistent"))
}

func TestIsProviderRegistered_CaseSensitive(t *testing.T) {
	clearRegistry()

	Register("OpenAI", createTestFactory("model", 4096))

	assert.True(t, IsProviderRegistered("OpenAI"))
	assert.False(t, IsProviderRegistered("openai"))
	assert.False(t, IsProviderRegistered("OPENAI"))
}

func TestProviderCount_Empty(t *testing.T) {
	clearRegistry()

	assert.Equal(t, 0, ProviderCount())
}

func TestProviderCount_Multiple(t *testing.T) {
	clearRegistry()

	Register("a", createTestFactory("a", 1024))
	assert.Equal(t, 1, ProviderCount())

	Register("b", createTestFactory("b", 1024))
	assert.Equal(t, 2, ProviderCount())

	Register("c", createTestFactory("c", 1024))
	assert.Equal(t, 3, ProviderCount())
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	clearRegistry()

	const numGoroutines = 100
	var wg sync.WaitGroup

	// Concurrent registrations.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := string(rune('a' + (id % 26)))
			Register(name, createTestFactory(name+"-model", 4096))
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ListProviders()
			_ = ProviderCount()
			_ = IsProviderRegistered("a")
		}()
	}

	wg.Wait()

	// Verify no panics occurred and registry is in a valid state.
	count := ProviderCount()
	assert.True(t, count > 0 && count <= 26) // Up to 26 unique providers (a-z)
}

func TestRegistry_ConcurrentGetFactory(t *testing.T) {
	clearRegistry()

	Register("concurrent-test", createTestFactory("model", 4096))

	const numGoroutines = 50
	var wg sync.WaitGroup
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			factory, err := GetFactory("concurrent-test")
			if err != nil {
				results <- err
				return
			}
			client, err := factory(context.Background(), &schema.AtmosConfiguration{})
			if err != nil {
				results <- err
				return
			}
			if client.GetModel() != "model" {
				results <- errors.New("unexpected model")
				return
			}
			results <- nil
		}()
	}

	wg.Wait()
	close(results)

	// Verify all goroutines succeeded.
	for err := range results {
		assert.NoError(t, err)
	}
}

func TestClient_Interface(t *testing.T) {
	// Verify that mockClient implements the Client interface.
	var _ Client = (*mockClient)(nil)
}

func TestGetFactory_ErrorMessageIncludesAvailable(t *testing.T) {
	clearRegistry()

	Register("anthropic", createTestFactory("claude", 4096))
	Register("openai", createTestFactory("gpt-4", 4096))

	_, err := GetFactory("invalid")

	assert.Error(t, err)
	// Error message should include available providers.
	assert.Contains(t, err.Error(), "anthropic")
	assert.Contains(t, err.Error(), "openai")
}
