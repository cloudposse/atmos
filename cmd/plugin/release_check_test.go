package plugin

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestReleaseCheckPublishedVersion(t *testing.T) {
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	parentLayout := publicReleaseLayout(t, descriptors)
	index, err := atmosoci.LoadIndex(parentLayout, "")
	require.NoError(t, err)
	parent, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	reference, err := name.NewTag("registry.example.com/example/fixture:1.0.0")
	require.NoError(t, err)
	missing := &transport.Error{StatusCode: http.StatusNotFound}
	for _, test := range []struct {
		name        string
		changed     bool
		check       bool
		destination string
		registryErr error
		wantErr     error
	}{
		{name: "unpublished version", changed: true, check: true, destination: "oci://registry.example.com/example/fixture", registryErr: missing},
		{name: "identical published parent", check: true, destination: "oci://registry.example.com/example/fixture"},
		{name: "child reuses published parent version", changed: true, check: true, destination: "oci://registry.example.com/example/fixture", wantErr: errUtils.ErrPluginImmutableRelease},
		{name: "registry access denied", check: true, destination: "oci://registry.example.com/example/fixture", registryErr: os.ErrPermission, wantErr: os.ErrPermission},
		{name: "registry timeout", check: true, destination: "oci://registry.example.com/example/fixture", registryErr: os.ErrDeadlineExceeded, wantErr: os.ErrDeadlineExceeded},
		{name: "offline directory", check: true},
		{name: "publication checks later", destination: "oci://registry.example.com/example/fixture"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			require.NoError(t, parent.Extract("linux/amd64", directory))
			if test.changed {
				binary := filepath.Join(directory, "bin", "plugin")
				raw, err := os.ReadFile(binary)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(binary, append(raw, []byte("child change")...), 0o755))
			}
			client := NewMockRegistry(gomock.NewController(t))
			if test.check && test.destination != "" {
				client.EXPECT().Head(gomock.Any(), reference).Return(&v1.Descriptor{Digest: hashOf(t, parent.Digest)}, test.registryErr)
			}
			values := viper.New()
			values.Set(releaseCheckFlag, test.check)
			values.Set(publishDestinationFlag, test.destination)
			backend := &releaseBackend{values: values, manager: &manage.Manager{Installer: &install.Service{Registry: client, Descriptors: descriptors}}}
			err := backend.Pack(t.Context(), directory, filepath.Join(t.TempDir(), "layout"))
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
