package policy

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

const registryRead = "registry.read"

// Registry admits each actual destination before OCI resolves credentials or performs I/O.
// It also covers discovery, publication, deletion, and signature attachment transport.
type Registry struct{ Next oci.Registry }

// Get admits fetching plugin or catalog metadata.
func (r Registry) Get(ctx context.Context, ref name.Reference) (*remote.Descriptor, error) {
	defer perf.Track(nil, "pluginpolicy.Registry.Get")()
	if err := Source(ctx, registryRead, ref.Context().Name()); err != nil {
		return nil, err
	}
	return r.Next.Get(ctx, ref)
}

// Head admits reading an OCI descriptor.
func (r Registry) Head(ctx context.Context, ref name.Reference) (*v1.Descriptor, error) {
	defer perf.Track(nil, "pluginpolicy.Registry.Head")()
	if err := Source(ctx, registryRead, ref.Context().Name()); err != nil {
		return nil, err
	}
	return r.Next.Head(ctx, ref)
}

// Tags admits release discovery.
func (r Registry) Tags(ctx context.Context, repository name.Repository) ([]string, error) {
	defer perf.Track(nil, "pluginpolicy.Registry.Tags")()
	if err := Source(ctx, registryRead, repository.Name()); err != nil {
		return nil, err
	}
	return r.Next.Tags(ctx, repository)
}

// WriteImage admits catalog and signature publication.
func (r Registry) WriteImage(ctx context.Context, ref name.Reference, img v1.Image) error {
	defer perf.Track(nil, "pluginpolicy.Registry.WriteImage")()
	if err := Source(ctx, "registry.write", ref.Context().Name()); err != nil {
		return err
	}
	return r.Next.WriteImage(ctx, ref, img)
}

// WriteIndex admits plugin publication.
func (r Registry) WriteIndex(ctx context.Context, ref name.Reference, index v1.ImageIndex) error {
	defer perf.Track(nil, "pluginpolicy.Registry.WriteIndex")()
	if err := Source(ctx, "registry.write", ref.Context().Name()); err != nil {
		return err
	}
	return r.Next.WriteIndex(ctx, ref, index)
}

// Delete admits remote artifact deletion; local removal remains possible.
func (r Registry) Delete(ctx context.Context, ref name.Digest) error {
	defer perf.Track(nil, "pluginpolicy.Registry.Delete")()
	if err := Source(ctx, "registry.delete", ref.Context().Name()); err != nil {
		return err
	}
	return r.Next.Delete(ctx, ref)
}

// Referrers preserves signature discovery while enforcing destination policy.
func (r Registry) Referrers(ctx context.Context, ref name.Digest) (v1.ImageIndex, error) {
	defer perf.Track(nil, "pluginpolicy.Registry.Referrers")()
	if err := Source(ctx, registryRead, ref.Context().Name()); err != nil {
		return nil, err
	}
	registry, ok := r.Next.(oci.ReferrerRegistry)
	if !ok {
		return nil, errUtils.ErrPluginSignatureInvalid
	}
	return registry.Referrers(ctx, ref)
}
