package install

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// versionResolveTagsTimeout bounds a single remote tag listing, matching
// pkg/vendoring/update.go's listTagsTimeout for the same operation used by `atmos vendor update`.
const versionResolveTagsTimeout = 30 * time.Second

// versionResolveKind is the vendor.lock.yaml "kind" reserved exclusively for version-range
// resolution receipts (see VersionResolveParams/ResolveDeclaredVersion's doc comment). It is
// deliberately disjoint from PkgType.String()'s "remote"/"oci"/"local" values used by real
// installed-files materialization receipts, so the two kinds of entries can never collide even
// though they live in the same lock file.
const versionResolveKind = "version-range"

// versionResolveTarget is the synthetic, non-filesystem "target" recorded on every version-range
// resolution receipt. It is never used for real file operations (these receipts never carry
// Files), but lockfile.Save/Replace still validate every artifact's Target as a relative,
// non-escaping path, so a stable, human-recognizable placeholder is used instead of a real path.
const versionResolveTarget = ".atmos/vendor-version-resolutions"

// VersionResolveParams configures ResolveDeclaredVersion.
type VersionResolveParams struct {
	// RawVersion is the source's (or per-target override's) declared version string, exactly as
	// written in vendor.yaml/component.yaml -- an exact pin, a semver range, or an opaque literal
	// (commit SHA, branch name).
	RawVersion string
	// Name identifies the declaring source/target for the lock cache key -- typically the
	// component name (or the raw source URI, when a source declares no component name).
	Name string
	// Discriminator, when non-empty, is folded into the lock cache key alongside Name. Needed when
	// multiple declarations share one Name but must resolve/cache independently -- e.g. a
	// vendor.yaml source's per-target `targets[].version` overrides, which all share the source's
	// Component name.
	Discriminator string
	// SourceForGitURI is the source's raw, un-templated URI/source string (before {{.Version}} is
	// substituted). Used both to extract the Git remote to list tags from, and as part of the lock
	// cache key -- it is stable across pulls regardless of how the range resolves, unlike an
	// installed target path, which may itself template {{.Version}} and therefore isn't knowable
	// until after this resolution completes.
	SourceForGitURI string
	// Constraints are the source's separate `constraints:` block. Only ExcludedVersions/
	// NoPrereleases are applied here -- Constraints.Version is an unrelated concept (the ceiling
	// `atmos vendor update`'s bump search may not exceed) and is never consulted by this function.
	Constraints *schema.VendorConstraints
	// RefreshLock forces fresh resolution even when a matching lock entry exists.
	RefreshLock bool
	// Lister lists remote Git tags; defaults to version.DefaultLister when nil.
	Lister version.RemoteLister
}

// ResolveDeclaredVersion resolves a declared `version:` value to the concrete version that should
// be templated into a source URI (and, indirectly, any target path that itself templates
// {{.Version}}).
//
// Fast path: when RawVersion is not a semver range (version.IsSemverConstraint), it is returned
// unchanged with no lock or network access whatsoever -- this is the overwhelmingly common case
// (an exact tag/commit/branch pin) and must stay free.
//
// Range path: the first resolution for a given (Name, Discriminator, SourceForGitURI) triple lists
// the source's remote Git tags and applies the same constraint-filtering pipeline
// version.ResolveVersionConstraints already provides for `atmos vendor update`'s bump search
// (RawVersion as the primary constraint, Constraints.ExcludedVersions/NoPrereleases layered on
// top). The result is recorded in vendor.lock.yaml under a dedicated "version-range" lock entry --
// see versionResolveKind's doc comment for why this is a receipt distinct from the source's real
// installed-files materialization receipt, rather than the exact same entry: an installed target
// path may itself contain a {{.Version}} placeholder, so it isn't knowable until after this
// resolution completes, and can't double as the pre-resolution lookup key. Every subsequent pull
// with the same declared range and a matching lock entry reuses the recorded resolution with zero
// network calls, until RefreshLock is set or the declared range string itself changes.
//
// Ranges are only supported for Git sources (the only source type with a tag-listing mechanism in
// this codebase); a range declared on an OCI, local-file, or plain HTTP/S3 source returns
// ErrVersionRangeRequiresGitSource.
func ResolveDeclaredVersion(ctx context.Context, atmosConfig *schema.AtmosConfiguration, params *VersionResolveParams) (string, error) {
	defer perf.Track(atmosConfig, "install.ResolveDeclaredVersion")()

	if !version.IsSemverConstraint(params.RawVersion) {
		return params.RawVersion, nil
	}

	id, err := versionResolveArtifactID(atmosConfig, params.Name, params.Discriminator, version.ExtractGitURI(params.SourceForGitURI))
	if err != nil {
		return "", err
	}

	if !params.RefreshLock {
		resolved, found, err := lookupResolvedVersion(atmosConfig, id, params.RawVersion)
		if err != nil {
			return "", err
		}
		if found {
			return resolved, nil
		}
	}

	resolved, err := resolveVersionRangeFresh(ctx, params)
	if err != nil {
		return "", err
	}

	if err := recordResolvedVersion(atmosConfig, id, params.Name, params.RawVersion, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// resolveVersionRangeFresh lists remote Git tags and applies RawVersion's constraint-filtering
// pipeline, with no lock involvement -- the "cache miss or RefreshLock" branch of
// ResolveDeclaredVersion, split out to keep that function's cyclomatic complexity within this
// repo's limit.
func resolveVersionRangeFresh(ctx context.Context, params *VersionResolveParams) (string, error) {
	if !version.IsGitSource(params.SourceForGitURI) {
		return "", fmt.Errorf("%w: %q", ErrVersionRangeRequiresGitSource, params.RawVersion)
	}

	lister := params.Lister
	if lister == nil {
		lister = version.DefaultLister
	}

	resolveCtx, cancel := context.WithTimeout(ctx, versionResolveTagsTimeout)
	defer cancel()

	gitURI := version.ExtractGitURI(params.SourceForGitURI)
	tags, err := lister.ListTags(resolveCtx, gitURI)
	if err != nil {
		return "", err
	}

	constraints := &schema.VendorConstraints{Version: params.RawVersion}
	if params.Constraints != nil {
		constraints.ExcludedVersions = params.Constraints.ExcludedVersions
		constraints.NoPrereleases = params.Constraints.NoPrereleases
	}

	return version.ResolveVersionConstraints(tags, constraints)
}

// versionResolveArtifactID reuses lockfile.ArtifactID for the version-resolution lock cache key
// (versionResolveKind/versionResolveTarget as the fixed kind/target, name+discriminator+gitURI as
// the writer list), matching this package's other artifact-key derivations.
func versionResolveArtifactID(atmosConfig *schema.AtmosConfiguration, name, discriminator, gitURI string) (string, error) {
	writers := []string{name}
	if discriminator != "" {
		writers = append(writers, discriminator)
	}
	writers = append(writers, gitURI)
	return lockfile.ArtifactID(atmosConfig, versionResolveKind, versionResolveTarget, writers...)
}

// lookupResolvedVersion returns the previously recorded resolution for id, when one exists and was
// recorded for the exact same declared range. A lock file that doesn't exist yet (lockfile.Load's
// documented "no lock entry" case, not an error) or an entry recorded for a since-changed range
// both report found=false, sending the caller down the fresh-resolution path.
func lookupResolvedVersion(atmosConfig *schema.AtmosConfiguration, id, rawVersion string) (string, bool, error) {
	lock, err := lockfile.Load(atmosConfig)
	if err != nil {
		return "", false, err
	}
	artifact, found := lock.Artifacts[id]
	if !found {
		return "", false, nil
	}
	if artifact.Source.VersionConstraint != rawVersion || artifact.Source.ResolvedVersion == "" {
		return "", false, nil
	}
	return artifact.Source.ResolvedVersion, true, nil
}

// recordResolvedVersion persists a fresh version-range resolution as its own lock artifact (see
// versionResolveKind's doc comment for why it's a dedicated entry rather than the source's eventual
// installed-files receipt). Uses lockfile.Replace directly (Order bookkeeping and atomic Save),
// same as every other lock writer in this package.
func recordResolvedVersion(atmosConfig *schema.AtmosConfiguration, id, name, rawVersion, resolvedVersion string) error {
	artifact := lockfile.Artifact{
		Name:   name,
		Kind:   versionResolveKind,
		Target: versionResolveTarget,
		Source: lockfile.Source{
			Declared:          rawVersion,
			VersionConstraint: rawVersion,
			ResolvedVersion:   resolvedVersion,
		},
	}
	return lockfile.Replace(atmosConfig, id, artifact)
}
