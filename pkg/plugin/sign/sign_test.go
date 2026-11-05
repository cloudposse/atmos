package sign

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSignPublishesVerifiableEvidenceForExactIndex(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin"), []byte("prebuilt"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(`apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/fixture
  version: 1.0.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: plugin
  compatibility:
    protocol: ">=1 <2"
  platforms:
    linux/amd64:
      executable: plugin
  capabilities:
    fixtures:
      acme/fixture: {}
`), 0o600))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	server := httptest.NewServer(registry.New())
	defer server.Close()
	client := oci.NewClient(nil)
	repo, err := name.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/test/plugin")
	require.NoError(t, err)
	require.NoError(t, client.WriteIndex(t.Context(), repo.Tag("1.0.0"), index))
	digest, err := index.Digest()
	require.NoError(t, err)
	raw, err := index.RawManifest()
	require.NoError(t, err)
	authority := signaturetest.New(t)
	signed := authority.Sign(t, "builder@example.com", "https://issuer.example.com", raw)
	signer := NewMockBlobSigner(gomock.NewController(t))
	signer.EXPECT().Sign(gomock.Any(), raw).Return(signed, nil)
	service := Service{Registry: client, Signer: signer}
	reference := repo.Digest(digest.String())
	release, err := service.Release(t.Context(), reference.Name())
	require.NoError(t, err)
	require.Equal(t, digest.String(), release.Digest)
	evidence, err := signature.Fetch(t.Context(), client, reference)
	require.NoError(t, err)
	require.Equal(t, [][]byte{signed}, evidence)
	verifier := signature.Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }}
	result, err := verifier.Verify(t.Context(), evidence[0], release.Digest, []signature.Identity{{Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}, false)
	require.NoError(t, err)
	require.Equal(t, "builder@example.com", result.Identity)
	_, err = service.Release(t.Context(), repo.Tag("1.0.0").Name())
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	// Failed signing must never replace the previous valid attachment.
	signer.EXPECT().Sign(gomock.Any(), raw).Return(nil, context.Canceled)
	_, err = service.Release(t.Context(), reference.Name())
	require.ErrorIs(t, err, context.Canceled)
	signer.EXPECT().Sign(gomock.Any(), raw).Return(authority.Sign(t, "builder@example.com", "https://issuer.example.com", []byte("different release")), nil)
	_, err = service.Release(t.Context(), reference.Name())
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	after, err := signature.Fetch(t.Context(), client, reference)
	require.NoError(t, err)
	require.Equal(t, evidence, after)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = service.Release(canceled, reference.Name())
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.Release(t.Context(), repo.Digest("sha256:"+strings.Repeat("0", 64)).Name())
	require.Error(t, err)
	require.NoError(t, client.WriteIndex(t.Context(), repo.Tag("invalid"), empty.Index))
	invalid, err := empty.Index.Digest()
	require.NoError(t, err)
	_, err = service.Release(t.Context(), repo.Digest(invalid.String()).Name())
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NoError(t, client.WriteImage(t.Context(), repo.Tag("image"), empty.Image))
	invalid, err = empty.Image.Digest()
	require.NoError(t, err)
	_, err = service.Release(t.Context(), repo.Digest(invalid.String()).Name())
	require.Error(t, err)
}

func TestCosignManagedInvocation(t *testing.T) {
	for _, yes := range []bool{false, true} {
		t.Run(map[bool]string{false: "interactive", true: "confirmed"}[yes], func(t *testing.T) {
			payload := []byte("index bytes")
			var temporary string
			signer := Cosign{Yes: yes, Resolve: func(_ context.Context, _ *schema.AtmosConfiguration, version string) (string, error) {
				require.Equal(t, DefaultVersion, version)
				return "managed-cosign", nil
			}, Run: func(_ context.Context, binary string, args []string) error {
				require.Equal(t, "managed-cosign", binary)
				require.Equal(t, []string{"sign-blob", "--new-bundle-format=true", "--bundle"}, args[:3])
				require.Equal(t, yes, len(args) == 6)
				if yes {
					require.Equal(t, "--yes", args[4])
				}
				raw, err := os.ReadFile(args[len(args)-1])
				require.NoError(t, err)
				require.Equal(t, payload, raw)
				temporary = filepath.Dir(args[3])
				return os.WriteFile(args[3], []byte("bundle"), 0o600)
			}}
			result, err := signer.Sign(t.Context(), payload)
			require.NoError(t, err)
			require.Equal(t, []byte("bundle"), result)
			_, err = os.Stat(temporary)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestCosignFailurePaths(t *testing.T) {
	for _, version := range []string{"2.6.0", "latest", "^3.0", "4.0.0"} {
		_, err := (Cosign{Version: version}).Sign(t.Context(), nil)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := (Cosign{}).Sign(canceled, nil)
	require.ErrorIs(t, err, context.Canceled)
	signer := Cosign{Resolve: func(context.Context, *schema.AtmosConfiguration, string) (string, error) {
		return "", context.DeadlineExceeded
	}}
	_, err = signer.Sign(t.Context(), nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	signer.Resolve = func(context.Context, *schema.AtmosConfiguration, string) (string, error) {
		return "managed-cosign", nil
	}
	signer.Run = func(context.Context, string, []string) error { return context.Canceled }
	_, err = signer.Sign(t.Context(), nil)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	signer.Run = func(context.Context, string, []string) error { return nil }
	_, err = signer.Sign(t.Context(), nil)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorIs(t, validateBundle([]byte("malformed"), nil), errUtils.ErrPluginSignatureInvalid)
	require.ErrorIs(t, validateBundle(nil, nil), errUtils.ErrPluginSignatureInvalid)
	require.ErrorIs(t, validateBundle(make([]byte, signature.MaxBundleSize+1), nil), errUtils.ErrPluginSignatureInvalid)
}
