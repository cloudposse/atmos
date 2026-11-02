package plugin

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/oci/ocitest"
)

// publisherTransport distinguishes credentialed writes/reads from the anonymous verifier.
type publisherTransport struct{}

func (publisherTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("X-Test-Publisher", "true")
	return http.DefaultTransport.RoundTrip(request)
}

func TestPublishPublicAccessAfterUpload(t *testing.T) {
	backend := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	var public atomic.Bool
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !public.Load() && r.Header.Get("X-Test-Publisher") == "" {
			http.Error(w, "anonymous access denied", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/") {
			writes.Add(1)
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client := atmosoci.NewClient(nil, atmosoci.WithTransport(publisherTransport{}))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	manager := &manage.Manager{Installer: &install.Service{Registry: client, Descriptors: descriptors}}
	layout := publicReleaseLayout(t, descriptors)
	repository := server.Listener.Addr().String() + "/example/plugin"
	values := viper.New()
	values.Set(publishDestinationFlag, repository)
	values.Set(verifyPublicFlag, true)

	_, err := publishLayout(t.Context(), manager, layout, values)
	require.ErrorContains(t, err, "was published but public access verification failed")
	require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "rerun the same")
	ref, err := name.NewTag(repository + ":1.0.0")
	require.NoError(t, err)
	uploaded, err := client.Head(t.Context(), ref)
	require.NoError(t, err, "the first push must create the package despite private visibility")
	initialWrites := writes.Load()
	require.Positive(t, initialWrites)

	public.Store(true)
	release, err := publishLayout(t.Context(), manager, layout, values)
	require.NoError(t, err)
	require.Equal(t, uploaded.Digest.String(), release.Digest)
	require.Equal(t, initialWrites, writes.Load(), "retry must reuse the immutable release")

	values.Set(publishDestinationFlag, server.Listener.Addr().String()+"/example/private")
	values.Set(verifyPublicFlag, false)
	public.Store(false)
	_, err = publishLayout(t.Context(), manager, layout, values)
	require.NoError(t, err, "ordinary publication must support private registries")
}

func publicReleaseLayout(t *testing.T, descriptors *extension.Registry) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), ocitest.Binary("linux", "amd64"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(`apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: example/fixture
  version: 1.0.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/plugin
  compatibility:
    protocol: ">=1 <2"
  platforms:
    linux/amd64:
      executable: bin/plugin
  capabilities:
    fixtures:
      example/fixture: {}
`), 0o644))
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, descriptors))
	return layout
}
