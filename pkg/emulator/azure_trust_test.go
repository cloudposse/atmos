package emulator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/cacerts"
)

func TestBuildAzureTrustBundle_AppendsFlociCertToBaseBundle(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base.pem")
	require.NoError(t, os.WriteFile(base, []byte("system-root\n"), trustBundlePerm))
	t.Setenv(cacerts.EnvSSLCertFile, base)

	dir := t.TempDir()
	cert := filepath.Join(dir, flociAzureCertPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(cert), 0o755))
	require.NoError(t, os.WriteFile(cert, []byte("floci-root\n"), trustBundlePerm))

	bundle := filepath.Join(dir, azureTrustBundle)
	got, ok, err := buildAzureTrustBundle(cert, bundle)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, bundle, got)

	data, err := os.ReadFile(bundle)
	require.NoError(t, err)
	assert.Equal(t, "system-root\n\nfloci-root\n", string(data))
}

func TestBuildAzureTrustBundle_MissingCertIsNoOp(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base.pem")
	require.NoError(t, os.WriteFile(base, []byte("system-root\n"), trustBundlePerm))
	t.Setenv(cacerts.EnvSSLCertFile, base)

	got, ok, err := buildAzureTrustBundle(filepath.Join(t.TempDir(), flociAzureCertPath), filepath.Join(t.TempDir(), azureTrustBundle))
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, got)
}

func TestAddAzureTrustEnv_WritesBundleInInstanceDataDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base.pem")
	require.NoError(t, os.WriteFile(base, []byte("system-root\n"), trustBundlePerm))
	t.Setenv(cacerts.EnvSSLCertFile, base)
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")

	dataDir, err := InstanceDataDir("dev", "azure")
	require.NoError(t, err)
	cert := filepath.Join(dataDir, flociAzureCertPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(cert), 0o755))
	require.NoError(t, os.WriteFile(cert, []byte("floci-root\n"), trustBundlePerm))

	profile := &Profile{}
	require.NoError(t, addAzureTrustEnv(profile, "dev", "azure"))

	bundle := filepath.Join(dataDir, azureTrustBundle)
	assert.Equal(t, bundle, profile.Env[cacerts.EnvSSLCertFile])
	require.FileExists(t, bundle)
}
