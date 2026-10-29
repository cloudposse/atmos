package oci

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"maps"
	"slices"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Files lists every file in every platform payload with its size and mode, sorted
// by platform and path. The listing is read from the verified release layers, so
// it shows exactly what a registry receives. Publish fills it while validating.
func (r *Release) Files() ([]BundleFile, error) {
	defer perf.Track(nil, "pluginoci.Release.Files")()

	if r.files != nil {
		return r.files, nil
	}
	var all []BundleFile
	for _, platform := range slices.Sorted(maps.Keys(r.Platforms)) {
		files, err := r.scanPlatform(platform, &bundleRules{})
		if err != nil {
			return nil, err
		}
		all = append(all, labelFiles(platform, files)...)
	}
	r.files = all
	return all, nil
}

// LayoutFiles lists the files inside every platform payload of a local OCI layout.
func LayoutFiles(layout string) ([]BundleFile, error) {
	defer perf.Track(nil, "pluginoci.LayoutFiles")()

	index, err := atmosoci.LoadIndex(layout, "")
	if err != nil {
		return nil, err
	}
	release, err := Inspect(index, "")
	if err != nil {
		return nil, err
	}
	return release.Files()
}

func labelFiles(platform string, files []BundleFile) []BundleFile {
	slices.SortFunc(files, func(a, b BundleFile) int { return strings.Compare(a.Path, b.Path) })
	for index := range files {
		files[index].Platform = platform
	}
	return files
}

// digestWriter hashes and counts the bytes read through a tee.
type digestWriter struct {
	hash hash.Hash
	size int64
}

func (w *digestWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "pluginoci.digestWriter.Write")()

	w.size += int64(len(p))
	return w.hash.Write(p)
}

// scanPlatform streams one platform's payload through scanBundle. The layer digest
// and size are verified after the walk by draining the stream, so a corrupt blob
// fails closed without ever buffering the whole payload.
func (r *Release) scanPlatform(platform string, rules *bundleRules) ([]BundleFile, error) {
	descriptor, found := r.Platforms[platform]
	if !found {
		return nil, fmt.Errorf("%w: platform %s is not available", errUtils.ErrPluginInvalidConfig, platform)
	}
	img, err := r.Index.Image(descriptor.Digest)
	if err != nil {
		return nil, err
	}
	definition, err := verifiedManifest(img, &descriptor)
	if err != nil {
		return nil, err
	}
	expected := definition.Layers[0]
	if expected.Size < 0 || expected.Size > maxBundleSize {
		return nil, errUtils.ErrOCIIntegrity
	}
	layer, err := img.LayerByDigest(expected.Digest)
	if err != nil {
		return nil, err
	}
	reader, err := layer.Compressed()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return scanVerifiedBundle(reader, &expected, rules)
}

func scanVerifiedBundle(reader io.Reader, expected *v1.Descriptor, rules *bundleRules) ([]BundleFile, error) {
	digest := &digestWriter{hash: sha256.New()}
	stream := io.TeeReader(io.LimitReader(reader, maxBundleSize+1), digest)
	files, err := scanBundle(stream, rules)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(io.Discard, stream); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrOCIIntegrity, err)
	}
	if digest.size != expected.Size || fmt.Sprintf("sha256:%x", digest.hash.Sum(nil)) != expected.Digest.String() {
		return nil, errUtils.ErrOCIIntegrity
	}
	return files, nil
}
