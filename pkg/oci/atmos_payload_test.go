package oci

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// Compile-time guard for the artifact media types these tests build packages from.
var _ = []string{artifact.MediaTypeConfig, artifact.MediaTypeTar, artifact.MediaTypeTarGzip, artifact.MediaTypeZip, artifact.DocumentMediaType}

// atmosFiles is a payload as packaging produces it: the user's files plus the generated
// distribution directory, with the `.atmos` directory member that holds it.
func atmosFiles() map[string]string {
	return map[string]string{
		"main.tf":                       "# main\n",
		".terraform.lock.hcl":           "# lock\n",
		"scripts/helper.sh":             "#!/bin/sh\n",
		".atmos/artifact/manifest.json": "{}\n",
		".atmos/artifact/README.md":     "# package readme\n",
	}
}

// atmosLayer builds a payload layer. A gzip layer is compressed the way a registry serves it, so
// the layer's Uncompressed stream is the plain tar, like a layer read back from a registry.
func atmosLayer(t *testing.T, mediaType types.MediaType, files map[string]string) v1.Layer {
	t.Helper()
	switch string(mediaType) {
	case artifact.MediaTypeZip:
		return static.NewLayer(writeTestZip(t, files).Bytes(), mediaType)
	case artifact.MediaTypeTarGzip:
		data := writeTestTar(t, files).Bytes()
		layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }, tarball.WithMediaType(mediaType))
		require.NoError(t, err)
		return layer
	default:
		return static.NewLayer(writeTestTar(t, files).Bytes(), mediaType)
	}
}

func atmosPackage(t *testing.T, layers ...v1.Layer) v1.Image {
	t.Helper()
	img := mutate.ConfigMediaType(empty.Image, types.MediaType(artifact.MediaTypeConfig))
	img, err := mutate.AppendLayers(img, layers...)
	require.NoError(t, err)
	return img
}

func documentLayer(content string) v1.Layer {
	return static.NewLayer([]byte(content), types.MediaType(artifact.DocumentMediaType))
}

func TestSelectLayers_AtmosPackageExtractsOnlyThePayload(t *testing.T) {
	payload := atmosLayer(t, types.MediaType(artifact.MediaTypeTarGzip), atmosFiles())
	img := atmosPackage(t, payload, documentLayer("# readme\n"), documentLayer("# changelog\n"))
	layers, err := img.Layers()
	require.NoError(t, err)
	require.Len(t, layers, 3)

	selected, err := selectLayers(img, layers)

	require.NoError(t, err)
	require.Len(t, selected, 1)
	want, err := payload.Digest()
	require.NoError(t, err)
	got, err := selected[0].Digest()
	require.NoError(t, err)
	assert.Equal(t, want, got, "the archive payload is selected, not the Markdown documents")
}

func TestSelectLayers_LegacyImageKeepsEveryLayer(t *testing.T) {
	img, err := mutate.AppendLayers(
		empty.Image,
		static.NewLayer(writeTestTar(t, map[string]string{"a.tf": "a"}).Bytes(), types.OCILayer),
		static.NewLayer(writeTestTar(t, map[string]string{"b.tf": "b"}).Bytes(), types.OCILayer),
	)
	require.NoError(t, err)
	layers, err := img.Layers()
	require.NoError(t, err)

	selected, err := selectLayers(img, layers)

	require.NoError(t, err)
	assert.Len(t, selected, 2, "legacy component and scaffold layer heuristics are untouched")
}

func TestSelectLayers_RejectsPackagesWithoutExactlyOnePayload(t *testing.T) {
	tarLayer := func(name string) v1.Layer {
		return atmosLayer(t, types.MediaType(artifact.MediaTypeTar), map[string]string{name: "x"})
	}
	tests := []struct {
		name   string
		layers []v1.Layer
	}{
		{name: "documents only", layers: []v1.Layer{documentLayer("# readme\n")}},
		{name: "two payloads", layers: []v1.Layer{tarLayer("a"), tarLayer("b")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := atmosPackage(t, tt.layers...)
			layers, err := img.Layers()
			require.NoError(t, err)

			_, err = selectLayers(img, layers)

			require.ErrorIs(t, err, errUtils.ErrArtifactPayloadLayer)
		})
	}
}

func TestProcessLayer_AtmosPayloadWithholdsDistributionMetadata(t *testing.T) {
	for _, mediaType := range []string{artifact.MediaTypeTar, artifact.MediaTypeTarGzip, artifact.MediaTypeZip} {
		t.Run(mediaType, func(t *testing.T) {
			dest := t.TempDir()
			files := atmosFiles()
			layer := atmosLayer(t, types.MediaType(mediaType), files)

			require.NoError(t, processLayer(layer, 0, dest))

			for _, kept := range []string{"main.tf", ".terraform.lock.hcl", filepath.Join("scripts", "helper.sh")} {
				assert.FileExists(t, filepath.Join(dest, kept))
			}
			assert.NoDirExists(t, filepath.Join(dest, ".atmos"), "distribution metadata never reaches the consumer")
		})
	}
}

func TestProcessLayer_AtmosPayloadKeepsUserFilesUnderDotAtmos(t *testing.T) {
	dest := t.TempDir()
	layer := atmosLayer(t, types.MediaType(artifact.MediaTypeTar), map[string]string{
		"main.tf":                   "# main\n",
		".atmos/scaffold.yaml":      "kind: AtmosScaffoldConfig\n",
		".atmos/artifact/README.md": "# package readme\n",
	})

	require.NoError(t, processLayer(layer, 0, dest))

	assert.FileExists(t, filepath.Join(dest, ".atmos", "scaffold.yaml"), "only the reserved distribution directory is withheld")
	assert.NoFileExists(t, filepath.Join(dest, ".atmos", "artifact", "README.md"))
}

func TestProcessLayer_AtmosPayloadKeepsExecutableBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this platform does not honor Unix permission bits")
	}
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	content := "#!/bin/sh\n"
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "scripts/helper.sh", Mode: 0o755, Size: int64(len(content))}))
	_, err := writer.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	dest := t.TempDir()

	require.NoError(t, processLayer(static.NewLayer(buf.Bytes(), types.MediaType(artifact.MediaTypeTar)), 0, dest))

	info, err := os.Stat(filepath.Join(dest, "scripts", "helper.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestIsDistributionMetadata(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: ".atmos", want: true},
		{name: ".atmos/", want: true},
		{name: "./.atmos/artifact", want: true},
		{name: ".atmos/artifact/", want: true},
		{name: ".atmos/artifact/manifest.json", want: true},
		{name: ".atmos/artifact/nested/x", want: true},
		{name: ".atmos/scaffold.yaml", want: false},
		{name: ".atmos/artifacts/x", want: false},
		{name: "docs/.atmos/artifact/x", want: false},
		{name: "main.tf", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDistributionMetadata(tt.name))
		})
	}
}

func TestSelectedLayers_AtmosPackageExtractsThroughTheRetryPath(t *testing.T) {
	// A package shaped the way publishing shapes it extracts through the same per-layer retry path ProcessImage uses.
	dest := t.TempDir()
	img := atmosPackage(t, atmosLayer(t, types.MediaType(artifact.MediaTypeTarGzip), atmosFiles()), documentLayer("# readme\n"))
	layers, err := img.Layers()
	require.NoError(t, err)
	selected, err := selectLayers(img, layers)
	require.NoError(t, err)

	for index, layer := range selected {
		require.NoError(t, processLayerWithRetry(context.Background(), layer, index, dest, defaultOCILayerRetryConfig()))
	}

	assert.FileExists(t, filepath.Join(dest, "main.tf"))
	assert.NoDirExists(t, filepath.Join(dest, ".atmos"))
}
