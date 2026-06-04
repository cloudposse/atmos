package ci

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// mockProvider is a mock CI provider for testing.
type mockProvider struct {
	name     string
	detected bool
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Detect() bool { return m.detected }

func (m *mockProvider) Context() (*provider.Context, error) {
	return &provider.Context{
		Provider:   m.name,
		RunID:      "123",
		Repository: "owner/repo",
		RepoOwner:  "owner",
		RepoName:   "repo",
		SHA:        "abc123",
	}, nil
}

func (m *mockProvider) GetStatus(_ context.Context, _ provider.StatusOptions) (*provider.Status, error) {
	return &provider.Status{}, nil
}

func (m *mockProvider) CreateCheckRun(_ context.Context, _ *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	return &provider.CheckRun{ID: 1}, nil
}

func (m *mockProvider) UpdateCheckRun(_ context.Context, _ *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	return &provider.CheckRun{ID: 1}, nil
}

func (m *mockProvider) PostComment(_ context.Context, _ *provider.PostCommentOptions) (*provider.Comment, error) {
	return &provider.Comment{ID: 1}, nil
}

func (m *mockProvider) OutputWriter() provider.OutputWriter {
	return nil
}

func (m *mockProvider) ResolveBase() (*provider.BaseResolution, error) {
	return nil, nil
}

func TestRegisterAndGet(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

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
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

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
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

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
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

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
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	// Not in CI.
	assert.False(t, IsCI())

	// In CI.
	Register(&mockProvider{name: "ci", detected: true})
	assert.True(t, IsCI())
}

// debugMockProvider extends mockProvider with the optional
// DebugModeDetector capability for DetectDebugMode tests.
type debugMockProvider struct {
	mockProvider
	debug bool
}

func (m *debugMockProvider) IsDebugMode() bool { return m.debug }

func TestDetectDebugMode(t *testing.T) {
	t.Run("no provider detected -> zero value", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		info := DetectDebugMode()
		assert.False(t, info.Active)
		assert.Empty(t, info.Provider)
	})

	t.Run("detected provider without capability -> Active=false, Provider set", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		Register(&mockProvider{name: "plain-ci", detected: true})
		info := DetectDebugMode()
		assert.False(t, info.Active)
		assert.Equal(t, "plain-ci", info.Provider)
	})

	t.Run("detected provider with capability, debug off -> Active=false", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		Register(&debugMockProvider{
			mockProvider: mockProvider{name: "debug-ci", detected: true},
			debug:        false,
		})
		info := DetectDebugMode()
		assert.False(t, info.Active)
		assert.Equal(t, "debug-ci", info.Provider)
	})

	t.Run("detected provider with capability, debug on -> Active=true", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		Register(&debugMockProvider{
			mockProvider: mockProvider{name: "debug-ci", detected: true},
			debug:        true,
		})
		info := DetectDebugMode()
		assert.True(t, info.Active)
		assert.Equal(t, "debug-ci", info.Provider)
	})
}

// testSaveAndClearRegistry clears the provider registry and returns a restore function.
func testSaveAndClearRegistry() func() {
	return SwapRegistryForTest()
}

// testRestoreRegistry restores the provider registry from a previous snapshot.
func testRestoreRegistry(restore func()) {
	restore()
}
