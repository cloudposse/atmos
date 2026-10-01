package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/installer"
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
	atmosOwnerRepo, exists := BuiltinAliases["atmos"]
	assert.True(t, exists, "Expected builtin alias 'atmos' to exist")
	assert.Equal(t, "cloudposse/atmos", atmosOwnerRepo)

	// Verify the expected tofu alias exists (explicit rather than relying on
	// aqua-registry short-name search -- see BuiltinAliases' doc comment).
	tofuOwnerRepo, exists := BuiltinAliases["tofu"]
	assert.True(t, exists, "Expected builtin alias 'tofu' to exist")
	assert.Equal(t, "opentofu/opentofu", tofuOwnerRepo)
}

// TestDefaultToolResolver_BuiltinAliasResolution exercises the builtin-alias
// branch of Resolve itself (installer.go step 1b), not just the map entry:
// TestDefaultToolResolver_AliasResolution's "tofu" case supplies the same
// mapping through AtmosConfig.Toolchain.Aliases (the user-alias branch, step
// 1), so it would not catch a regression that broke the builtin fallback.
func TestDefaultToolResolver_BuiltinAliasResolution(t *testing.T) {
	resolver := &installer.DefaultToolResolver{
		AtmosConfig: &schema.AtmosConfiguration{},
	}

	owner, repo, err := resolver.Resolve("tofu")
	require.NoError(t, err)
	assert.Equal(t, "opentofu", owner)
	assert.Equal(t, "opentofu", repo)
}
