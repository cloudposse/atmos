package manager

import "github.com/cloudposse/atmos/pkg/perf"

// Pin policy values (after normalization).
const (
	// PinNone emits plain version references.
	PinNone = "none"
	// PinDigest emits the immutable identifier (git commit SHA or OCI digest).
	PinDigest = "digest"
	// Configuration alias for PinDigest.
	pinSHAAlias = "sha"
)

// VersionRef is the template-context value for one managed version, exposing
// both the human-readable version and the locked immutable digest.
type VersionRef struct {
	Version string `yaml:"version" json:"version"`
	Digest  string `yaml:"digest,omitempty" json:"digest,omitempty"`
	Pin     string `yaml:"pin,omitempty" json:"pin,omitempty"`
}

// String returns the reference form to embed in rendered output: the digest
// when pinning is enabled and a digest is locked, otherwise the version. Go
// templates call this automatically, so `{{ .version.name }}` yields the
// pinned form for pinned entries.
func (r VersionRef) String() string {
	defer perf.Track(nil, "manager.VersionRef.String")()

	if r.Pin == PinDigest && r.Digest != "" {
		return r.Digest
	}
	return r.Version
}

// normalizePin canonicalizes a configured pin value ("sha" is an alias for
// "digest"; empty means none).
func normalizePin(pin string) string {
	switch pin {
	case PinDigest, pinSHAAlias:
		return PinDigest
	default:
		return PinNone
	}
}

// pinEnabled reports whether an entry's effective policy requests digest pinning.
func pinEnabled(entry *EffectiveEntry) bool {
	return normalizePin(entry.Update.Pin) == PinDigest
}

// VersionRefs joins effective entries with their lock records into the
// template-context/file-manager reference form.
func VersionRefs(entries map[string]EffectiveEntry, lock map[string]LockEntry) map[string]VersionRef {
	defer perf.Track(nil, "manager.VersionRefs")()

	refs := make(map[string]VersionRef, len(lock))
	for name := range lock {
		ref := VersionRef{Version: lock[name].Version, Digest: lock[name].Digest}
		if entry, ok := entries[name]; ok {
			ref.Pin = normalizePin(entry.Update.Pin)
		}
		refs[name] = ref
	}
	return refs
}
