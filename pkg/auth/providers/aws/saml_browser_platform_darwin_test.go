//go:build darwin

package aws

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Regression: a failed CPU-brand lookup used to be swallowed and the host treated as Intel, so an
// Apple Silicon host with only the Intel browser directory cached skipped the browser install.
func TestDarwinKernelInfo_CPUBrandFailureIsReported(t *testing.T) {
	errBrand := errors.New("sysctl failed")
	original := darwinSysctl
	t.Cleanup(func() { darwinSysctl = original })
	darwinSysctl = func(name string) (string, error) {
		if name == "machdep.cpu.brand_string" {
			return "", errBrand
		}
		return "23.4.0", nil
	}

	_, _, err := darwinKernelInfo()
	require.ErrorIs(t, err, errBrand)
}

// Negative path: a readable CPU brand is returned unchanged.
func TestDarwinKernelInfo_CPUBrandSuccess(t *testing.T) {
	original := darwinSysctl
	t.Cleanup(func() { darwinSysctl = original })
	darwinSysctl = func(name string) (string, error) {
		if name == "machdep.cpu.brand_string" {
			return "Apple M2", nil
		}
		return "23.4.0", nil
	}

	release, brand, err := darwinKernelInfo()
	require.NoError(t, err)
	assert.Equal(t, "23.4.0", release)
	assert.Equal(t, "Apple M2", brand)
}

// An undeterminable host platform makes the required browser undeterminable, which enables the
// install (and so the exact-revision check) instead of assuming an Intel host.
func TestRequiredBrowserDir_HostPlatformFailureIsReported(t *testing.T) {
	t.Setenv(playwrightHostPlatformOverrideEnv, "")
	original := darwinHostInfo
	t.Cleanup(func() { darwinHostInfo = original })
	darwinHostInfo = func() (string, string, error) { return "", "", assert.AnError }

	_, err := requiredBrowserDir(chromiumBrowserName)
	require.ErrorIs(t, err, errUtils.ErrPlaywrightBrowserManifest)
	require.ErrorIs(t, err, assert.AnError)
}
