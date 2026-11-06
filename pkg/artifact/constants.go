package artifact

// Serialization and bounded-read limits shared by every repository adapter.
const (
	// SchemaVersion is the only Record schema version defined by V1.
	SchemaVersion = 1

	// MaxRecordSize is the largest canonical Record (and catalog JSON) read or written, in bytes.
	MaxRecordSize int64 = 8 << 20

	// MaxDocumentSize is the largest README or CHANGELOG document, in bytes.
	MaxDocumentSize int64 = 4 << 20

	// MaxEvidenceSize is the largest single signature evidence blob read into memory, in bytes.
	MaxEvidenceSize int64 = 1 << 20

	// MaxNameLength is the longest artifact name, matching the OCI repository name limit.
	MaxNameLength = 255

	// MaxVersionLength is the longest artifact version, matching the OCI tag limit.
	MaxVersionLength = 128

	// MaxKindLength is the longest artifact kind.
	MaxKindLength = 128

	// DefaultConcurrency is the default transfer and discovery concurrency.
	DefaultConcurrency = 4

	// ReservedNamespace is the managed-record directory below a local or S3 root. Payload
	// names and key patterns must never enter it.
	ReservedNamespace = ".atmos-artifacts"

	// DocumentMediaType is the default media type of README and CHANGELOG documents.
	DocumentMediaType = "text/markdown"
)

// OCI media types from the artifact PRD. Existing plugin and foreign OCI graphs keep their own types.
const (
	// MediaTypeArtifact is the generic artifact type.
	MediaTypeArtifact = "application/vnd.atmos.artifact.v1"

	// MediaTypeScaffold is the scaffold artifact type.
	MediaTypeScaffold = "application/vnd.atmos.scaffold.v1"

	// MediaTypeComponent is the component artifact type.
	MediaTypeComponent = "application/vnd.atmos.component.v1"

	// MediaTypeToolchainPackage is the toolchain package artifact type.
	MediaTypeToolchainPackage = "application/vnd.atmos.toolchain.package.v1"

	// MediaTypeCatalog is the artifact type of a catalog Record.
	MediaTypeCatalog = "application/vnd.atmos.artifact.catalog.v1"

	// MediaTypeCatalogPayload is the media type of a catalog's direct JSON payload layer.
	MediaTypeCatalogPayload = "application/vnd.atmos.artifact.catalog.v1+json"

	// MediaTypeConfig is the media type of the OCI config blob holding the canonical Record bytes.
	MediaTypeConfig = "application/vnd.atmos.artifact.config.v1+json"

	// MediaTypeTar is the media type of an uncompressed tar payload.
	MediaTypeTar = "application/vnd.atmos.artifact.content.v1.tar"

	// MediaTypeTarGzip is the media type of a gzip-compressed tar payload.
	MediaTypeTarGzip = "application/vnd.atmos.artifact.content.v1.tar+gzip"

	// MediaTypeZip is the media type of a zip payload.
	MediaTypeZip = "application/vnd.atmos.artifact.content.v1.zip"
)

// Well-known artifact kinds.
const (
	// KindFiles is the default kind for generic file packages.
	KindFiles = "files"

	// KindCatalog is the kind of an artifact catalog Record.
	KindCatalog = "catalog"
)
