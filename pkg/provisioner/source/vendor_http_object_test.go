package source

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// S3 object URLs remain single files even when IsS3URI recognizes their host.
// The loopback path includes an S3 host to exercise the same URI classification
// without credentials, DNS overrides, or external network access.
func TestVendorSourceHTTPObjectRecognizedAsS3(t *testing.T) {
	const content = "Resources: {}\n"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, content)
	}))
	defer server.Close()
	uri := server.URL + "/bucket.s3.amazonaws.com/template.yaml"
	require.True(t, vendor.IsS3URI(uri))
	target := filepath.Join(t.TempDir(), "template.yaml")
	require.NoError(t, VendorSource(context.Background(), &schema.AtmosConfiguration{}, &schema.VendorComponentSource{Uri: uri}, target))
	actual, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, content, string(actual))
	assert.Positive(t, requests.Load())
}

func TestVendorSourceUppercaseArchiveExtensionRemainsFile(t *testing.T) {
	const content = "raw bytes from a case-sensitive source"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, content)
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, VendorSource(context.Background(), &schema.AtmosConfiguration{}, &schema.VendorComponentSource{Uri: server.URL + "/module.ZIP"}, target))
	actual, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, content, string(actual))
}
