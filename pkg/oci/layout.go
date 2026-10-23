package oci

import (
	"context"
	"errors"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SaveIndex writes a complete index to a standard OCI layout. The outer layout
// index contains exactly one descriptor for the unchanged release index.
func SaveIndex(directory string, index v1.ImageIndex) error {
	defer perf.Track(nil, "oci.SaveIndex")()

	path, err := layout.Write(directory, empty.Index)
	if err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	if err := path.AppendIndex(index); err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	return nil
}

// LoadIndex opens the single root index in an OCI layout, optionally enforcing
// its expected digest. Referenced blobs remain lazy so callers can select one
// platform; they must validate that platform's bytes before materializing it.
func LoadIndex(directory, expected string) (v1.ImageIndex, error) {
	defer perf.Track(nil, "oci.LoadIndex")()

	root, err := layout.ImageIndexFromPath(directory)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	manifest, err := root.IndexManifest()
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	if len(manifest.Manifests) != 1 || !manifest.Manifests[0].MediaType.IsIndex() {
		return nil, errUtils.ErrOCILayout
	}
	selected := manifest.Manifests[0]
	if expected != "" && selected.Digest.String() != expected {
		return nil, errUtils.ErrOCILayout
	}
	index, err := root.ImageIndex(selected.Digest)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	digest, err := index.Digest()
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	if digest != selected.Digest {
		return nil, errUtils.ErrOCILayout
	}
	return index, nil
}

// ExportIndex downloads all platforms into an offline OCI layout.
func ExportIndex(ctx context.Context, source Registry, ref name.Reference, directory string) error {
	defer perf.Track(nil, "oci.ExportIndex")()

	descriptor, err := source.Get(ctx, ref)
	if err != nil {
		return err
	}
	index, err := descriptor.ImageIndex()
	if err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	return SaveIndex(directory, index)
}

// ImportIndex uploads a layout with the destination's credentials, independent
// of any source registry credentials used to create it.
func ImportIndex(ctx context.Context, destination Registry, directory string, ref name.Reference) error {
	defer perf.Track(nil, "oci.ImportIndex")()

	index, err := LoadIndex(directory, "")
	if err != nil {
		return err
	}
	return destination.WriteIndex(ctx, ref, index)
}
