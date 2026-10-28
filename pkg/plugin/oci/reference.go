package oci

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxTagsInspected bounds the registry lookups made to list the tags of one digest.
const maxTagsInspected = 200

// ParseDestination validates a publish destination: an OCI repository without a
// tag or digest. The release tag always comes from the manifest version, so a tag
// in the destination would be ignored or contradict it.
func ParseDestination(destination string) (name.Repository, error) {
	defer perf.Track(nil, "pluginoci.ParseDestination")()

	trimmed := strings.TrimPrefix(destination, "oci://")
	repository, err := name.NewRepository(trimmed, name.StrictValidation)
	if err == nil {
		return repository, nil
	}
	cause := fmt.Errorf("%w: %q is not an OCI repository", errUtils.ErrPluginInvalidConfig, destination)
	// A bare repository always parses above, so a valid reference here carries a tag or digest.
	if _, refErr := name.ParseReference(trimmed, name.StrictValidation); refErr == nil {
		cause = fmt.Errorf("%w: %q includes a tag or digest", errUtils.ErrPluginInvalidConfig, destination)
	}
	return name.Repository{}, errUtils.Build(cause).
		WithHint("Pass only the repository, for example `--to=oci://registry.example.com/org/plugin`. The release tag is taken from `metadata.version` in plugin.yaml.").
		Err()
}

// ParseDigestReference validates a `registry/repository@sha256:digest` reference.
func ParseDigestReference(value string) (name.Digest, error) {
	defer perf.Track(nil, "pluginoci.ParseDigestReference")()

	reference, err := name.NewDigest(strings.TrimPrefix(value, "oci://"), name.StrictValidation)
	if err != nil {
		return name.Digest{}, errUtils.Build(fmt.Errorf("%w: %q is not a repository digest reference", errUtils.ErrPluginInvalidConfig, value)).
			WithHint("Use the form `registry.example.com/org/plugin@sha256:<64 hex characters>`; the digest is printed by `atmos plugin publish`.").
			Err()
	}
	return reference, nil
}

// DeletionTarget describes what removing a digest would affect.
type DeletionTarget struct {
	Repository string
	Digest     string
	// Tags are the repository tags that currently resolve to the digest.
	Tags []string
}

// DescribeDeletion lists the tags that point at the digest so the caller can show
// them before a destructive delete. Tag lookups are best effort: a registry that
// cannot list tags still allows deletion by digest.
func DescribeDeletion(ctx context.Context, client atmosoci.Registry, reference name.Digest) DeletionTarget {
	defer perf.Track(nil, "pluginoci.DescribeDeletion")()

	target := DeletionTarget{Repository: reference.Context().Name(), Digest: reference.DigestStr()}
	tags, err := client.Tags(ctx, reference.Context())
	if err != nil {
		return target
	}
	for _, tag := range tags[:min(len(tags), maxTagsInspected)] {
		descriptor, err := client.Head(ctx, reference.Context().Tag(tag))
		if err == nil && descriptor.Digest.String() == reference.DigestStr() {
			target.Tags = append(target.Tags, tag)
		}
	}
	slices.Sort(target.Tags)
	return target
}
