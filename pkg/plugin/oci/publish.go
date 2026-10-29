package oci

import (
	"context"
	"maps"
	"os"
	"slices"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
)

// Publish validates all platform payloads and writes the immutable version tag.
// Publishing identical bytes is idempotent. Publishers must serialize releases
// per repository because the OCI distribution API has no tag compare-and-swap.
//
//nolint:revive // Preserve the public publishing API and its functional options.
func Publish(ctx context.Context, client atmosoci.Registry, directory, destination string, descriptors *extension.Registry, opts ...Option) (*Release, error) {
	defer perf.Track(nil, "pluginoci.Publish")()

	if err := pluginpolicy.Source(ctx, "publish", destination); err != nil {
		return nil, err
	}
	repository, err := ParseDestination(destination)
	if err != nil {
		return nil, err
	}
	index, err := atmosoci.LoadIndex(directory, "")
	if err != nil {
		return nil, err
	}
	release, err := Inspect(index, "")
	if err != nil {
		return nil, err
	}
	if err := pluginpolicy.Package(ctx, "publish", destination, release.Definition, pluginpolicy.WithDigest(release.Digest), pluginpolicy.WithExtensions(descriptors)); err != nil {
		return nil, err
	}
	if err := validatePayloads(release, descriptors, newOptions(opts)); err != nil {
		return nil, err
	}
	if err := publishIndex(ctx, client, repository.Tag(release.Version), release, index); err != nil {
		return nil, err
	}
	return release, nil
}

func publishIndex(ctx context.Context, client atmosoci.Registry, reference name.Reference, release *Release, index v1.ImageIndex) error {
	existing, err := client.Head(ctx, reference)
	if err == nil {
		if existing.Digest.String() != release.Digest {
			return errUtils.ErrPluginImmutableRelease
		}
		return nil
	}
	if !atmosoci.IsNotFound(err) {
		return err
	}
	if err := client.WriteIndex(ctx, reference, index); err != nil {
		return err
	}
	return checkPublished(ctx, client, reference, release.Digest)
}

func validatePayloads(release *Release, descriptors *extension.Registry, configured *options) error {
	if err := checkDeclaredPaths(declaredExecutables(release.Definition)...); err != nil {
		return err
	}
	var reference map[string][]byte
	var inventory []BundleFile
	for _, platform := range slices.Sorted(maps.Keys(release.Platforms)) {
		listed, err := inspectPayload(release, platform, configured)
		if err != nil {
			return err
		}
		inventory = append(inventory, listed...)
		files, err := validatePayload(release, platform, descriptors)
		if err != nil {
			return err
		}
		if reference == nil {
			reference = files
		}
		if !equalFiles(reference, files) {
			return errUtils.ErrOCIIntegrity
		}
	}
	release.files = inventory
	return nil
}

// inspectPayload streams one platform's layer, re-checking every tar entry
// against the path guard and the exact release file set. Layouts can come from
// other tools or older versions, so Publish never trusts how they were built.
func inspectPayload(release *Release, platform string, configured *options) ([]BundleFile, error) {
	parsed, err := parsePlatform(platform)
	if err != nil {
		return nil, err
	}
	if configured.checkExecutableFormat {
		if err := checkRuntimeExecutable(release.Definition.Spec.Runtime.Executable, parsed); err != nil {
			return nil, err
		}
	}
	rules := &bundleRules{executable: release.Definition.Spec.Runtime.Executable, exactSet: true, platform: parsed, checkFormat: configured.checkExecutableFormat}
	files, err := release.scanPlatform(platform, rules)
	if err != nil {
		return nil, err
	}
	return labelFiles(platform, files), nil
}

func validatePayload(release *Release, platform string, descriptors *extension.Registry) (map[string][]byte, error) {
	directory, err := os.MkdirTemp("", "atmos-plugin-validate-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	if err := release.Extract(platform, directory); err != nil {
		return nil, err
	}
	if _, err := descriptors.Validate(directory, release.Definition.Spec.Capabilities); err != nil {
		return nil, err
	}
	return packageFiles(directory)
}

func equalFiles(first, second map[string][]byte) bool {
	if len(first) != len(second) {
		return false
	}
	for path, content := range first {
		if string(content) != string(second[path]) {
			return false
		}
	}
	return true
}

func checkPublished(ctx context.Context, client atmosoci.Registry, reference name.Reference, digest string) error {
	actual, err := client.Head(ctx, reference)
	if err != nil {
		return err
	}
	if actual.Digest.String() != digest {
		return errUtils.ErrPluginImmutableRelease
	}
	return nil
}
