package ocitest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewRegistry_RoundTrip(t *testing.T) {
	imageRef := NewRegistry(t, "test/roundtrip:v1", map[string]string{
		"marker.txt": "hello-from-ocitest\n",
	})

	dest := t.TempDir()
	err := oci.ProcessImage(&schema.AtmosConfiguration{}, imageRef, dest)
	require.NoError(t, err)
}
