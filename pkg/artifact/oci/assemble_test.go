package oci

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

func parseManifest(t *testing.T, raw []byte) ocispec.Manifest {
	t.Helper()
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	return manifest
}

func TestAssemble_ManifestShape(t *testing.T) {
	prepared := prepare(t, publishSpec{content: "shape", labels: map[string]string{"team": "platform"}})
	pkg, err := Assemble(prepared)
	require.NoError(t, err)

	canonical, err := prepared.Record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, canonical, pkg.ConfigBytes, "the config blob is the canonical Record bytes")
	recordDigest, err := prepared.Record.RecordDigest()
	require.NoError(t, err)
	assert.Equal(t, recordDigest, pkg.Config.Digest)
	assert.Equal(t, artifact.MediaTypeConfig, pkg.Config.MediaType)

	manifest := parseManifest(t, pkg.ManifestBytes)
	assert.Equal(t, ocispec.MediaTypeImageManifest, manifest.MediaType)
	assert.Equal(t, artifact.MediaTypeArtifact, manifest.ArtifactType)
	assert.Equal(t, pkg.Config, manifest.Config)
	assert.Equal(t, preparedAt.Format(time.RFC3339), manifest.Annotations[ocispec.AnnotationCreated], "the captured creation time, not the clock")
	assert.Equal(t, appName, manifest.Annotations[ocispec.AnnotationTitle])
	assert.Equal(t, appVersion, manifest.Annotations[ocispec.AnnotationVersion])
	assert.Equal(t, "a test package", manifest.Annotations[ocispec.AnnotationDescription])

	require.Len(t, manifest.Layers, 3, "one archive payload plus README and CHANGELOG as separate blobs")
	assert.Equal(t, artifact.MediaTypeTarGzip, manifest.Layers[0].MediaType)
	assert.Equal(t, "content.tar.gz", manifest.Layers[0].Annotations[ocispec.AnnotationTitle])
	assert.Equal(t, artifact.DocumentMediaType, manifest.Layers[1].MediaType)
	assert.Equal(t, string(artifact.DocumentReadme), manifest.Layers[1].Annotations[annotationDocumentRole])
	assert.Equal(t, "README.md", manifest.Layers[1].Annotations[ocispec.AnnotationTitle])
	assert.Equal(t, string(artifact.DocumentChangelog), manifest.Layers[2].Annotations[annotationDocumentRole])
	assert.Equal(t, "CHANGELOG.md", manifest.Layers[2].Annotations[ocispec.AnnotationTitle])
	for _, doc := range prepared.Record.Documents {
		var found bool
		for _, layer := range manifest.Layers[1:] {
			found = found || (layer.Digest == doc.Digest && layer.Size == doc.Size)
		}
		assert.True(t, found, "%s layer matches the Record's descriptor", doc.Role)
	}
	assert.Len(t, pkg.Blobs, 3)
	assert.Equal(t, pkg.Manifest.Digest.String(), digestOf(pkg.ManifestBytes))
}

func digestOf(raw []byte) string { return digest.FromBytes(raw).String() }

func TestAssemble_IdenticalInputYieldsIdenticalManifestDigest(t *testing.T) {
	first, err := Assemble(prepare(t, publishSpec{content: "same"}))
	require.NoError(t, err)
	second, err := Assemble(prepare(t, publishSpec{content: "same"}))
	require.NoError(t, err)
	assert.Equal(t, first.Manifest.Digest, second.Manifest.Digest)
	assert.Equal(t, first.ManifestBytes, second.ManifestBytes)

	later, err := Assemble(prepare(t, publishSpec{content: "same", at: preparedAt.Add(time.Hour)}))
	require.NoError(t, err)
	assert.NotEqual(t, first.Manifest.Digest, later.Manifest.Digest, "the creation time is part of the Record")
	assert.NotEqual(t, first.Config.Digest, later.Config.Digest)

	changed, err := Assemble(prepare(t, publishSpec{content: "different"}))
	require.NoError(t, err)
	assert.NotEqual(t, first.Manifest.Digest, changed.Manifest.Digest)
}

func TestAssemble_PackagingChangesEnvelopeNotRecord(t *testing.T) {
	tgz, err := Assemble(prepare(t, publishSpec{content: "pack", packaging: artifact.PackagingTgz}))
	require.NoError(t, err)
	tar, err := Assemble(prepare(t, publishSpec{content: "pack", packaging: artifact.PackagingTar}))
	require.NoError(t, err)
	zip, err := Assemble(prepare(t, publishSpec{content: "pack", packaging: artifact.PackagingZip}))
	require.NoError(t, err)

	assert.Equal(t, tgz.Config.Digest, tar.Config.Digest, "Record and content identity do not depend on packaging")
	assert.Equal(t, tgz.Config.Digest, zip.Config.Digest)
	assert.NotEqual(t, tgz.Manifest.Digest, tar.Manifest.Digest, "the envelope digest does")
	assert.NotEqual(t, tgz.Manifest.Digest, zip.Manifest.Digest)
	assert.Equal(t, artifact.MediaTypeTar, parseManifest(t, tar.ManifestBytes).Layers[0].MediaType)
	assert.Equal(t, artifact.MediaTypeZip, parseManifest(t, zip.ManifestBytes).Layers[0].MediaType)
}

func TestAssemble_WithoutDocumentsHasOnlyThePayloadLayer(t *testing.T) {
	pkg, err := Assemble(prepare(t, publishSpec{content: "bare", noDocs: true}))
	require.NoError(t, err)
	assert.Len(t, parseManifest(t, pkg.ManifestBytes).Layers, 1)
}

func TestAssemble_Rejections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*artifact.PreparedArtifact) *artifact.PreparedArtifact
	}{
		{"nil", func(*artifact.PreparedArtifact) *artifact.PreparedArtifact { return nil }},
		{"unarchived packaging", func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			p.Packaging = artifact.PackagingNone
			return p
		}},
		{"no payload", func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			p.Blobs = p.Blobs[1:]
			return p
		}},
		{"two payloads", func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			p.Blobs = append(p.Blobs, p.Blobs[0])
			return p
		}},
		{"document the Record does not describe", func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			p.Blobs[1].DocumentRole = artifact.DocumentChangelog
			p.Record.Documents = p.Record.Documents[:1]
			return p
		}},
		{"missing staged document", func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			p.Blobs = p.Blobs[:2]
			return p
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared := prepare(t, publishSpec{content: "reject"})
			_, err := Assemble(tt.mutate(prepared))
			require.Error(t, err)
		})
	}
	_, err := Assemble(nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
}

func TestArtifactTypeFor(t *testing.T) {
	tests := map[string]string{
		"files":             artifact.MediaTypeArtifact,
		"scaffold":          artifact.MediaTypeScaffold,
		"scaffold/simple":   artifact.MediaTypeScaffold,
		"component":         artifact.MediaTypeComponent,
		"component/helm":    artifact.MediaTypeComponent,
		"toolchain/package": artifact.MediaTypeToolchainPackage,
		"catalog":           artifact.MediaTypeCatalog,
		"scaffolding":       artifact.MediaTypeArtifact,
		"other/kind":        artifact.MediaTypeArtifact,
	}
	for kind, want := range tests {
		assert.Equal(t, want, artifactTypeFor(kind), kind)
	}
}
