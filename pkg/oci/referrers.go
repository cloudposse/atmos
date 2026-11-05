package oci

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ReferrerRegistry extends OCI transport with OCI 1.1 attachment discovery.
type ReferrerRegistry interface {
	Registry
	Referrers(context.Context, name.Digest) (v1.ImageIndex, error)
}

// Referrers uses the shared credentials and transport, including the standard
// referrers-tag fallback on registries without the OCI 1.1 API. Attachment
// descriptors are returned as an index; no content is fetched.
func (c *Client) Referrers(ctx context.Context, ref name.Digest) (v1.ImageIndex, error) {
	defer perf.Track(c.config, "oci.Client.Referrers")()

	store, err := c.Repository(ctx, repositoryOf(ref.Context()))
	if err != nil {
		return nil, err
	}
	found, err := store.Referrers(ctx, ocispec.Descriptor{Digest: digest.Digest(ref.DigestStr())}, "")
	if err != nil {
		return nil, err
	}
	index, err := newReferrersIndex(found)
	if err != nil {
		return nil, err
	}
	return index, nil
}
