package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	configType     types.MediaType = "application/vnd.atmos.plugin.catalog.config.v1+json"
	layerType      types.MediaType = "application/vnd.atmos.plugin.catalog.v1+json"
	maxCatalogSize                 = 8 << 20
)

// Read fetches a bounded catalog and verifies its descriptor and layer bytes.
// Once a manifest is found, its digest is returned even if its content is unreadable.
// Callers can distinguish an absent catalog from a present but corrupt artifact.
func Read(ctx context.Context, registry atmosoci.Registry, reference string) (*File, string, error) {
	defer perf.Track(nil, "plugincatalog.Read")()

	ref, err := name.ParseReference(strings.TrimPrefix(reference, "oci://"))
	if err != nil {
		return nil, "", err
	}
	descriptor, err := registry.Get(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	img, err := descriptor.Image()
	if err != nil {
		return nil, descriptor.Digest.String(), err
	}
	raw, err := readCatalogLayer(img, &descriptor.Descriptor)
	if err != nil {
		return nil, descriptor.Digest.String(), err
	}
	file, err := Parse(raw)
	return file, descriptor.Digest.String(), err
}

// Publish checks the caller's observed revision and digest immediately before
// replacement. Publishers must serialize writes; registries without CAS cannot
// make the final check and tag update atomic.
func Publish(ctx context.Context, registry atmosoci.Registry, reference string, file *File, expected string) error {
	defer perf.Track(nil, "plugincatalog.Publish")()

	ref, err := name.NewTag(strings.TrimPrefix(reference, "oci://"))
	if err != nil {
		return err
	}
	if err := checkRevision(ctx, registry, reference, file, expected); err != nil {
		return err
	}
	copy := *file
	copy.Metadata.Revision++
	copy.Metadata.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.Marshal(copy)
	if err != nil {
		return err
	}
	if len(raw) > maxCatalogSize {
		return errUtils.ErrPluginPayloadTooLarge
	}
	if _, err := Parse(raw); err != nil {
		return err
	}
	img, err := mutate.AppendLayers(empty.Image, static.NewLayer(raw, layerType))
	if err != nil {
		return err
	}
	img = mutate.ConfigMediaType(mutate.MediaType(img, types.OCIManifestSchema1), configType)
	return registry.WriteImage(ctx, ref, img)
}

func readCatalogLayer(img v1.Image, descriptor *v1.Descriptor) ([]byte, error) {
	rawManifest, err := img.RawManifest()
	if err != nil {
		return nil, err
	}
	if _, err := atmosoci.ReadVerifiedBlob(bytes.NewReader(rawManifest), descriptor, maxCatalogSize); err != nil {
		return nil, err
	}
	var definition v1.Manifest
	if err := json.Unmarshal(rawManifest, &definition); err != nil {
		return nil, err
	}
	if definition.Config.MediaType != configType || len(definition.Layers) != 1 || definition.Layers[0].MediaType != layerType {
		return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("The reference does not point at an Atmos plugin catalog.").
			WithHint("Pass a catalog published with `atmos plugin catalog publish`, not a plugin release or another artifact.").Err()
	}
	layer, err := img.LayerByDigest(definition.Layers[0].Digest)
	if err != nil {
		return nil, err
	}
	reader, err := layer.Compressed()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	raw, err := atmosoci.ReadVerifiedBlob(reader, &definition.Layers[0], maxCatalogSize)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func checkRevision(ctx context.Context, registry atmosoci.Registry, reference string, file *File, expected string) error {
	current, digest, err := Read(ctx, registry, reference)
	if err != nil && !missingCatalog(digest, err) {
		return err
	}
	if err == nil {
		if digest != expected || current.Metadata.Revision != file.Metadata.Revision {
			return errUtils.Build(errUtils.ErrPluginCatalogConflict).
				WithHint("Someone published a newer revision. Rerun the command so it starts from the latest catalog.").Err()
		}
	} else if expected != "" || file.Metadata.Revision != 0 {
		return errUtils.Build(errUtils.ErrPluginCatalogConflict).
			WithHint("The catalog was deleted since it was read. Rerun the command to start from the current state.").Err()
	}
	return nil
}

// missingCatalog permits creation only when the manifest itself was absent.
func missingCatalog(digest string, err error) bool {
	return digest == "" && atmosoci.IsNotFound(err)
}
