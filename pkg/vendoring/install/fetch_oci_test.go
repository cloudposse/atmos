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

func TestFetchOCI_PullsThroughSharedDownloader(t *testing.T) {
	ref := ocitest.NewRegistry(t, "vendor:v1", map[string]string{"main.tf": "# tf\n"})

	dir, err := fetchOCI(context.Background(), &schema.AtmosConfiguration{}, ref, t.TempDir())
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "main.tf"))
}

func TestFetchOCI_FailureWrapsSentinels(t *testing.T) {
	ref := ocitest.NewRegistry(t, "vendor-exists:v1", map[string]string{"main.tf": "# tf\n"})
	missing := ref[:len(ref)-len("vendor-exists:v1")] + "vendor-missing:v1"

	_, err := fetchOCI(context.Background(), &schema.AtmosConfiguration{}, missing, t.TempDir())
	require.ErrorIs(t, err, ErrProcessOCIImage)
	require.ErrorIs(t, err, errUtils.ErrPullImage)
}
