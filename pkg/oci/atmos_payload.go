package oci

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// Atmos artifact packages (`atmos artifacts publish`, `atmos scaffold publish`) describe themselves
// with the shared Record config media type. Their layers are the archive payload followed by the
// README and CHANGELOG documents as separate Markdown blobs. Component and scaffold consumers that
// unpack an image into a directory must extract only the payload layer: the document layers are not
// archives, and the payload's generated `.atmos/artifact` distribution members are never part of
// what a consumer provisions or generates from.
//
// Legacy images (a single tar+gzip component layer, OpenTofu module zips, and generic images) do
// not carry that config media type, so they keep extracting every layer exactly as before.

// isAtmosPackage reports whether the image is an Atmos artifact package.
func isAtmosPackage(img v1.Image) (bool, error) {
	manifest, err := img.Manifest()
	if err != nil {
		return false, err
	}
	return string(manifest.Config.MediaType) == artifact.MediaTypeConfig, nil
}

// atmosPayloadExtractor returns the extractor for an Atmos payload layer media type. Every extractor
// withholds the distribution metadata of the package.
func atmosPayloadExtractor(mediaType string) (func(io.Reader, string) error, bool) {
	switch mediaType {
	case artifact.MediaTypeTar, artifact.MediaTypeTarGzip:
		return func(reader io.Reader, dest string) error {
			return extractTarballSkipping(reader, dest, isDistributionMetadata)
		}, true
	case artifact.MediaTypeZip:
		return func(reader io.Reader, dest string) error {
			return extractZipSkipping(reader, dest, isDistributionMetadata)
		}, true
	default:
		return nil, false
	}
}

// isDistributionMetadata reports whether an archive member belongs to the reserved distribution
// directory. The `.atmos` directory member itself is withheld too, because packaging adds it only
// to hold the distribution directory; files that a package really keeps under `.atmos` still create
// it when they are extracted.
func isDistributionMetadata(name string) bool {
	clean := path.Clean(strings.TrimPrefix(filepath.ToSlash(name), "./"))
	return clean == artifact.DistributionParent || clean == artifact.DistributionDir || strings.HasPrefix(clean, artifact.DistributionDir+"/")
}

// selectLayers returns the layers a consumer extracts. A legacy image extracts every layer. An Atmos
// package extracts its single payload layer and leaves the document blobs untouched.
func selectLayers(img v1.Image, layers []v1.Layer) ([]v1.Layer, error) {
	atmos, err := isAtmosPackage(img)
	if err != nil {
		return nil, err
	}
	if !atmos {
		return layers, nil
	}
	manifest, err := img.Manifest()
	if err != nil {
		return nil, err
	}
	var payload []v1.Layer
	for index := range manifest.Layers {
		if _, isPayload := atmosPayloadExtractor(string(manifest.Layers[index].MediaType)); isPayload && index < len(layers) {
			payload = append(payload, layers[index])
		}
	}
	if len(payload) != 1 {
		return nil, fmt.Errorf("%w: found %d payload layers", errUtils.ErrArtifactPayloadLayer, len(payload))
	}
	return payload, nil
}
