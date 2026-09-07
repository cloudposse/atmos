package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStartManagedTerraformCache_ExternalCacheIsNoop(t *testing.T) {
	// An externally managed cache (e.g. the mirror's shared proxy) is reused, so the
	// per-component startup is a no-op.
	setup, cleanup, err := startManagedTerraformCache(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{
		TerraformCacheExternal: true,
	})
	require.NoError(t, err)
	assert.Nil(t, setup)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestStartManagedTerraformCache_DisabledIsNoop(t *testing.T) {
	// With no cache configured, tfcache.Start returns nil and the helper does nothing.
	info := &schema.ConfigAndStacksInfo{}
	setup, cleanup, err := startManagedTerraformCache(&schema.AtmosConfiguration{}, info)
	require.NoError(t, err)
	assert.Nil(t, setup)
	assert.Nil(t, info.TerraformCache)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestStartManagedTerraformCache_EnabledStartsProxy(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Cache = &schema.TerraformCacheConfig{
		Enabled:  true,
		Location: t.TempDir(),
	}
	info := &schema.ConfigAndStacksInfo{}

	setup, cleanup, err := startManagedTerraformCache(atmosConfig, info)
	if err != nil {
		// macOS/Windows: the fresh loopback cert is not yet OS-trusted, so VerifyTrust
		// rejects it and the proxy is closed. On Linux/BSD the happy path below runs
		// (the case Codecov's Linux runner exercises).
		require.ErrorIs(t, err, errUtils.ErrCacheCertUntrusted)
		assert.Nil(t, setup)
		return
	}

	require.NotNil(t, setup)
	assert.Same(t, setup, info.TerraformCache)
	t.Cleanup(cleanup)
}
