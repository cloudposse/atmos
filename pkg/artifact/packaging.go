package artifact

// Packaging identifies how a file payload is stored in a repository.
type Packaging string

// Payload packaging modes.
const (
	// PackagingNone stores copied files (local directories, S3 object-per-file).
	PackagingNone Packaging = "none"

	// PackagingTar stores one uncompressed tar archive.
	PackagingTar Packaging = "tar"

	// PackagingTgz stores one gzip-compressed tar archive.
	PackagingTgz Packaging = "tgz"

	// PackagingZip stores one zip archive.
	PackagingZip Packaging = "zip"

	// PackagingCatalog stores one catalog JSON document as the direct payload layer. Only Records
	// of kind "catalog" use it.
	PackagingCatalog Packaging = "catalog"

	// PackagingOCIGraph identifies an existing native OCI graph carried unchanged.
	PackagingOCIGraph Packaging = "oci-graph"
)
