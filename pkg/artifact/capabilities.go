package artifact

import (
	"slices"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DeleteSemantics describes what deleting a reference removes.
type DeleteSemantics string

// Delete semantics a repository can advertise.
const (
	// DeleteNone means the repository cannot delete.
	DeleteNone DeleteSemantics = "none"

	// DeleteReference removes only the named reference (tag-safe).
	DeleteReference DeleteSemantics = "reference"

	// DeleteManifest removes a manifest by digest, affecting every reference to it.
	DeleteManifest DeleteSemantics = "manifest"

	// DeleteObject removes the stored record and payload objects.
	DeleteObject DeleteSemantics = "object"
)

// Capabilities describes what a repository can do. It is computed without network access.
type Capabilities struct {
	// Read reports that Resolve, Inspect, OpenDocument, and Read are supported.
	Read bool `json:"read"`

	// Write reports that Publish is supported.
	Write bool `json:"write"`

	// EnumerateNamed reports that listing versions of a known name is supported.
	EnumerateNamed bool `json:"enumerate_named"`

	// EnumerateCollection reports that listing every name is supported (catalog or native).
	EnumerateCollection bool `json:"enumerate_collection"`

	// Delete describes delete semantics.
	Delete DeleteSemantics `json:"delete"`

	// ConditionalUpdate reports conditional (compare-and-swap) reference publication.
	ConditionalUpdate bool `json:"conditional_update"`

	// Packagings lists supported file payload packaging modes.
	Packagings []Packaging `json:"packagings,omitempty"`

	// PreservesModes reports that ordinary permission bits survive a round trip.
	PreservesModes bool `json:"preserves_modes"`

	// SubjectKinds lists the immutable signature subjects the repository can bind evidence to.
	SubjectKinds []SubjectKind `json:"subject_kinds,omitempty"`

	// EvidenceFormats lists the signature evidence formats the repository can store and verify.
	EvidenceFormats []EvidenceFormat `json:"evidence_formats,omitempty"`

	// LockScopes lists the mutation lock scopes the repository can provide.
	LockScopes []LockScope `json:"lock_scopes,omitempty"`
}

// HasPackaging reports whether the repository supports the packaging mode.
func (c *Capabilities) HasPackaging(p Packaging) bool {
	defer perf.Track(nil, "artifact.Capabilities.HasPackaging")()

	return slices.Contains(c.Packagings, p)
}

// HasEvidenceFormat reports whether the repository supports the evidence format.
func (c *Capabilities) HasEvidenceFormat(f EvidenceFormat) bool {
	defer perf.Track(nil, "artifact.Capabilities.HasEvidenceFormat")()

	return slices.Contains(c.EvidenceFormats, f)
}

// HasSubjectKind reports whether the repository supports the signature subject kind.
func (c *Capabilities) HasSubjectKind(k SubjectKind) bool {
	defer perf.Track(nil, "artifact.Capabilities.HasSubjectKind")()

	return slices.Contains(c.SubjectKinds, k)
}

// HasLockScope reports whether the repository can provide at least the requested lock scope.
// A repository-scope lock satisfies a host-scope request.
func (c *Capabilities) HasLockScope(s LockScope) bool {
	defer perf.Track(nil, "artifact.Capabilities.HasLockScope")()

	for _, have := range c.LockScopes {
		if have.Satisfies(s) {
			return true
		}
	}
	return false
}
