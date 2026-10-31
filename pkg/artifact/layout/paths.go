package layout

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Names of the managed layout.
const (
	// SchemaVersion is the schema version of every layout document.
	SchemaVersion = 1

	// Version is the layout version directory below the reserved namespace.
	Version = "v1"

	// ManagedRoot is the slash-separated managed directory below a repository root.
	ManagedRoot = artifact.ReservedNamespace + "/" + Version

	// MetadataSuffix is appended to an archive payload path for its adjacent metadata sidecar.
	MetadataSuffix = ".metadata.json"

	// DocsSuffix is appended to an archive payload path for its adjacent documents directory.
	DocsSuffix = ".docs"

	// DistributionDir is the distribution directory below a copied-files payload, and inside an
	// archive, that holds README, CHANGELOG, and manifest.json. Managed readers omit it when
	// materializing user content.
	DistributionDir = ".atmos/artifact"

	// ManifestName is the generated manifest file name inside the distribution directory.
	ManifestName = "manifest.json"
)

// Directory names inside the managed root.
const (
	refsDir            = "refs"
	recordsDir         = "records"
	representationsDir = "representations"
	operationsDir      = "operations"
	signaturesDir      = "signatures"
	locksDir           = "locks"
	stagingDirName     = "staging"
	jsonSuffix         = ".json"
	lockSuffix         = ".lock"

	// The longest operation identifier used verbatim as a file name.
	maxSafeOperationID = 128

	// The prefix of a file name derived from an unsafe operation identifier.
	hashedOperationPrefix = "sha256-"
)

// safeOperationID matches identifiers that are safe path segments on every platform.
var safeOperationID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ReferenceKey returns the private reference key for a name and version: the lowercase hex
// SHA-256 of "name:version". The colon is unambiguous because names never contain one.
func ReferenceKey(name, version string) string {
	defer perf.Track(nil, "layout.ReferenceKey")()

	sum := sha256.Sum256([]byte(name + ":" + version))
	return hex.EncodeToString(sum[:])
}

// ReferencePath returns the slash-separated path of the reference record for key.
func ReferencePath(key string) string {
	defer perf.Track(nil, "layout.ReferencePath")()

	return ManagedRoot + "/" + refsDir + "/" + key + jsonSuffix
}

// ReferencesDir returns the directory that holds every reference record.
func ReferencesDir() string {
	defer perf.Track(nil, "layout.ReferencesDir")()

	return ManagedRoot + "/" + refsDir
}

// RecordPath returns the slash-separated path of the immutable Record with the given digest.
func RecordPath(recordDigest digest.Digest) string {
	defer perf.Track(nil, "layout.RecordPath")()

	return ManagedRoot + "/" + recordsDir + "/" + recordDigest.Encoded() + jsonSuffix
}

// RepresentationPath returns the slash-separated path of the representation record for key.
func RepresentationPath(key string) string {
	defer perf.Track(nil, "layout.RepresentationPath")()

	return ManagedRoot + "/" + representationsDir + "/" + key + jsonSuffix
}

// OperationFileName returns the file stem for an operation identifier. Safe identifiers are used
// verbatim; anything else (slashes, spaces, overlong values) maps to a hash so that no identifier
// can address a path outside the operations directory.
func OperationFileName(operationID string) string {
	defer perf.Track(nil, "layout.OperationFileName")()

	if len(operationID) <= maxSafeOperationID && safeOperationID.MatchString(operationID) {
		return operationID
	}
	sum := sha256.Sum256([]byte(operationID))
	return hashedOperationPrefix + hex.EncodeToString(sum[:])
}

// OperationPath returns the slash-separated path of the journal for an operation.
func OperationPath(operationID string) string {
	defer perf.Track(nil, "layout.OperationPath")()

	return ManagedRoot + "/" + operationsDir + "/" + OperationFileName(operationID) + jsonSuffix
}

// StagingPath returns the slash-separated staging directory owned by an operation. Staging lives
// below the managed root so it shares a filesystem with the payload for atomic renames.
func StagingPath(operationID string) string {
	defer perf.Track(nil, "layout.StagingPath")()

	return ManagedRoot + "/" + stagingDirName + "/" + OperationFileName(operationID)
}

// TempDir returns the directory for short-lived temporary files used by atomic replacement.
func TempDir() string {
	defer perf.Track(nil, "layout.TempDir")()

	return ManagedRoot + "/tmp"
}

// LockPath returns the slash-separated path of the lock file for a resource. The file name is the
// hash of the resource so any resource string is safe.
func LockPath(resource string) string {
	defer perf.Track(nil, "layout.LockPath")()

	sum := sha256.Sum256([]byte(resource))
	return ManagedRoot + "/" + locksDir + "/" + hex.EncodeToString(sum[:]) + lockSuffix
}

// SignatureDir returns the directory that holds the detached evidence for a subject.
func SignatureDir(kind artifact.SubjectKind, subject digest.Digest) string {
	defer perf.Track(nil, "layout.SignatureDir")()

	return ManagedRoot + "/" + signaturesDir + "/" + string(kind) + "/" + subject.Encoded()
}

// SignaturePath returns the slash-separated path of one detached evidence file:
// .atmos-artifacts/v1/signatures/<subject-kind>/<subject-digest>/<evidence-digest>.json, with
// the digests written as lowercase hex.
func SignaturePath(kind artifact.SubjectKind, subject, evidence digest.Digest) string {
	defer perf.Track(nil, "layout.SignaturePath")()

	return SignatureDir(kind, subject) + "/" + evidence.Encoded() + jsonSuffix
}

// MetadataSidecarPath returns the adjacent metadata sidecar of an archive payload location.
func MetadataSidecarPath(location string) string {
	defer perf.Track(nil, "layout.MetadataSidecarPath")()

	return location + MetadataSuffix
}

// DocsSidecarDir returns the adjacent documents directory of an archive payload location.
func DocsSidecarDir(location string) string {
	defer perf.Track(nil, "layout.DocsSidecarDir")()

	return location + DocsSuffix
}

// DocumentFileName returns the file name of a distribution document.
func DocumentFileName(role artifact.DocumentRole) string {
	defer perf.Track(nil, "layout.DocumentFileName")()

	if role == artifact.DocumentChangelog {
		return "CHANGELOG.md"
	}
	return "README.md"
}

// DocumentSidecarPath returns the adjacent document file of an archive payload location.
func DocumentSidecarPath(location string, role artifact.DocumentRole) string {
	defer perf.Track(nil, "layout.DocumentSidecarPath")()

	return DocsSidecarDir(location) + "/" + DocumentFileName(role)
}

// DistributionPath returns the distribution file of a copied-files payload location.
func DistributionPath(location, name string) string {
	defer perf.Track(nil, "layout.DistributionPath")()

	return location + "/" + DistributionDir + "/" + name
}

// InManagedNamespace reports whether a slash-separated path enters the reserved namespace. The
// comparison folds case because common filesystems alias names that differ only by case.
func InManagedNamespace(p string) bool {
	defer perf.Track(nil, "layout.InManagedNamespace")()

	first, _, _ := strings.Cut(strings.TrimPrefix(p, "./"), "/")
	return strings.EqualFold(first, artifact.ReservedNamespace)
}
