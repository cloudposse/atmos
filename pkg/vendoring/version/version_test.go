package version

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockLister is a RemoteLister returning canned tags for unit tests.
type mockLister struct {
	tags []string
	err  error
}

func (m *mockLister) ListTags(context.Context, string) ([]string, error) {
	return m.tags, m.err
}

// Compile-time guard that mockLister satisfies RemoteLister.
var _ RemoteLister = (*mockLister)(nil)

func TestExtractGitURI(t *testing.T) {
	tests := []struct{ in, want string }{
		{"github.com/cloudposse/terraform-aws-vpc", "https://github.com/cloudposse/terraform-aws-vpc.git"},
		{"git::https://github.com/cloudposse/terraform-aws-vpc.git?ref=v1", "https://github.com/cloudposse/terraform-aws-vpc.git"},
		{"github.com/org/repo//modules/vpc?ref=v1.2.3", "https://github.com/org/repo.git"},
		{"git::https://github.com/org/repo.git//modules/vpc?ref=v1.2.3", "https://github.com/org/repo.git"},
		{"https://github.com/cloudposse/terraform-aws-vpc.git", "https://github.com/cloudposse/terraform-aws-vpc.git"},
		{"github.com/foo/bar?ref={{.Version}}", "https://github.com/foo/bar.git"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ExtractGitURI(tt.in), "in=%s", tt.in)
	}
}

func TestGoGitLister_ListTagsLocalRepository(t *testing.T) {
	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	hash, err := wt.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Atmos Test",
			Email: "atmos@example.com",
			When:  time.Unix(1000, 0),
		},
	})
	require.NoError(t, err)
	_, err = repo.CreateTag("v1.0.0", hash, nil)
	require.NoError(t, err)
	_, err = repo.CreateTag("not-semver", hash, nil)
	require.NoError(t, err)

	tags, err := (&GoGitLister{}).ListTags(context.Background(), repoDir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"v1.0.0", "not-semver"}, tags)

	_, err = (&GoGitLister{}).ListTags(context.Background(), filepath.Join(t.TempDir(), "missing.git"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitLsRemoteFailed)
}

func TestIsGitSource(t *testing.T) {
	assert.True(t, IsGitSource("github.com/cloudposse/terraform-aws-vpc"))
	assert.True(t, IsGitSource("git::https://example.com/x.git"))
	assert.True(t, IsGitSource("https://gitlab.com/foo/bar"))
	assert.False(t, IsGitSource("oci://ghcr.io/cloudposse/mock:1.0.0"))
	assert.False(t, IsGitSource("s3://bucket/key"))
}

func TestIsValidCommitSHA(t *testing.T) {
	assert.True(t, IsValidCommitSHA("abc1234"))
	assert.True(t, IsValidCommitSHA("0123456789abcdef0123456789abcdef01234567"))
	assert.False(t, IsValidCommitSHA("v1.2.3"))
	assert.False(t, IsValidCommitSHA("xyz"))
}

func TestIsTemplatedVersion(t *testing.T) {
	assert.True(t, IsTemplatedVersion("{{.Version}}"))
	assert.False(t, IsTemplatedVersion("1.2.3"))
}

func TestIsSemverConstraint(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty", "", false},
		{"exact bare version", "1.2.3", false},
		{"exact v-prefixed version", "v1.2.3", false},
		{"commit sha", "abc1234", false},
		{"branch name", "main", false},
		{"caret range", "^1.0.0", true},
		{"tilde range", "~1.2.3", true},
		{"comparator range", ">=1.0.0 <2.0.0", true},
		{"wildcard", "*", true},
		{"minor wildcard shorthand", "1.x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsSemverConstraint(tt.value))
		})
	}
}

// TestValidateVersionRangeConstraints proves version: and constraints.version are mutually
// exclusive: a range-shaped version: combined with a non-empty constraints.version is a hard
// error naming both fields, while constraints.excluded_versions/no_prereleases remain accepted
// alongside a range, and an exact-pinned version: is never rejected regardless of constraints.
func TestValidateVersionRangeConstraints(t *testing.T) {
	tests := []struct {
		name        string
		rawVersion  string
		constraints *schema.VendorConstraints
		wantErr     bool
	}{
		{"exact pin, no constraints", "v1.2.3", nil, false},
		{"exact pin, constraints.version set", "v1.2.3", &schema.VendorConstraints{Version: "<2.0.0"}, false},
		{"range, no constraints", "^1.0.0", nil, false},
		{"range, constraints.version empty", "^1.0.0", &schema.VendorConstraints{ExcludedVersions: []string{"1.5.0"}}, false},
		{"range, constraints.excluded_versions and no_prereleases only", "^1.0.0", &schema.VendorConstraints{ExcludedVersions: []string{"1.5.0"}, NoPrereleases: true}, false},
		{"range, constraints.version conflicts", "^1.0.0", &schema.VendorConstraints{Version: "<2.0.0"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersionRangeConstraints(tt.rawVersion, tt.constraints)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrVersionRangeConflictsWithConstraints)
			assert.Contains(t, err.Error(), tt.rawVersion)
			assert.Contains(t, err.Error(), tt.constraints.Version)
		})
	}
}

func TestFindLatestSemVerTag(t *testing.T) {
	tags := []string{"v1.0.0", "v1.2.0", "not-a-version", "v1.1.5", "0.9.0"}
	ver, tag := FindLatestSemVerTag(tags)
	require.NotNil(t, ver)
	assert.Equal(t, "v1.2.0", tag)
}

func TestSelectLatestVersion(t *testing.T) {
	got, err := SelectLatestVersion([]string{"1.0.0", "2.3.1", "2.0.0"})
	require.NoError(t, err)
	assert.Equal(t, "2.3.1", got)

	_, err = SelectLatestVersion(nil)
	require.Error(t, err)
}

func TestMatchesWildcard(t *testing.T) {
	assert.True(t, MatchesWildcard("1.2.3", "1.2.3"))
	assert.True(t, MatchesWildcard("1.5.9", "1.5.*"))
	assert.False(t, MatchesWildcard("1.6.0", "1.5.*"))
	assert.False(t, MatchesWildcard("1.2.3", "1.2.4"))
}

func TestResolveVersionConstraints(t *testing.T) {
	available := []string{"1.0.0", "1.2.3", "1.5.0", "1.5.1", "2.0.0", "2.1.0-rc.1"}

	tests := []struct {
		name        string
		constraints *schema.VendorConstraints
		want        string
		wantErr     bool
	}{
		{
			name:        "no constraints -> absolute latest (incl. prerelease)",
			constraints: nil,
			want:        "2.1.0-rc.1", // semver: 2.1.0-rc.1 > 2.0.0; no_prereleases is opt-in
		},
		{
			name:        "no_prereleases drops the rc, picks 2.0.0",
			constraints: &schema.VendorConstraints{NoPrereleases: true},
			want:        "2.0.0",
		},
		{
			name:        "caret keeps 1.x",
			constraints: &schema.VendorConstraints{Version: "^1.0.0"},
			want:        "1.5.1",
		},
		{
			name:        "tilde patch only",
			constraints: &schema.VendorConstraints{Version: "~1.5.0"},
			want:        "1.5.1",
		},
		{
			name:        "exclude wildcard series",
			constraints: &schema.VendorConstraints{Version: "^1.0.0", ExcludedVersions: []string{"1.5.*"}},
			want:        "1.2.3",
		},
		{
			name:        "no prereleases on range that only has a prerelease",
			constraints: &schema.VendorConstraints{Version: ">=2.1.0", NoPrereleases: true},
			wantErr:     true,
		},
		{
			name:        "invalid constraint errors",
			constraints: &schema.VendorConstraints{Version: "not-a-constraint"},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveVersionConstraints(available, tt.constraints)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterPrereleases(t *testing.T) {
	got := FilterPrereleases([]string{"1.0.0", "1.1.0-rc.1", "2.0.0-beta"})
	assert.Equal(t, []string{"1.0.0"}, got)
}

func TestFilterBySemverConstraint_InvalidConstraint(t *testing.T) {
	_, err := FilterBySemverConstraint([]string{"1.0.0"}, "not-a-constraint")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidSemverConstraint)
}

func TestFilterBySemverConstraint_ValidConstraint(t *testing.T) {
	got, err := FilterBySemverConstraint([]string{"0.9.0", "1.0.0", "1.5.0", "2.0.0"}, "^1.0.0")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "1.0.0", got[0])
	assert.Equal(t, "1.5.0", got[1])
}

func TestFilterExcludedVersions(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		excluded []string
		want     []string
	}{
		{
			name:     "no exclusions",
			versions: []string{"1.0.0", "1.1.0"},
			excluded: nil,
			want:     []string{"1.0.0", "1.1.0"},
		},
		{
			name:     "exact match excluded",
			versions: []string{"1.0.0", "1.1.0", "1.2.0"},
			excluded: []string{"1.1.0"},
			want:     []string{"1.0.0", "1.2.0"},
		},
		{
			name:     "wildcard match excluded",
			versions: []string{"1.0.0", "1.5.0", "1.5.1", "2.0.0"},
			excluded: []string{"1.5.*"},
			want:     []string{"1.0.0", "2.0.0"},
		},
		{
			name:     "no match keeps everything",
			versions: []string{"1.0.0", "1.1.0"},
			excluded: []string{"9.9.9"},
			want:     []string{"1.0.0", "1.1.0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterExcludedVersions(tt.versions, tt.excluded)
			assert.Equal(t, tt.want, got)
		})
	}
}
