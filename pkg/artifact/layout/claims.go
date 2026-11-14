package layout

import (
	"path"
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Claim is the set of storage paths one reference owns. Two references may never own overlapping
// paths, which is how two references with a colliding key_pattern are detected: the second fails
// with a conflict and is never bound to the first reference's payload.
type Claim struct {
	// ReferenceKey identifies the owning reference.
	ReferenceKey string

	// Name and Version identify the owner for diagnostics.
	Name    string
	Version string

	// OperationID is the pending operation holding the claim, empty for a completed reference.
	OperationID string

	// Paths are the owned slash-separated paths relative to the repository root.
	Paths []string
}

// PayloadPaths returns the paths a payload at location owns for the repository kind: a copied
// directory owns the directory; an archive owns the file, its metadata sidecar, and its
// documents directory.
func PayloadPaths(repositoryKind, location string) []string {
	defer perf.Track(nil, "layout.PayloadPaths")()

	if repositoryKind == artifact.RepositoryKindLocalArchive {
		return []string{location, MetadataSidecarPath(location), DocsSidecarDir(location)}
	}
	return []string{location}
}

// FoldPath returns the comparison form of a slash-separated path: cleaned and case folded, because
// common filesystems alias names that differ only by case.
func FoldPath(p string) string {
	defer perf.Track(nil, "layout.FoldPath")()

	return strings.ToLower(path.Clean(p))
}

// PathsOverlap reports whether two paths are equal or one contains the other.
func PathsOverlap(a, b string) bool {
	defer perf.Track(nil, "layout.PathsOverlap")()

	a, b = FoldPath(a), FoldPath(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// Overlaps reports whether any path of c overlaps any path of other.
func (c *Claim) Overlaps(other *Claim) bool {
	defer perf.Track(nil, "layout.Claim.Overlaps")()

	for _, mine := range c.Paths {
		for _, theirs := range other.Paths {
			if PathsOverlap(mine, theirs) {
				return true
			}
		}
	}
	return false
}

// FindConflict returns the first existing claim, owned by a different reference, that overlaps the
// candidate, or nil. A reference never conflicts with itself, so replacement and same-intent
// retry keep working.
func FindConflict(candidate *Claim, existing []Claim) *Claim {
	defer perf.Track(nil, "layout.FindConflict")()

	for index := range existing {
		if existing[index].ReferenceKey == candidate.ReferenceKey {
			continue
		}
		if candidate.Overlaps(&existing[index]) {
			return &existing[index]
		}
	}
	return nil
}
