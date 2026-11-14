package artifact

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	// Name segments use the OCI distribution path-component grammar. It is lowercase-only so
	// that a name valid here is valid on every backend without case folding or other rewriting.
	nameSegmentPattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)

	// Versions use the OCI tag grammar. Versions are opaque labels, not required to be SemVer.
	versionPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

	// Kind segments are lowercase tokens, for example "component" in "component/terraform".
	kindSegmentPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
)

// Ref addresses an artifact by name and exactly one of version or digest. The repository alias is
// a separate request field and never part of a Ref.
//
// The textual forms are "name:version" and "name@sha256:<hex>". There is no implicit "latest".
type Ref struct {
	// Name is the slash-separated relative artifact name, e.g. "components/terraform/vpc".
	Name string `json:"name"`

	// Version is the explicit version label. Mutually exclusive with Digest.
	Version string `json:"version,omitempty"`

	// Digest is an immutable record or manifest digest. Mutually exclusive with Version.
	Digest digest.Digest `json:"digest,omitempty"`
}

// ParseRef parses "name:version" or "name@sha256:<hex>" and validates the result.
func ParseRef(s string) (Ref, error) {
	defer perf.Track(nil, "artifact.ParseRef")()

	if s == "" {
		return Ref{}, fmt.Errorf("%w: reference is empty", errUtils.ErrArtifactRefInvalid)
	}
	var ref Ref
	if name, dgst, found := strings.Cut(s, "@"); found {
		ref = Ref{Name: name, Digest: digest.Digest(dgst)}
	} else {
		index := strings.LastIndex(s, ":")
		if index < 0 {
			return Ref{}, fmt.Errorf("%w: %q needs a version (name:version) or digest (name@sha256:...); there is no implicit latest", errUtils.ErrArtifactRefInvalid, s)
		}
		ref = Ref{Name: s[:index], Version: s[index+1:]}
	}
	if err := ref.Validate(); err != nil {
		return Ref{}, err
	}
	return ref, nil
}

// Validate checks the name and that exactly one of version or digest is set and well formed.
func (r Ref) Validate() error {
	defer perf.Track(nil, "artifact.Ref.Validate")()

	if err := ValidateName(r.Name); err != nil {
		return err
	}
	switch {
	case r.Version != "" && r.Digest != "":
		return fmt.Errorf("%w: %q sets both a version and a digest", errUtils.ErrArtifactRefInvalid, r.Name)
	case r.Version == "" && r.Digest == "":
		return fmt.Errorf("%w: %q needs a version or a digest", errUtils.ErrArtifactRefInvalid, r.Name)
	case r.Version != "":
		return ValidateVersion(r.Version)
	default:
		return ValidateDigest(r.Digest)
	}
}

// String returns the textual form of the reference.
func (r Ref) String() string {
	defer perf.Track(nil, "artifact.Ref.String")()

	if r.Digest != "" {
		return r.Name + "@" + r.Digest.String()
	}
	return r.Name + ":" + r.Version
}

// IsDigest reports whether the reference selects an immutable digest.
//
//nolint:lintroller // Trivial getter with no I/O.
func (r Ref) IsDigest() bool { return r.Digest != "" }

// ValidateName checks an artifact name: relative, slash-separated, with no empty, "." or ".."
// segments and each segment matching the OCI path-component grammar. Names are never rewritten.
func ValidateName(name string) error {
	defer perf.Track(nil, "artifact.ValidateName")()

	if name == "" {
		return fmt.Errorf("%w: name is empty", errUtils.ErrArtifactRefInvalid)
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("%w: name exceeds %d characters", errUtils.ErrArtifactRefInvalid, MaxNameLength)
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("%w: %w: %q must be relative and slash-separated", errUtils.ErrArtifactRefInvalid, errUtils.ErrArtifactUnsafePath, name)
	}
	for _, segment := range strings.Split(name, "/") {
		switch segment {
		case "":
			return fmt.Errorf("%w: %w: %q has an empty segment", errUtils.ErrArtifactRefInvalid, errUtils.ErrArtifactUnsafePath, name)
		case ".", "..":
			return fmt.Errorf("%w: %w: %q has a %q segment", errUtils.ErrArtifactRefInvalid, errUtils.ErrArtifactUnsafePath, name, segment)
		}
		if !nameSegmentPattern.MatchString(segment) {
			return fmt.Errorf("%w: segment %q of %q must be lowercase alphanumerics separated by '.', '_', '__', or '-'", errUtils.ErrArtifactRefInvalid, segment, name)
		}
	}
	return nil
}

// ValidateVersion checks a version against the OCI tag grammar. Versions are never normalized.
func ValidateVersion(version string) error {
	defer perf.Track(nil, "artifact.ValidateVersion")()

	if version == "" {
		return fmt.Errorf("%w: version is empty", errUtils.ErrArtifactRefInvalid)
	}
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("%w: version %q must match the OCI tag syntax [A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", errUtils.ErrArtifactRefInvalid, version)
	}
	return nil
}

// ValidateDigest checks that d is a well-formed sha256 digest. V1 supports sha256 only.
func ValidateDigest(d digest.Digest) error {
	defer perf.Track(nil, "artifact.ValidateDigest")()

	if err := d.Validate(); err != nil {
		return fmt.Errorf("%w: digest %q: %w", errUtils.ErrArtifactRefInvalid, d, err)
	}
	if d.Algorithm() != digest.SHA256 {
		return fmt.Errorf("%w: digest %q must use sha256", errUtils.ErrArtifactRefInvalid, d)
	}
	return nil
}

// ValidateKind checks an artifact kind such as "files", "component/terraform", or "catalog".
func ValidateKind(kind string) error {
	defer perf.Track(nil, "artifact.ValidateKind")()

	if kind == "" || len(kind) > MaxKindLength {
		return fmt.Errorf("%w: kind must be 1-%d characters", errUtils.ErrArtifactRecordInvalid, MaxKindLength)
	}
	for _, segment := range strings.Split(kind, "/") {
		if !kindSegmentPattern.MatchString(segment) {
			return fmt.Errorf("%w: kind %q must be slash-separated lowercase segments", errUtils.ErrArtifactRecordInvalid, kind)
		}
	}
	return nil
}
