package sign

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var _ = schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: "never"}}

func TestManagedCosignRespectsInstallPolicy(t *testing.T) {
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: "never", InstallPath: t.TempDir()}}
	toolchain.SetAtmosConfig(config)
	_, err := resolveCosign(t.Context(), config, DefaultVersion)
	require.ErrorIs(t, err, errUtils.ErrToolNotInstalled)
	config.Toolchain.Install = "invalid"
	_, err = resolveCosign(t.Context(), config, DefaultVersion)
	require.ErrorIs(t, err, errUtils.ErrInvalidToolchainInstall)
	config.Toolchain.Install = "never"
	name := "cosign"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := toolchain.NewInstaller().GetBinaryPath("sigstore", "cosign", DefaultVersion, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(binary), 0o700))
	require.NoError(t, os.WriteFile(binary, []byte("not executed"), 0o700))
	resolved, err := resolveCosign(t.Context(), config, DefaultVersion)
	require.NoError(t, err)
	require.Equal(t, binary, resolved)
	// Auto mode reuses installed dependencies without network or declaration edits.
	config.Toolchain.Install = "auto"
	resolved, err = resolveCosign(t.Context(), config, DefaultVersion)
	require.NoError(t, err)
	require.Equal(t, binary, resolved)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = resolveCosign(ctx, config, DefaultVersion)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSignerSubprocessCancellation(t *testing.T) {
	binary, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, runCosign(ctx, binary, []string{"-test.run=^$"}), context.Canceled)
}

func TestReadBundleBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.json")
	require.NoError(t, os.WriteFile(path, make([]byte, signature.MaxBundleSize+1), 0o600))
	_, err := readBundle(path)
	require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
	_, err = readBundle(filepath.Dir(path))
	require.Error(t, err)
}
