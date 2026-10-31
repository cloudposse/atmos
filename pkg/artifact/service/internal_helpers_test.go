package service

import (
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func ocispecDescriptor() ocispec.Descriptor {
	return ocispec.Descriptor{MediaType: ocispec.MediaTypeImageIndex, Digest: digest.FromString("root"), Size: 4}
}
