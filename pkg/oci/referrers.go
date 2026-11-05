package oci

import (
	"context"
	"errors"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ReferrerRegistry extends OCI transport with OCI 1.1 attachment discovery.
type ReferrerRegistry interface {
	Registry
	Referrers(context.Context, name.Digest) (v1.ImageIndex, error)
}

// Referrers uses the shared credentials and transport, including the standard
// referrers-tag fallback on registries without the OCI 1.1 API.
func (c *Client) Referrers(ctx context.Context, ref name.Digest) (v1.ImageIndex, error) {
	defer perf.Track(c.config, "oci.Client.Referrers")()
	options, err := c.options(ctx, ref.Context())
	if err != nil {
		return nil, err
	}
	index, err := remote.Referrers(ref, options...)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return index, nil
}
