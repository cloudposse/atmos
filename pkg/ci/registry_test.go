package ci

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// mockProvider is a mock CI provider for testing.
type mockProvider struct {
	name     string
	detected bool
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Detect() bool { return m.detected }

func (m *mockProvider) Context() (*Context, error) {
	return &Context{
		Provider:   m.name,
		RunID:      "123",
		Repository: "owner/repo",
		RepoOwner:  "owner",
		RepoName:   "repo",
		SHA:        "abc123",
	}, nil
}

func (m *mockProvider) GetStatus(_ context.Context, _ StatusOptions) (*Status, error) {
	return &Status{}, nil
}

func (m *mockProvider) CreateCheckRun(_ context.Context, _ CreateCheckRunOptions) (*CheckRun, error) {
	return &CheckRun{ID: 1}, nil
}

func (m *mockProvider) UpdateCheckRun(_ context.Context, _ UpdateCheckRunOptions) (*CheckRun, error) {
	return &CheckRun{ID: 1}, nil
}

func (m *mockProvider) OutputWriter() OutputWriter {
	return nil
}

func TestRegisterAndGet(t *testing.T) {
	// Clear providers for isolated test.
	providersMu.Lock()
	originalProviders := providers
	providers = make(map[string]Provider)
	providersMu.Unlock()

	defer func() {
		providersMu.Lock()
		providers = originalProviders
		providersMu.Unlock()
	}()

	mock := &mockProvider{name: "test-provider", detected: false}
	Register(mock)

	// Test Get.
	p, err := Get("test-provider")
	assert.NoError(t, err)
	assert.Equal(t, "test-provider", p.Name())

	// Test Get not found.
	_, err = Get("nonexistent")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrCIProviderNotFound))
}

func TestDetect(t *testing.T) {
	// Clear providers for isolated test.
	providersMu.Lock()
	originalProviders := providers
	providers = make(map[string]Provider)
	providersMu.Unlock()

	defer func() {
		providersMu.Lock()
		providers = originalProviders
		providersMu.Unlock()
	}()

	// Test no providers detected.
	p := Detect()
	assert.Nil(t, p)

	// Register a non-detected provider.
	notDetected := &mockProvider{name: "not-detected", detected: false}
	Register(notDetected)
	p = Detect()
	assert.Nil(t, p)

	// Register a detected provider.
	detected := &mockProvider{name: "detected", detected: true}
	Register(detected)
	p = Detect()
	assert.NotNil(t, p)
	assert.Equal(t, "detected", p.Name())
}

func TestDetectOrError(t *testing.T) {
	// Clear providers for isolated test.
	providersMu.Lock()
	originalProviders := providers
	providers = make(map[string]Provider)
	providersMu.Unlock()

	defer func() {
		providersMu.Lock()
		providers = originalProviders
		providersMu.Unlock()
	}()

	// Test no providers detected.
	_, err := DetectOrError()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrCIProviderNotDetected))

	// Register a detected provider.
	detected := &mockProvider{name: "detected", detected: true}
	Register(detected)
	p, err := DetectOrError()
	assert.NoError(t, err)
	assert.Equal(t, "detected", p.Name())
}

func TestList(t *testing.T) {
	// Clear providers for isolated test.
	providersMu.Lock()
	originalProviders := providers
	providers = make(map[string]Provider)
	providersMu.Unlock()

	defer func() {
		providersMu.Lock()
		providers = originalProviders
		providersMu.Unlock()
	}()

	// Empty list.
	names := List()
	assert.Empty(t, names)

	// Register providers.
	Register(&mockProvider{name: "provider1", detected: false})
	Register(&mockProvider{name: "provider2", detected: false})

	names = List()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "provider1")
	assert.Contains(t, names, "provider2")
}

func TestIsCI(t *testing.T) {
	// Clear providers for isolated test.
	providersMu.Lock()
	originalProviders := providers
	providers = make(map[string]Provider)
	providersMu.Unlock()

	defer func() {
		providersMu.Lock()
		providers = originalProviders
		providersMu.Unlock()
	}()

	// Not in CI.
	assert.False(t, IsCI())

	// In CI.
	Register(&mockProvider{name: "ci", detected: true})
	assert.True(t, IsCI())
}
