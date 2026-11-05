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

// TestOCIGetter_GetExtractsImage proves Get pulls an image from an in-process registry and extracts every layer into the destination.
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

// TestOCIGetter_GetWithoutClientUsesBackgroundContext proves Get still works when no go-getter client was attached via SetClient.
func TestOCIGetter_GetWithoutClientUsesBackgroundContext(t *testing.T) {
	ref := ocitest.NewRegistry(t, "noclient:v1", map[string]string{"a.txt": "a"})
	u, err := url.Parse("oci://" + ref)
	require.NoError(t, err)

	dst := t.TempDir()
	require.NoError(t, (&OCIGetter{AtmosConfig: &schema.AtmosConfiguration{}}).Get(dst, u))
	assert.FileExists(t, filepath.Join(dst, "a.txt"))
}

// TestOCIGetter_GetHonorsCancelledContext proves Get fails when the attached client context is already cancelled.
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

// TestOCIGetter_GetFailsWhenDestinationCannotBeCreated proves Get reports a directory-creation error when the destination sits under a regular file.
func TestOCIGetter_GetFailsWhenDestinationCannotBeCreated(t *testing.T) {
	ref := ocitest.NewRegistry(t, "baddst:v1", map[string]string{"a.txt": "a"})
	u, err := url.Parse("oci://" + ref)
	require.NoError(t, err)

	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	err = (&OCIGetter{AtmosConfig: &schema.AtmosConfiguration{}}).Get(filepath.Join(blocker, "out"), u)
	require.ErrorIs(t, err, errUtils.ErrCreateTempDirectory)
}

// TestOCIGetter_GetFileUnsupported proves single-file mode is rejected with ErrOCIGetFileUnsupported.
func TestOCIGetter_GetFileUnsupported(t *testing.T) {
	u, err := url.Parse("oci://ghcr.io/org/image:v1")
	require.NoError(t, err)

	err = (&OCIGetter{}).GetFile(filepath.Join(t.TempDir(), "f"), u)
	require.ErrorIs(t, err, errUtils.ErrOCIGetFileUnsupported)
}

// TestOCIGetter_ClientModeIsDir proves the getter always asks go-getter for directory mode.
func TestOCIGetter_ClientModeIsDir(t *testing.T) {
	mode, err := (&OCIGetter{}).ClientMode(&url.URL{})
	require.NoError(t, err)
	assert.Equal(t, getter.ClientModeDir, mode)
}

// TestFetch_OCISourceThroughSharedDownloader proves an oci:// source fetched with the ordinary Fetch method reaches the registered OCI getter.
func TestFetch_OCISourceThroughSharedDownloader(t *testing.T) {
	ref := ocitest.NewRegistry(t, "shared:v1", map[string]string{
		"vars.yaml":     "a: 1\n",
		"sub/inner.txt": "inner",
	})

	dst := t.TempDir()
	err := NewGoGetterDownloader(&schema.AtmosConfiguration{}).Fetch("oci://"+ref, dst, ClientModeDir, time.Minute)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dst, "vars.yaml"))
	assert.FileExists(t, filepath.Join(dst, "sub", "inner.txt"))
}

// TestFetch_OCISourceSubdirSelectsDirectory proves go-getter's //subdir handling selects a directory out of a pulled image.
func TestFetch_OCISourceSubdirSelectsDirectory(t *testing.T) {
	ref := ocitest.NewRegistry(t, "subdir:v1", map[string]string{
		"vars.yaml":     "a: 1\n",
		"sub/inner.txt": "inner",
	})

	dst := t.TempDir()
	err := NewGoGetterDownloader(&schema.AtmosConfiguration{}).Fetch("oci://"+ref+"//sub", dst, ClientModeDir, time.Minute)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dst, "inner.txt"))
	assert.NoFileExists(t, filepath.Join(dst, "vars.yaml"))
}

// TestFetch_OCISourceInvalidReference proves a malformed oci:// reference fails with ErrInvalidImageReference.
func TestFetch_OCISourceInvalidReference(t *testing.T) {
	err := NewGoGetterDownloader(&schema.AtmosConfiguration{}).Fetch("oci://not a ref!!", t.TempDir(), ClientModeDir, time.Minute)
	require.ErrorIs(t, err, errUtils.ErrInvalidImageReference)
}

// TestFetch_OCISourcePullFailureKeepsSentinel proves a failed pull keeps ErrPullImage in the error chain.
func TestFetch_OCISourcePullFailureKeepsSentinel(t *testing.T) {
	ref := ocitest.NewRegistry(t, "exists:v1", map[string]string{"a.txt": "a"})
	missing := ref[:len(ref)-len("exists:v1")] + "missing:v1"

	// FetchWithMetadataContext (what vendoring and scaffold use) preserves the error chain; plain
	// Fetch formats the cause with %v and drops it.
	dl := NewGoGetterDownloader(&schema.AtmosConfiguration{}).(ContextFileDownloader)
	_, err := dl.FetchWithMetadataContext(context.Background(), "oci://"+missing, t.TempDir(), ClientModeDir, time.Minute)
	require.ErrorIs(t, err, errUtils.ErrPullImage)
}

// TestPrepareOCISource covers normalization of oci:// sources before go-getter parses them.
func TestPrepareOCISource(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"non-oci source is untouched", "https://example.com/a.yaml", "https://example.com/a.yaml"},
		{"tag is kept and archive detection disabled", "oci://ghcr.io/org/image:v1", "oci://ghcr.io/org/image:v1?archive=false"},
		{"registry port is kept", "oci://127.0.0.1:5000/repo:v1", "oci://127.0.0.1:5000/repo:v1?archive=false"},
		{"host-less ref is made fully qualified", "oci://ubuntu:latest", "oci://index.docker.io/library/ubuntu:latest?archive=false"},
		{"subdir is preserved before the query", "oci://ghcr.io/org/image:v1//sub/dir", "oci://ghcr.io/org/image:v1//sub/dir?archive=false"},
		{"archive-like ref name is not decompressed", "oci://ghcr.io/org/pack.tgz:v1", "oci://ghcr.io/org/pack.tgz:v1?archive=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareOCISource(tt.src)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := prepareOCISource("oci://not a ref!!")
	require.ErrorIs(t, err, errUtils.ErrInvalidImageReference)
}

// TestIsRemoteSource_OCIDoesNotTriggerBrokers proves oci:// sources skip the git credential-broker gating.
func TestIsRemoteSource_OCIDoesNotTriggerBrokers(t *testing.T) {
	assert.False(t, isRemoteSource("oci://ghcr.io/org/image:v1"))
	assert.False(t, isRemoteSource("git::oci://ghcr.io/org/image:v1"))
	assert.True(t, isRemoteSource("https://github.com/org/repo"))
}
