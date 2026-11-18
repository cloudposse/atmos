package oci

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
)

// foreignImage describes a native image or index pushed with a third-party client.
type foreignImage struct {
	manifest digest.Digest
	config   digest.Digest
	layers   []digest.Digest
	children []digest.Digest
}

// pushImage pushes a random single-layer image.
func pushImage(t *testing.T, reg *registry, repository, tag string) foreignImage {
	t.Helper()
	image, err := random.Image(256, 1)
	require.NoError(t, err)
	reference, err := name.ParseReference(reg.server.Host + "/" + repository + ":" + tag)
	require.NoError(t, err)
	require.NoError(t, remote.Write(reference, image, remote.WithTransport(reg.recorder), remote.WithAuth(reg.auth())))
	return describeImage(t, image)
}

func describeImage(t *testing.T, image v1.Image) foreignImage {
	t.Helper()
	described := foreignImage{}
	manifestDigest, err := image.Digest()
	require.NoError(t, err)
	described.manifest = digest.Digest(manifestDigest.String())
	config, err := image.ConfigName()
	require.NoError(t, err)
	described.config = digest.Digest(config.String())
	layers, err := image.Layers()
	require.NoError(t, err)
	for _, layer := range layers {
		layerDigest, err := layer.Digest()
		require.NoError(t, err)
		described.layers = append(described.layers, digest.Digest(layerDigest.String()))
	}
	return described
}

// pushIndex pushes a multi-platform index of one random image per platform.
func pushIndex(t *testing.T, reg *registry, repository, tag string, platforms ...v1.Platform) foreignImage {
	t.Helper()
	var index v1.ImageIndex = empty.Index
	var children []digest.Digest
	for _, platform := range platforms {
		image, err := random.Image(256, 2)
		require.NoError(t, err)
		index = mutate.AppendManifests(index, mutate.IndexAddendum{Add: image, Descriptor: v1.Descriptor{Platform: &platform}})
		imageDigest, err := image.Digest()
		require.NoError(t, err)
		children = append(children, digest.Digest(imageDigest.String()))
	}
	reference, err := name.ParseReference(reg.server.Host + "/" + repository + ":" + tag)
	require.NoError(t, err)
	require.NoError(t, remote.WriteIndex(reference, index, remote.WithTransport(reg.recorder), remote.WithAuth(reg.auth())))
	indexDigest, err := index.Digest()
	require.NoError(t, err)
	return foreignImage{manifest: digest.Digest(indexDigest.String()), children: children}
}
