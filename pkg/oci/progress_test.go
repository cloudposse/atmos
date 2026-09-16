package oci

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObservedRegistryRetryPreservesResult(t *testing.T) {
	descriptor := &remote.Descriptor{}
	shim := &pullImageShim{results: []pullImageResult{{err: &net.OpError{Op: "read", Err: errors.New("connection reset")}}, {descriptor: descriptor}}}
	installPullImageShim(t, shim)
	var attempts []int
	ctx := WithRetryObserver(context.Background(), func(n int) { attempts = append(attempts, n) })
	ref, err := name.ParseReference("registry.example.com/component:v1")
	require.NoError(t, err)
	got, err := remoteGetWithRetry(ctx, ref, authn.Anonymous, testOCIManifestRetryConfig())
	require.NoError(t, err)
	assert.Same(t, descriptor, got)
	assert.Equal(t, []int{2}, attempts)
	assert.False(t, observed(context.Background()))
	assert.False(t, reportRetry(context.Background(), 2))
	assert.Equal(t, context.Background(), WithRetryObserver(context.Background(), nil))
}

func TestObservedLayerRetryPreservesExtraction(t *testing.T) {
	payload := writeTestTar(t, map[string]string{"main.tf": "# recovered\n"})
	layer := &MockLayer{digestVal: v1.Hash{Algorithm: "sha256", Hex: "123"}, uncompressedErrs: []error{&net.OpError{Op: "read", Err: errors.New("connection reset")}}, uncompressedData: payload.Bytes()}
	var attempts []int
	ctx := WithRetryObserver(context.Background(), func(n int) { attempts = append(attempts, n) })
	require.NoError(t, processLayerWithRetry(ctx, layer, 0, t.TempDir(), testOCILayerRetryConfig()))
	assert.Equal(t, []int{2}, attempts)
	assert.Equal(t, 2, layer.uncompressedCalls)
}
