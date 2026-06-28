package vendoring

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// mockDiffer records the refs it was asked to diff and returns a canned result.
type mockDiffer struct {
	gotFrom, gotTo, gotFile string
	result                  string
}

func (m *mockDiffer) Diff(_ context.Context, _, from, to, file string) (string, error) {
	m.gotFrom, m.gotTo, m.gotFile = from, to, file
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
	full := "diff --git a/main.tf b/main.tf\n+x\ndiff --git a/README.md b/README.md\n+y\n"
	got := selectFileSections(full, "main.tf")
	assert.Contains(t, got, "main.tf")
	assert.NotContains(t, got, "README.md")
}
