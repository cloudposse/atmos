package install

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

// countingLister is a version.RemoteLister that records how many times ListTags was called and
// returns canned tags, letting tests assert zero-network reuse precisely rather than just "no
// error occurred".
type countingLister struct {
	tags  []string
	err   error
	calls int
}

func (l *countingLister) ListTags(context.Context, string) ([]string, error) {
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return l.tags, nil
}

// panicLister fails the test immediately if ListTags is ever invoked -- used to prove the
// exact-pin fast path never touches the network.
type panicLister struct{ t *testing.T }

func (l *panicLister) ListTags(context.Context, string) ([]string, error) {
	l.t.Fatal("ListTags must not be called for an exact-pinned version")
	return nil, nil
}

func newVersionResolveConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	return &schema.AtmosConfiguration{BasePath: t.TempDir()}
}

// TestResolveDeclaredVersion_ExactPinFastPath proves an exact pin (not a semver range) is returned
// unchanged with zero lock or network access: no lock file is ever created, and a Lister that fails
// the test if invoked is never called.
func TestResolveDeclaredVersion_ExactPinFastPath(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)

	for _, raw := range []string{"v1.2.3", "1.2.3", "abc1234", "main", ""} {
		t.Run(raw, func(t *testing.T) {
			resolved, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
				RawVersion:      raw,
				Name:            "vpc",
				SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
				Lister:          &panicLister{t: t},
			})
			require.NoError(t, err)
			assert.Equal(t, raw, resolved)
		})
	}

	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, lock.Artifacts, "exact-pin resolution must never write a lock entry")
}

// TestResolveDeclaredVersion_RangeFirstResolutionRecordsLock proves the first resolution of a
// semver range lists remote tags exactly once and records both Source.VersionConstraint and
// Source.ResolvedVersion in vendor.lock.yaml.
func TestResolveDeclaredVersion_RangeFirstResolutionRecordsLock(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)
	lister := &countingLister{tags: []string{"v1.0.0", "v1.2.3", "v1.5.0", "v2.0.0"}}

	resolved, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
		RawVersion:      "^1.0.0",
		Name:            "vpc",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Lister:          lister,
	})

	require.NoError(t, err)
	assert.Equal(t, "v1.5.0", resolved)
	assert.Equal(t, 1, lister.calls)

	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	require.Len(t, lock.Artifacts, 1)
	for _, artifact := range lock.Artifacts {
		assert.Equal(t, "^1.0.0", artifact.Source.VersionConstraint)
		assert.Equal(t, "v1.5.0", artifact.Source.ResolvedVersion)
	}
}

// TestResolveDeclaredVersion_RangeSecondPullReusesLockWithZeroNetworkCalls proves a second
// resolution for the same declared range reuses the recorded resolution instead of listing tags
// again -- the "no network dependency on subsequent pulls" guarantee.
func TestResolveDeclaredVersion_RangeSecondPullReusesLockWithZeroNetworkCalls(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)
	lister := &countingLister{tags: []string{"v1.0.0", "v1.2.3", "v1.5.0", "v2.0.0"}}

	params := VersionResolveParams{
		RawVersion:      "^1.0.0",
		Name:            "vpc",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Lister:          lister,
	}

	first, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &params)
	require.NoError(t, err)
	require.Equal(t, 1, lister.calls)

	second, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &params)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, 1, lister.calls, "second pull with an unchanged range must not call ListTags again")
}

// TestResolveDeclaredVersion_RefreshLockReResolves proves --refresh-lock (params.RefreshLock)
// bypasses a matching lock entry and lists tags again, even though the declared range is unchanged.
func TestResolveDeclaredVersion_RefreshLockReResolves(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)
	lister := &countingLister{tags: []string{"v1.0.0", "v1.2.3", "v1.5.0"}}

	base := VersionResolveParams{
		RawVersion:      "^1.0.0",
		Name:            "vpc",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Lister:          lister,
	}

	_, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &base)
	require.NoError(t, err)
	require.Equal(t, 1, lister.calls)

	refreshParams := base
	refreshParams.RefreshLock = true
	// New upstream tag published between the two calls -- refresh must be able to observe it.
	lister.tags = append(lister.tags, "v1.9.0")

	resolved, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &refreshParams)
	require.NoError(t, err)
	assert.Equal(t, "v1.9.0", resolved)
	assert.Equal(t, 2, lister.calls, "--refresh-lock must re-resolve even with a matching lock entry")
}

// TestResolveDeclaredVersion_ChangedRangeReResolves proves that declaring a different range for
// the same component invalidates the previous resolution automatically (no --refresh-lock needed).
func TestResolveDeclaredVersion_ChangedRangeReResolves(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)
	lister := &countingLister{tags: []string{"v1.0.0", "v1.5.0", "v2.0.0", "v2.5.0"}}

	first, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
		RawVersion:      "^1.0.0",
		Name:            "vpc",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Lister:          lister,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.5.0", first)
	require.Equal(t, 1, lister.calls)

	second, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
		RawVersion:      "^2.0.0",
		Name:            "vpc",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Lister:          lister,
	})
	require.NoError(t, err)
	assert.Equal(t, "v2.5.0", second)
	assert.Equal(t, 2, lister.calls, "a changed declared range must trigger fresh resolution")
}

// TestResolveDeclaredVersion_NonGitSourceReturnsClearError proves a range declared on a source with
// no tag-listing mechanism (OCI here) fails with ErrVersionRangeRequiresGitSource instead of
// panicking or silently mis-templating the raw range string into a URI.
func TestResolveDeclaredVersion_NonGitSourceReturnsClearError(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)

	_, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
		RawVersion:      "^1.0.0",
		Name:            "vpc",
		SourceForGitURI: "oci://ghcr.io/cloudposse/vpc",
		Lister:          &panicLister{t: t},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVersionRangeRequiresGitSource)
}

// TestResolveDeclaredVersion_ConstraintsExcludedVersionsAndNoPrereleases proves Constraints'
// ExcludedVersions/NoPrereleases are layered on top of RawVersion's range, exactly as
// version.ResolveVersionConstraints already does for `atmos vendor update`'s bump search.
func TestResolveDeclaredVersion_ConstraintsExcludedVersionsAndNoPrereleases(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)
	lister := &countingLister{tags: []string{"v1.0.0", "v1.5.0", "v1.9.0", "v2.0.0-rc.1"}}

	resolved, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
		RawVersion:      ">=1.0.0 <3.0.0",
		Name:            "vpc",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Constraints: &schema.VendorConstraints{
			ExcludedVersions: []string{"v1.9.0"},
			NoPrereleases:    true,
		},
		Lister: lister,
	})

	require.NoError(t, err)
	assert.Equal(t, "v1.5.0", resolved)
}

// TestResolveDeclaredVersion_DiscriminatorDisambiguatesSharedNames proves two declarations sharing
// the same Name (e.g. a vendor.yaml source's per-target version overrides) resolve and cache
// independently when given distinct Discriminator values.
func TestResolveDeclaredVersion_DiscriminatorDisambiguatesSharedNames(t *testing.T) {
	atmosConfig := newVersionResolveConfig(t)
	lister := &countingLister{tags: []string{"v1.0.0", "v1.5.0", "v2.0.0", "v2.5.0"}}

	targetA, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
		RawVersion:      "^1.0.0",
		Name:            "vpc",
		Discriminator:   "target-0",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Lister:          lister,
	})
	require.NoError(t, err)

	targetB, err := ResolveDeclaredVersion(context.Background(), atmosConfig, &VersionResolveParams{
		RawVersion:      "^2.0.0",
		Name:            "vpc",
		Discriminator:   "target-1",
		SourceForGitURI: "github.com/cloudposse/terraform-aws-vpc",
		Lister:          lister,
	})
	require.NoError(t, err)

	assert.Equal(t, "v1.5.0", targetA)
	assert.Equal(t, "v2.5.0", targetB)
	assert.Equal(t, 2, lister.calls)

	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, lock.Artifacts, 2, "distinct discriminators must produce distinct lock entries")
}
