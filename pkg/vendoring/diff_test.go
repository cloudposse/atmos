package vendoring

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
)

// mockDiffer records the refs it was asked to diff and returns a canned result.
type mockDiffer struct {
	gotFrom, gotTo, gotFile string
	result                  string
	err                     error
}

func (m *mockDiffer) Diff(_ context.Context, _, from, to, file string) (string, error) {
	m.gotFrom, m.gotTo, m.gotFile = from, to, file
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func TestDiff_ExplicitFromTo(t *testing.T) {
	differ := &mockDiffer{result: "DIFF-BODY"}
	out, err := Diff(nil, &DiffParams{
		Source: "github.com/cloudposse/terraform-aws-vpc",
		From:   "1.0.0",
		To:     "2.0.0",
		Differ: differ,
	})
	require.NoError(t, err)
	assert.Equal(t, "DIFF-BODY", out)
	assert.Equal(t, "1.0.0", differ.gotFrom)
	assert.Equal(t, "2.0.0", differ.gotTo)
}

func TestDiff_DefaultToLatest(t *testing.T) {
	differ := &mockDiffer{result: "DIFF"}
	lister := &fakeLister{tagsByURI: map[string][]string{
		"https://github.com/cloudposse/terraform-aws-vpc.git": {"1.0.0", "1.2.0", "1.1.0"},
	}}
	_, err := Diff(nil, &DiffParams{
		Source: "github.com/cloudposse/terraform-aws-vpc",
		From:   "1.0.0",
		Differ: differ,
		Lister: lister,
	})
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", differ.gotTo, "To should default to the latest tag")
}

func TestDiff_PropagatesListerError(t *testing.T) {
	wantErr := assert.AnError
	_, err := Diff(nil, &DiffParams{
		Source: "github.com/cloudposse/terraform-aws-vpc",
		From:   "1.0.0",
		Differ: &mockDiffer{},
		Lister: &fakeLister{err: wantErr},
	})
	require.ErrorIs(t, err, wantErr)
}

func TestDiff_DefaultToLatestRequiresSemverTag(t *testing.T) {
	_, err := Diff(nil, &DiffParams{
		Source: "github.com/cloudposse/terraform-aws-vpc",
		From:   "1.0.0",
		Differ: &mockDiffer{},
		Lister: &fakeLister{tagsByURI: map[string][]string{
			"https://github.com/cloudposse/terraform-aws-vpc.git": {"main", "release"},
		}},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVendorDiffFailed)
}

func TestDiff_ForwardsFileFilter(t *testing.T) {
	differ := &mockDiffer{result: "DIFF"}
	_, err := Diff(nil, &DiffParams{
		Source: "github.com/cloudposse/terraform-aws-vpc",
		From:   "1.0.0",
		To:     "2.0.0",
		File:   "main.tf",
		Differ: differ,
	})
	require.NoError(t, err)
	assert.Equal(t, "main.tf", differ.gotFile)
}

func TestDiff_UsesDefaultDiffer(t *testing.T) {
	original := DefaultDiffer
	differ := &mockDiffer{result: "DIFF"}
	DefaultDiffer = differ
	t.Cleanup(func() {
		DefaultDiffer = original
	})

	got, err := Diff(nil, &DiffParams{
		Source: "github.com/cloudposse/terraform-aws-vpc",
		From:   "1.0.0",
		To:     "2.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "DIFF", got)
	assert.Equal(t, "2.0.0", differ.gotTo)
}

func TestDiff_NonGitSourceRejected(t *testing.T) {
	_, err := Diff(nil, &DiffParams{
		Source: "oci://ghcr.io/cloudposse/mock:1.0.0",
		From:   "1.0.0",
		To:     "2.0.0",
		Differ: &mockDiffer{},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVendorSourceNotGit)
}

func TestDiff_MissingFromErrors(t *testing.T) {
	_, err := Diff(nil, &DiffParams{
		Source: "github.com/cloudposse/terraform-aws-vpc",
		To:     "2.0.0",
		Differ: &mockDiffer{},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVendorDiffFailed)
}

func TestSelectFileSections(t *testing.T) {
	full := "diff --git a/main.tf b/main.tf\n+x\ndiff --git a/examples/main.tfvars b/examples/main.tfvars\n+y\ndiff --git a/README.md b/README.md\n+z\n"
	got := selectFileSections(full, "main.tf")
	assert.Contains(t, got, "main.tf")
	assert.NotContains(t, got, "main.tfvars")
	assert.NotContains(t, got, "README.md")
}

func TestRefCandidates(t *testing.T) {
	assert.Equal(t, []string{"v1.2.3", "1.2.3"}, refCandidates("v1.2.3"))
	assert.Equal(t, []string{"1.2.3", "v1.2.3"}, refCandidates("1.2.3"))
}

func TestGoGitDiffer_DiffLocalRepository(t *testing.T) {
	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)

	commit := func(tag, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.tf"), []byte(body), 0o644))
		wt, err := repo.Worktree()
		require.NoError(t, err)
		_, err = wt.Add("main.tf")
		require.NoError(t, err)
		hash, err := wt.Commit(tag, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Atmos Test",
				Email: "atmos@example.com",
				When:  time.Unix(1000, 0),
			},
		})
		require.NoError(t, err)
		_, err = repo.CreateTag(tag, hash, nil)
		require.NoError(t, err)
	}

	commit("v1.0.0", "resource \"null_resource\" \"old\" {}\n")
	commit("v1.1.0", "resource \"null_resource\" \"new\" {}\n")

	got, err := (&GoGitDiffer{}).Diff(context.Background(), repoDir, "1.0.0", "v1.1.0", "main.tf")
	require.NoError(t, err)
	assert.Contains(t, got, "diff --git")
	assert.Contains(t, got, "null_resource")

	_, err = (&GoGitDiffer{}).Diff(context.Background(), repoDir, "missing", "v1.1.0", "")
	require.ErrorIs(t, err, errUtils.ErrInvalidGitRef)
}
