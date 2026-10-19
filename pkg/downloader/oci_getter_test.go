package downloader

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestOCIGetter_GetExtractsImage(t *testing.T) {
	ref := ocitest.NewRegistry(t, "getter:v1", map[string]string{
		"vars.yaml":      "region: us-east-1\n",
		"nested/two.txt": "two",
	})
	u, err := url.Parse("oci://" + ref)
	require.NoError(t, err)

	g := &OCIGetter{AtmosConfig: &schema.AtmosConfiguration{}}
	g.SetClient(&getter.Client{Ctx: context.Background()})

	dst := filepath.Join(t.TempDir(), "out")
	require.NoError(t, g.Get(dst, u))

	data, err := os.ReadFile(filepath.Join(dst, "vars.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "region: us-east-1\n", string(data))
	data, err = os.ReadFile(filepath.Join(dst, "nested", "two.txt"))
	require.NoError(t, err)
	assert.Equal(t, "two", string(data))
}

func TestOCIGetter_GetWithoutClientUsesBackgroundContext(t *testing.T) {
	ref := ocitest.NewRegistry(t, "noclient:v1", map[string]string{"a.txt": "a"})
	u, err := url.Parse("oci://" + ref)
	require.NoError(t, err)

	dst := t.TempDir()
	require.NoError(t, (&OCIGetter{AtmosConfig: &schema.AtmosConfiguration{}}).Get(dst, u))
	assert.FileExists(t, filepath.Join(dst, "a.txt"))
}

func TestOCIGetter_GetHonorsCancelledContext(t *testing.T) {
	ref := ocitest.NewRegistry(t, "cancel:v1", map[string]string{"a.txt": "a"})
	u, err := url.Parse("oci://" + ref)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := &OCIGetter{AtmosConfig: &schema.AtmosConfiguration{}}
	g.SetClient(&getter.Client{Ctx: ctx})

	assert.Error(t, g.Get(t.TempDir(), u))
}

func TestOCIGetter_GetFileUnsupported(t *testing.T) {
	u, err := url.Parse("oci://ghcr.io/org/image:v1")
	require.NoError(t, err)

	err = (&OCIGetter{}).GetFile(filepath.Join(t.TempDir(), "f"), u)
	require.ErrorIs(t, err, errUtils.ErrOCIGetFileUnsupported)
}

func TestOCIGetter_ClientModeIsDir(t *testing.T) {
	mode, err := (&OCIGetter{}).ClientMode(&url.URL{})
	require.NoError(t, err)
	assert.Equal(t, getter.ClientModeDir, mode)
}

func TestFetchOCIContext_ThroughSharedDownloader(t *testing.T) {
	ref := ocitest.NewRegistry(t, "shared:v1", map[string]string{
		"vars.yaml":     "a: 1\n",
		"sub/inner.txt": "inner",
	})

	dst := t.TempDir()
	require.NoError(t, FetchOCIContext(context.Background(), &schema.AtmosConfiguration{}, ref, dst, WithOCITimeout(time.Minute)))
	assert.FileExists(t, filepath.Join(dst, "vars.yaml"))
	assert.FileExists(t, filepath.Join(dst, "sub", "inner.txt"))
}

func TestFetchOCIContext_InvalidReference(t *testing.T) {
	err := FetchOCIContext(context.Background(), &schema.AtmosConfiguration{}, "not a ref!!", t.TempDir(), WithOCITimeout(time.Minute))
	require.ErrorIs(t, err, errUtils.ErrInvalidImageReference)
}

func TestFetchOCIContext_PullFailureKeepsSentinel(t *testing.T) {
	ref := ocitest.NewRegistry(t, "exists:v1", map[string]string{"a.txt": "a"})
	missing := ref[:len(ref)-len("exists:v1")] + "missing:v1"

	err := FetchOCIContext(context.Background(), &schema.AtmosConfiguration{}, missing, t.TempDir(), WithOCITimeout(time.Minute))
	require.ErrorIs(t, err, errUtils.ErrPullImage)
}

func TestIsRemoteSource_OCIDoesNotTriggerBrokers(t *testing.T) {
	assert.False(t, isRemoteSource("oci://ghcr.io/org/image:v1"))
	assert.False(t, isRemoteSource("git::oci://ghcr.io/org/image:v1"))
	assert.True(t, isRemoteSource("https://github.com/org/repo"))
}
