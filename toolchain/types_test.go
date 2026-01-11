package toolchain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/toolchain/registry"
)

func TestNewInstaller(t *testing.T) {
	t.Run("creates installer with default options", func(t *testing.T) {
		installer := NewInstaller()
		assert.NotNil(t, installer)
	})

	t.Run("creates installer with custom binDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		installer := NewInstaller(WithBinDir(tmpDir))
		assert.NotNil(t, installer)
	})

	t.Run("creates installer with custom cacheDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		installer := NewInstaller(WithCacheDir(tmpDir))
		assert.NotNil(t, installer)
	})

	t.Run("creates installer with multiple options", func(t *testing.T) {
		binDir := t.TempDir()
		cacheDir := t.TempDir()
		installer := NewInstaller(
			WithBinDir(binDir),
			WithCacheDir(cacheDir),
		)
		assert.NotNil(t, installer)
	})
}

func TestNewInstallerWithBinDir(t *testing.T) {
	tmpDir := t.TempDir()
	installer := NewInstallerWithBinDir(tmpDir)
	require.NotNil(t, installer)
}

func TestNewInstallerWithResolver(t *testing.T) {
	tmpDir := t.TempDir()

	// Use the existing mock resolver from mock_resolver_test.go.
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
		},
	}

	installer := NewInstallerWithResolver(mockResolver, tmpDir)
	require.NotNil(t, installer)
}

func TestWithBinDir(t *testing.T) {
	opt := WithBinDir("/custom/bin")
	assert.NotNil(t, opt)
}

func TestWithCacheDir(t *testing.T) {
	opt := WithCacheDir("/custom/cache")
	assert.NotNil(t, opt)
}

func TestWithResolver(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{},
	}
	opt := WithResolver(mockResolver)
	assert.NotNil(t, opt)
}

func TestWithConfiguredRegistry(t *testing.T) {
	// Create a mock registry that implements all required methods.
	mockRegistry := &mockToolRegistryForTypes{}
	opt := WithConfiguredRegistry(mockRegistry)
	assert.NotNil(t, opt)
}

func TestWithRegistryFactory(t *testing.T) {
	factory := &realRegistryFactory{}
	opt := WithRegistryFactory(factory)
	assert.NotNil(t, opt)
}

func TestRealRegistryFactory_NewAquaRegistry(t *testing.T) {
	// Set XDG_CACHE_HOME to temp dir to avoid writing to real user cache
	// and ensure hermetic, reproducible test behavior.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	factory := &realRegistryFactory{}
	reg := factory.NewAquaRegistry()
	assert.NotNil(t, reg)
}

func TestBuiltinAliases(t *testing.T) {
	// Verify builtin aliases are available.
	assert.NotNil(t, BuiltinAliases)

	// Verify the expected atmos alias exists.
	// Currently, BuiltinAliases only contains "atmos" -> "cloudposse/atmos".
	_, exists := BuiltinAliases["atmos"]
	assert.True(t, exists, "Expected builtin alias 'atmos' to exist")
}

// mockToolRegistryForTypes implements registry.ToolRegistry for testing.
type mockToolRegistryForTypes struct{}

func (m *mockToolRegistryForTypes) GetTool(owner, repo string) (*registry.Tool, error) {
	return nil, nil
}

func (m *mockToolRegistryForTypes) GetToolWithVersion(owner, repo, version string) (*registry.Tool, error) {
	return nil, nil
}

func (m *mockToolRegistryForTypes) GetLatestVersion(owner, repo string) (string, error) {
	return "1.0.0", nil
}

func (m *mockToolRegistryForTypes) LoadLocalConfig(configPath string) error {
	return nil
}

func (m *mockToolRegistryForTypes) Search(ctx context.Context, query string, opts ...registry.SearchOption) ([]*registry.Tool, error) {
	return nil, nil
}

func (m *mockToolRegistryForTypes) ListAll(ctx context.Context, opts ...registry.ListOption) ([]*registry.Tool, error) {
	return nil, nil
}

func (m *mockToolRegistryForTypes) GetMetadata(ctx context.Context) (*registry.RegistryMetadata, error) {
	return &registry.RegistryMetadata{Name: "mock"}, nil
}
