package install

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFetchToTempDir_OCI_PullsThroughSharedDownloader proves the PkgTypeOci path fetches an image straight into the temp directory.
func TestFetchToTempDir_OCI_PullsThroughSharedDownloader(t *testing.T) {
	ref := ocitest.NewRegistry(t, "vendor:v1", map[string]string{"main.tf": "# tf\n"})

	dir, _, err := fetchToTempDir(context.Background(), &schema.AtmosConfiguration{}, ref, PkgTypeOci, t.TempDir(), fetchOptions{})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "main.tf"))
}

// TestFetchToTempDir_OCI_FailureWrapsSentinels proves a failed OCI pull wraps both ErrProcessOCIImage and ErrPullImage.
func TestFetchToTempDir_OCI_FailureWrapsSentinels(t *testing.T) {
	ref := ocitest.NewRegistry(t, "vendor-exists:v1", map[string]string{"main.tf": "# tf\n"})
	missing := ref[:len(ref)-len("vendor-exists:v1")] + "vendor-missing:v1"

	_, _, err := fetchToTempDir(context.Background(), &schema.AtmosConfiguration{}, missing, PkgTypeOci, t.TempDir(), fetchOptions{})
	require.ErrorIs(t, err, ErrProcessOCIImage)
	require.ErrorIs(t, err, errUtils.ErrPullImage)
}
