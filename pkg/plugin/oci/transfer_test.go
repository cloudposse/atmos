package oci

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPayloadTransferFailsClosedAtEveryReadBoundary(t *testing.T) {
	for _, operation := range []string{"extract", "verify", "inventory"} {
		for _, phase := range []string{"image", "manifest", "layer", "stream", "corrupt bytes"} {
			t.Run(operation+"/"+phase, func(t *testing.T) {
				controller := gomock.NewController(t)
				index := NewMockImageIndex(controller)
				image := NewMockImage(controller)
				layer := NewMockLayer(controller)
				hash := v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("0", 64)}
				manifest := v1.Manifest{Config: v1.Descriptor{MediaType: ConfigMediaType, Digest: hash}, Layers: []v1.Descriptor{{MediaType: BundleMediaType, Size: 8, Digest: hash}}}
				raw, err := json.Marshal(manifest)
				require.NoError(t, err)
				digest, size, err := v1.SHA256(bytes.NewReader(raw))
				require.NoError(t, err)
				descriptor := v1.Descriptor{Digest: digest, Size: size}
				release := &Release{Index: index, Platforms: map[string]v1.Descriptor{"linux/amd64": descriptor}}
				configureTransferFailure(index, image, layer, raw, phase)
				destination := t.TempDir()
				switch operation {
				case "extract":
					err = release.Extract("linux/amd64", destination)
				case "verify":
					err = release.VerifyInstalled("linux/amd64", destination)
				case "inventory":
					var files []BundleFile
					files, err = release.Files()
					require.Nil(t, files, "corrupt payloads must not produce a partial inventory")
				}
				if phase == "corrupt bytes" {
					require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
				} else {
					require.ErrorIs(t, err, os.ErrPermission)
				}
				files, err := os.ReadDir(destination)
				require.NoError(t, err)
				require.Empty(t, files, "failed verification must not materialize payload files")
			})
		}
	}
}

func configureTransferFailure(index *MockImageIndex, image *MockImage, layer *MockLayer, raw []byte, phase string) {
	if phase == "image" {
		index.EXPECT().Image(gomock.Any()).Return(nil, os.ErrPermission)
		return
	}
	index.EXPECT().Image(gomock.Any()).Return(image, nil)
	if phase == "manifest" {
		image.EXPECT().RawManifest().Return(nil, os.ErrPermission)
		return
	}
	image.EXPECT().RawManifest().Return(raw, nil)
	if phase == "layer" {
		image.EXPECT().LayerByDigest(gomock.Any()).Return(nil, os.ErrPermission)
		return
	}
	image.EXPECT().LayerByDigest(gomock.Any()).Return(layer, nil)
	if phase == "stream" {
		layer.EXPECT().Compressed().Return(nil, os.ErrPermission)
		return
	}
	layer.EXPECT().Compressed().Return(io.NopCloser(bytes.NewReader([]byte("modified"))), nil)
}
