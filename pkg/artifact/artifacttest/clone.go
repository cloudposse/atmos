package artifacttest

import (
	"bytes"
	"maps"
	"slices"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// The repository never shares maps or slices with its callers: whatever crosses the boundary in
// either direction is copied, so a caller editing its own value can never change a stored record,
// a capability guarantee, or a descriptor after the fact.

// cloneDescriptor returns a descriptor that does not share maps, slices, or pointers with d.
func cloneDescriptor(in *ocispec.Descriptor) ocispec.Descriptor {
	d := *in
	d.URLs = slices.Clone(d.URLs)
	d.Annotations = maps.Clone(d.Annotations)
	d.Data = bytes.Clone(d.Data)
	if d.Platform != nil {
		platform := *d.Platform
		platform.OSFeatures = slices.Clone(platform.OSFeatures)
		d.Platform = &platform
	}
	return d
}

// cloneEvidence returns evidence whose descriptor does not share storage with e.
func cloneEvidence(in *artifact.SignatureEvidence) artifact.SignatureEvidence {
	e := *in
	e.Descriptor = cloneDescriptor(&in.Descriptor)
	return e
}
