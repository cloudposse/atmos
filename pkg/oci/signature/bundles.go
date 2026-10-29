package signature

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	bundleFileMode    = 0o600
	MaxBundles        = 32
	bundleFile        = "signatures.json"
	maxCollectionSize = 16 << 20
)

// Fetch reads bounded Sigstore bundle attachments for the exact subject digest.
func Fetch(ctx context.Context, registry oci.ReferrerRegistry, subject name.Digest) ([][]byte, error) {
	defer perf.Track(nil, "signature.Fetch")()
	index, err := registry.Referrers(ctx, subject)
	if err != nil {
		return nil, err
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(manifest.Manifests) > MaxBundles {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	var bundles [][]byte
	total := 0
	for i := range manifest.Manifests {
		descriptor := &manifest.Manifests[i]
		if !strings.HasPrefix(descriptor.ArtifactType, "application/vnd.dev.sigstore.bundle.") {
			continue
		}
		raw, err := fetchBundle(ctx, registry, subject, descriptor)
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > maxCollectionSize {
			return nil, errUtils.ErrPluginPayloadTooLarge
		}
		bundles = append(bundles, raw)
	}
	return bundles, nil
}

func fetchBundle(ctx context.Context, registry oci.Registry, subject name.Digest, expected *v1.Descriptor) ([]byte, error) {
	descriptor, err := registry.Get(ctx, subject.Context().Digest(expected.Digest.String()))
	if err != nil {
		return nil, err
	}
	if _, err := oci.ReadVerifiedBlob(bytes.NewReader(descriptor.Manifest), expected, MaxBundleSize); err != nil {
		return nil, err
	}
	image, err := descriptor.Image()
	if err != nil {
		return nil, err
	}
	manifest, err := image.Manifest()
	if err != nil {
		return nil, err
	}
	if manifest.Subject == nil || manifest.Subject.Digest.String() != subject.DigestStr() || len(manifest.Layers) != 1 {
		return nil, errUtils.ErrOCIIntegrity
	}
	layer, err := image.LayerByDigest(manifest.Layers[0].Digest)
	if err != nil {
		return nil, err
	}
	reader, err := layer.Compressed()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return oci.ReadVerifiedBlob(reader, &manifest.Layers[0], MaxBundleSize)
}

// Load reads cached evidence or a collection accompanying an offline OCI layout.
// A single standard bundle can instead be supplied as signature.sigstore.json.
func Load(directory string) ([][]byte, error) {
	defer perf.Track(nil, "signature.Load")()
	raw, err := readBounded(filepath.Join(directory, bundleFile), maxCollectionSize)
	if errors.Is(err, os.ErrNotExist) {
		raw, err = readBounded(filepath.Join(directory, "signature.sigstore.json"), MaxBundleSize)
		if err != nil {
			return nil, err
		}
		return [][]byte{raw}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	if len(items) > MaxBundles {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	result := make([][]byte, len(items))
	for index, item := range items {
		if len(item) > MaxBundleSize {
			return nil, errUtils.ErrPluginPayloadTooLarge
		}
		result[index] = item
	}
	return result, nil
}

// Save persists evidence, not a trusted verdict. Every consumer verifies it anew.
func Save(directory string, bundles [][]byte) error {
	defer perf.Track(nil, "signature.Save")()
	if len(bundles) > MaxBundles {
		return errUtils.ErrPluginPayloadTooLarge
	}
	items := make([]json.RawMessage, len(bundles))
	for index, item := range bundles {
		if len(item) > MaxBundleSize {
			return errUtils.ErrPluginPayloadTooLarge
		}
		items[index] = item
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	if len(raw) > maxCollectionSize {
		return errUtils.ErrPluginPayloadTooLarge
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(filepath.Join(directory, bundleFile), raw, bundleFileMode)
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	return raw, nil
}
