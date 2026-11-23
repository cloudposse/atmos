package autoinit

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/terraform/lockfile"
)

func TestAlternateArchitectureKeepsWarmInitAndBoundedRecovery(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, ".terraform")
	require.NoError(t, os.WriteFile(filepath.Join(dir, lockfile.Name), []byte(validLockFile), 0o600))
	provider := lockfile.Provider{Source: "registry.terraform.io/hashicorp/null", Version: "3.2.1"}
	native := providerPackageDirs(data, provider)[0]
	arch := "amd64"
	if runtime.GOARCH == arch {
		arch = "arm64"
	}
	alternate := filepath.Join(filepath.Dir(native), runtime.GOOS+"_"+arch)
	require.NoError(t, os.MkdirAll(alternate, 0o755))
	executable := filepath.Join(alternate, "terraform-provider-null_v3.2.1_x5")
	require.NoError(t, os.WriteFile(executable, []byte("alternate package"), 0o755))
	in := &Inputs{ComponentPath: dir, DataDir: data, Binary: testBinary()}
	require.NoError(t, Record(in, []string{"init"}, "test"))
	assert.False(t, Decide(&Request{Inputs: in}).RunInit, "an emulated engine's valid package must not cause repeated init")
	// A native engine still diagnoses its missing package, and gets one init/retry.
	require.NoError(t, os.MkdirAll(native, 0o755))
	assert.False(t, Decide(&Request{Inputs: in}).RunInit)
	initCalls, retries := 0, 0
	failure := errors.New("provider installation failed")
	err := Recover(RecoverParams{Err: failure, Output: "Required plugins are not installed", RunInit: func(Recovery) error {
		initCalls++
		_, err := PrepareProviderInstallation(in)
		return err
	}, Retry: func() error { retries++; return failure }})
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, initCalls)
	assert.Equal(t, 1, retries)
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	assert.Equal(t, "alternate package", string(content))
	_, err = os.Stat(native)
	assert.True(t, os.IsNotExist(err), "incomplete native package is preserved aside before retry")
}

func TestProviderPreparationLeavesIrrelevantAlternatePackages(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, ".terraform")
	require.NoError(t, os.WriteFile(filepath.Join(dir, lockfile.Name), []byte(validLockFile), 0o600))
	provider := lockfile.Provider{Source: "registry.terraform.io/hashicorp/null", Version: "3.2.1"}
	native := providerPackageDirs(data, provider)[0]
	require.NoError(t, os.MkdirAll(native, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(native, "terraform-provider-null"), []byte("valid native package"), 0o755))
	alternate := filepath.Join(filepath.Dir(native), runtime.GOOS+"_other")
	require.NoError(t, os.MkdirAll(alternate, 0o755))
	sentinel := filepath.Join(alternate, "LICENSE")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0o600))
	cache, err := PrepareProviderInstallation(&Inputs{ComponentPath: dir, DataDir: data})
	require.NoError(t, err)
	assert.Empty(t, cache)
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "preserve", string(content))
}
