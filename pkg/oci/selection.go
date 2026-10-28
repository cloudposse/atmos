package oci

import (
	"bytes"
	"errors"
	"io"
	"slices"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SaveSelection retains an unchanged index and all child manifests/configs,
// downloading layers only for selected image digests. The resulting partial
// layout supports offline inspection and restoration of those platforms.
// Nested indexes are rejected explicitly; they must use SaveIndex instead.
func SaveSelection(directory string, index v1.ImageIndex, selected []v1.Hash) error {
	defer perf.Track(nil, "oci.SaveSelection")()

	root, err := index.IndexManifest()
	if err != nil {
		return err
	}
	path, err := layout.Write(directory, empty.Index)
	if err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	if err := saveSelectedImages(path, index, root, selected); err != nil {
		return err
	}
	raw, err := index.RawManifest()
	if err != nil {
		return err
	}
	digest, size, err := v1.SHA256(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := path.WriteBlob(digest, io.NopCloser(bytes.NewReader(raw))); err != nil {
		return err
	}
	return path.AppendDescriptor(v1.Descriptor{Digest: digest, Size: size, MediaType: root.MediaType})
}

func saveImageMetadata(path layout.Path, img v1.Image, descriptor *v1.Descriptor) error {
	raw, err := img.RawManifest()
	if err != nil {
		return err
	}
	if _, err := ReadVerifiedBlob(bytes.NewReader(raw), descriptor, int64(len(raw))); err != nil {
		return err
	}
	if err := path.WriteBlob(descriptor.Digest, io.NopCloser(bytes.NewReader(raw))); err != nil {
		return err
	}
	manifest, err := img.Manifest()
	if err != nil {
		return err
	}
	config, err := img.RawConfigFile()
	if err != nil {
		return err
	}
	if _, err := ReadVerifiedBlob(bytes.NewReader(config), &manifest.Config, int64(len(config))); err != nil {
		return err
	}
	return path.WriteBlob(manifest.Config.Digest, io.NopCloser(bytes.NewReader(config)))
}

func saveSelectedImages(path layout.Path, index v1.ImageIndex, root *v1.IndexManifest, selected []v1.Hash) error {
	for itemIndex := range root.Manifests {
		descriptor := root.Manifests[itemIndex]
		if !descriptor.MediaType.IsImage() {
			return errUtils.ErrOCILayout
		}
		img, err := index.Image(descriptor.Digest)
		if err != nil {
			return err
		}
		if err := saveImageMetadata(path, img, &descriptor); err != nil {
			return err
		}
		if slices.Contains(selected, descriptor.Digest) {
			if err := path.WriteImage(img); err != nil {
				return errors.Join(errUtils.ErrOCILayout, err)
			}
		}
	}
	return nil
}
