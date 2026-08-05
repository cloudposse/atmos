package ocitest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewRegistry_RoundTrip(t *testing.T) {
	imageRef := NewRegistry(t, "test/roundtrip:v1", map[string]string{
		"marker.txt": "hello-from-ocitest\n",
	})

	dest := t.TempDir()
	err := oci.ProcessImage(context.Background(), &schema.AtmosConfiguration{}, imageRef, dest)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(dest, "marker.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello-from-ocitest\n", string(got))
}
