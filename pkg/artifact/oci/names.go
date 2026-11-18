package oci

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// Annotation keys written by this package. They select and describe evidence and documents; they
// never establish trust, which always comes from cryptographic verification.
const (
	// Annotation naming the signature subject kind (record or oci-root) of an attachment.
	annotationSubjectKind = "dev.atmos.artifact.signature.subject-kind"

	// Annotation carrying the digest of the signed subject.
	annotationSubjectDigest = "dev.atmos.artifact.signature.subject-digest"

	// Annotation binding a Record signature to the addressed manifest's config descriptor.
	annotationConfigDigest = "dev.atmos.artifact.signature.config-digest"

	// Annotation naming the evidence format of an attachment.
	annotationEvidenceFormat = "dev.atmos.artifact.signature.format"

	// Annotation naming the role (readme or changelog) of a document layer.
	annotationDocumentRole = "dev.atmos.artifact.document.role"

	// Annotation recording the artifact kind on the manifest for registry-side browsing.
	annotationKind = "dev.atmos.artifact.kind"
)

// Legacy Atmos plugin release format, as written by pkg/plugin/oci. The constants are repeated
// here, not imported, so generic OCI repositories stay independent of plugin packages; a drift
// test compares them with the plugin package.
const (
	// Config media type of a plugin platform image.
	pluginConfigMediaType = "application/vnd.atmos.plugin.config.v1+json"

	// Layer media type of a plugin platform bundle.
	pluginBundleMediaType = "application/vnd.atmos.plugin.bundle.v1+tar"

	// Release annotation naming the plugin.
	pluginAnnotationName = "dev.atmos.plugin.name"

	// Release annotation carrying the plugin version.
	pluginAnnotationVersion = "dev.atmos.plugin.version"

	// Release annotation carrying the manifest digest.
	pluginAnnotationManifest = "dev.atmos.plugin.manifest.digest"

	// Namespace shared by every plugin-format annotation and label.
	pluginPrefix = "dev.atmos.plugin."
)

// Sigstore bundle attachments.
const (
	// Media type prefix of Sigstore bundles.
	bundleMediaTypePrefix = "application/vnd.dev.sigstore.bundle."

	// Layer title of a stored bundle.
	bundleFileTitle = "signature.sigstore.json"
)

// Bounds on what one operation reads or scans.
const (
	// Bound on one manifest or index read.
	maxManifestSize int64 = 4 << 20

	// Bound on how many referrer manifests one verification or export inspects.
	maxAttachments = 32

	// Bound on how many tags a by-record-digest lookup or sibling check inspects.
	maxTagsScanned = 512

	// List page size when the query does not choose one.
	defaultPageSize = 50

	// Bound on the per-repository cache of verified manifests and configs.
	manifestCacheEntries = 256
)

// Layer annotation keys from the OCI image spec.
const (
	annotationTitle       = ocispec.AnnotationTitle
	annotationCreated     = ocispec.AnnotationCreated
	annotationVersion     = ocispec.AnnotationVersion
	annotationDescription = ocispec.AnnotationDescription
)

// artifactTypeFor maps an artifact kind onto the PRD artifact type of its manifest. Unlisted kinds
// use the generic artifact type; the component subtype stays in the portable Record.
func artifactTypeFor(kind string) string {
	switch {
	case kind == "scaffold" || hasKindPrefix(kind, "scaffold"):
		return artifact.MediaTypeScaffold
	case kind == "component" || hasKindPrefix(kind, "component"):
		return artifact.MediaTypeComponent
	case kind == "toolchain/package" || hasKindPrefix(kind, "toolchain/package"):
		return artifact.MediaTypeToolchainPackage
	case kind == artifact.KindCatalog:
		return artifact.MediaTypeCatalog
	default:
		return artifact.MediaTypeArtifact
	}
}

func hasKindPrefix(kind, prefix string) bool {
	return len(kind) > len(prefix) && kind[:len(prefix)] == prefix && kind[len(prefix)] == '/'
}
