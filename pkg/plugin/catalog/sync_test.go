package catalog

import (
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

const syncManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: example/fixture
  version: 1.2.3
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
`

func TestSyncPreservesHistoryAndPublishesOnce(t *testing.T) {
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer server.Close()
	client := atmosoci.NewClient(nil)
	source := server.Listener.Addr().String() + "/example/plugin"
	target := server.Listener.Addr().String() + "/example/catalog:stable"
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(syncManifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("native"), 0o755))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	ref, err := name.NewTag(source + ":1.2.3")
	require.NoError(t, err)
	require.NoError(t, client.WriteIndex(t.Context(), ref, index))
	hash, err := index.Digest()
	require.NoError(t, err)
	plan := []ReleaseSelection{{Name: "fixture", Repository: source, Version: "1.2.3"}}
	public := NewMockPublicVerifier(gomock.NewController(t))
	public.EXPECT().RequirePublic(gomock.Any(), server.Listener.Addr().String()+"/example/catalog").Return(nil)
	public.EXPECT().RequirePublic(gomock.Any(), source).Return(nil)
	public.EXPECT().Verify(gomock.Any(), source, "1.2.3", hash.String()).Return(hash.String(), nil)
	public.EXPECT().Verify(gomock.Any(), server.Listener.Addr().String()+"/example/catalog", "stable", gomock.Any()).Return("", nil)
	require.NoError(t, NewSyncer(client, descriptors).Sync(t.Context(), target, plan, WithPublicVerification(public)))
	file, digest, err := Read(t.Context(), client, target)
	require.NoError(t, err)
	require.Equal(t, 1, file.Metadata.Revision)
	file.Spec.Entries[0].Versions[0].Yanked = &Yanked{Reason: "regression"}
	require.NoError(t, Publish(t.Context(), client, target, file, digest))
	require.NoError(t, NewSyncer(client, descriptors).Sync(t.Context(), target, append(plan, ReleaseSelection{Repository: source, Version: "2.0.0"}), WithSkipMissing()))
	file, _, err = Read(t.Context(), client, target)
	require.NoError(t, err)
	require.Equal(t, 3, file.Metadata.Revision)
	require.Equal(t, "regression", file.Spec.Entries[0].Versions[0].Yanked.Reason)
	require.Equal(t, hash.String(), file.Spec.Entries[0].Versions[0].Digest)
	missingErr := NewSyncer(client, descriptors).Sync(t.Context(), target, []ReleaseSelection{{Repository: source, Version: "missing"}})
	require.Error(t, missingErr)
	require.Contains(t, missingErr.Error(), "MANIFEST_UNKNOWN")
	require.True(t, atmosoci.IsNotFound(missingErr))
	require.ErrorIs(t, NewSyncer(client, descriptors).Sync(t.Context(), target, []ReleaseSelection{{Repository: source, Version: "missing"}}, WithSkipMissing()), errUtils.ErrPluginVersionNotFound)
	for _, release := range []ReleaseSelection{{Repository: "bad repository", Version: "1.0.0"}, {Repository: source}, {Repository: source, Version: "invalid tag"}} {
		require.Error(t, NewSyncer(client, descriptors).Sync(t.Context(), target, []ReleaseSelection{release}))
	}
	public = NewMockPublicVerifier(gomock.NewController(t))
	public.EXPECT().RequirePublic(gomock.Any(), gomock.Any()).Return(errUtils.ErrOCIRegistry)
	require.ErrorIs(t, NewSyncer(client, descriptors).Sync(t.Context(), target, plan, WithPublicVerification(public)), errUtils.ErrOCIRegistry)

	public = NewMockPublicVerifier(gomock.NewController(t))
	public.EXPECT().RequirePublic(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	missingBlob := &transport.Error{StatusCode: 404}
	public.EXPECT().Verify(gomock.Any(), source, "1.2.3", hash.String()).Return("", missingBlob)
	require.ErrorIs(t, NewSyncer(client, descriptors).Sync(t.Context(), target, plan, WithSkipMissing(), WithPublicVerification(public)), missingBlob, "a missing blob is corruption, not an unpublished release")
	require.ErrorIs(t, NewSyncer(client, descriptors).Sync(t.Context(), target, nil), errUtils.ErrPluginInvalidConfig)
	require.Error(t, NewSyncer(client, descriptors).Sync(t.Context(), "not a reference", plan))
	// A tag whose packaged version disagrees with the selection is never advertised.
	wrong, err := name.NewTag(source + ":9.9.9")
	require.NoError(t, err)
	require.NoError(t, client.WriteIndex(t.Context(), wrong, index))
	require.ErrorIs(t, NewSyncer(client, descriptors).Sync(t.Context(), target, []ReleaseSelection{{Repository: source, Version: "9.9.9"}}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, NewSyncer(client, descriptors).Sync(t.Context(), target, []ReleaseSelection{{Name: "wrong-package", Repository: source, Version: "1.2.3"}}), errUtils.ErrPluginInvalidConfig)
	file, _, err = Read(t.Context(), client, target)
	require.NoError(t, err)
	require.Equal(t, 3, file.Metadata.Revision, "all failed reconciliations must leave catalog untouched")
}

func TestSyncFailsClosedOnRegistryAndPublicErrors(t *testing.T) {
	descriptors := extension.New()
	selected := ReleaseSelection{Repository: "ghcr.io/example/plugin", Version: "1.0.0"}
	client := NewMockRegistry(gomock.NewController(t))
	client.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, errUtils.ErrOCIRegistry)
	require.ErrorIs(t, NewSyncer(client, descriptors).Sync(t.Context(), "ghcr.io/example/catalog:stable", []ReleaseSelection{selected}, WithSkipMissing()), errUtils.ErrOCIRegistry)
	public := NewMockPublicVerifier(gomock.NewController(t))
	public.EXPECT().RequirePublic(gomock.Any(), selected.Repository).Return(errUtils.ErrOCIRegistry)
	require.ErrorIs(t, NewSyncer(client, descriptors).syncRelease(t.Context(), New("test"), selected, public), errUtils.ErrOCIRegistry)
	public = NewMockPublicVerifier(gomock.NewController(t))
	public.EXPECT().RequirePublic(gomock.Any(), selected.Repository).Return(nil)
	public.EXPECT().Verify(gomock.Any(), selected.Repository, selected.Version, gomock.Any()).Return("", errUtils.ErrOCIIntegrity)
	client.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&remote.Descriptor{}, nil)
	require.ErrorIs(t, NewSyncer(client, descriptors).syncRelease(t.Context(), New("test"), selected, public), errUtils.ErrOCIIntegrity)
}
