package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewInstaller(t *testing.T) {
	t.Run("creates installer with default options", func(t *testing.T) {
		installer := NewInstaller()
		assert.NotNil(t, installer)
		// Default binDir is set from GetInstallPath() + "/bin".
		assert.NotEmpty(t, installer.GetBinDir(), "binDir should have a default value")
	})

	t.Run("creates installer with custom binDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		installer := NewInstaller(WithBinDir(tmpDir))
		assert.NotNil(t, installer)
		assert.Equal(t, tmpDir, installer.GetBinDir(), "binDir should match the provided value")
	})

	t.Run("creates installer with custom cacheDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		installer := NewInstaller(WithCacheDir(tmpDir))
		assert.NotNil(t, installer)
		// cacheDir is unexported, but we verify the installer was created successfully.
		// The option is applied internally and affects download behavior.
	})

	t.Run("creates installer with multiple options", func(t *testing.T) {
		binDir := t.TempDir()
		cacheDir := t.TempDir()
		installer := NewInstaller(
			WithBinDir(binDir),
			WithCacheDir(cacheDir),
		)
		assert.NotNil(t, installer)
		assert.Equal(t, binDir, installer.GetBinDir(), "binDir should match the provided value")
	})
}

func TestNewInstallerWithBinDir(t *testing.T) {
	tmpDir := t.TempDir()
	installer := NewInstallerWithBinDir(tmpDir)
	require.NotNil(t, installer)
	assert.Equal(t, tmpDir, installer.GetBinDir(), "binDir should match the provided value")
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
	// Verify binDir is set correctly.
	assert.Equal(t, tmpDir, installer.GetBinDir(), "binDir should match the provided value")
	// The resolver is unexported but is used internally during ParseToolSpec.
	// We verify it works by testing the parsing behavior.
	owner, repo, err := installer.ParseToolSpec("terraform")
	require.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)
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
	// Create a mock registry using the generated mock.
	ctrl := gomock.NewController(t)
	mockRegistry := NewMockToolRegistry(ctrl)
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
