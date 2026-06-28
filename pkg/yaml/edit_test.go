package yaml

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureWithComments = `# Header comment.
vars:
  # Region used everywhere.
  region: us-east-1  # inline
  enabled: true
  count: 3
components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"
sources:
  - component: vpc
    version: "1.0.0"
  - component: eks
    version: "2.0.0"
`

func TestSet_PreservesCommentsAndUpdatesValue(t *testing.T) {
	out, err := Set([]byte(fixtureWithComments), "vars.region", "us-west-2")
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "# Header comment.", "header comment preserved")
	assert.Contains(t, s, "# Region used everywhere.", "head comment preserved")
	assert.Contains(t, s, "# inline", "inline comment preserved")
	assert.Contains(t, s, "us-west-2", "value updated")
	assert.NotContains(t, s, "us-east-1", "old value removed")
}

func TestSet_NestedAndIndexedPaths(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		value    string
		contains string
	}{
		{"deep nested", "components.terraform.vpc.vars.cidr", "10.1.0.0/16", "10.1.0.0/16"},
		{"sequence index", "sources[0].version", "1.2.3", "1.2.3"},
		{"second sequence index", "sources[1].version", "3.0.0", "3.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Set([]byte(fixtureWithComments), tt.path, tt.value)
			require.NoError(t, err)
			assert.Contains(t, string(out), tt.contains)
			// Comments must survive every edit.
			assert.Contains(t, string(out), "# Header comment.")
		})
	}
}

func TestSetRaw_TypedValues(t *testing.T) {
	out, err := SetRaw([]byte(fixtureWithComments), "vars.count", "5")
	require.NoError(t, err)
	assert.Contains(t, string(out), "count: 5")
	assert.NotContains(t, string(out), `count: "5"`, "raw int must not be quoted")
}

func TestGet(t *testing.T) {
	got, err := Get([]byte(fixtureWithComments), "vars.region")
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", got)

	got, err = Get([]byte(fixtureWithComments), "sources[1].component")
	require.NoError(t, err)
	assert.Equal(t, "eks", got)
}

func TestGet_NotFound(t *testing.T) {
	_, err := Get([]byte(fixtureWithComments), "vars.does_not_exist")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrYAMLPathNotFound))
}

func TestGetTyped(t *testing.T) {
	enabled, err := GetTyped[bool]([]byte(fixtureWithComments), "vars.enabled")
	require.NoError(t, err)
	assert.True(t, enabled)

	count, err := GetTyped[int]([]byte(fixtureWithComments), "vars.count")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestDelete(t *testing.T) {
	out, err := Delete([]byte(fixtureWithComments), "vars.enabled")
	require.NoError(t, err)
	assert.NotContains(t, string(out), "enabled:")
	assert.Contains(t, string(out), "# Header comment.", "comments survive delete")
}

// --- Anchor / alias strict-guard tests --------------------------------------

const fixtureWithAnchors = `vars:
  tags: &commontags
    Team: platform
    Env: prod
components:
  vpc:
    tags: *commontags
  rds:
    tags: *commontags
`

func TestSet_UntouchedAnchorsArePreserved(t *testing.T) {
	// Editing a value unrelated to the anchor must succeed and keep anchors intact.
	out, err := SetRaw([]byte(fixtureWithAnchors), "components.vpc.name", `"my-vpc"`)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "&commontags", "anchor preserved")
	assert.Equal(t, 2, strings.Count(s, "*commontags"), "both aliases preserved")
}

func TestSet_EditThroughAliasIsRejected(t *testing.T) {
	// Assigning through an alias (.components.vpc.tags is *commontags) mutates the
	// shared anchor and silently affects rds. The strict guard must reject it.
	_, err := Set([]byte(fixtureWithAnchors), "components.vpc.tags.Team", "networking")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrYAMLAnchorAltered), "got: %v", err)
}

func TestSet_EditAnchorDefinitionIsRejected(t *testing.T) {
	// Editing the anchor definition itself changes every aliasing location, so the
	// strict contract rejects it too.
	_, err := Set([]byte(fixtureWithAnchors), "vars.tags.Team", "networking")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrYAMLAnchorAltered), "got: %v", err)
}

// --- File wrapper tests ------------------------------------------------------

func TestSetFile_AtomicAndPreservesMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o640))

	require.NoError(t, SetFile(file, "vars.region", "eu-central-1"))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(got), "eu-central-1")
	assert.Contains(t, string(got), "# Header comment.")

	info, err := os.Stat(file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "original mode preserved")
}

func TestGetFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))

	got, err := GetFile(file, "components.terraform.vpc.vars.cidr")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.0/16", got)
}

// --- Dot-path translation tests ---------------------------------------------

func TestDotPathToYqPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"vars.region", ".vars.region"},
		{"sources[0].version", ".sources[0].version"},
		{"components.terraform.vpc.vars.cidr", ".components.terraform.vpc.vars.cidr"},
		{`metadata."weird.key"`, `.metadata.["weird.key"]`},
		{".already.yq.path", ".already.yq.path"}, // pass-through
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := DotPathToYqPath(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDotPathToYqPath_Errors(t *testing.T) {
	for _, in := range []string{"", "  ", "a[", `b."x`} {
		_, err := DotPathToYqPath(in)
		assert.Error(t, err, "input %q should error", in)
	}
}

// TestDotPathToYqPath_EmptySegments verifies that a dot separator producing an
// empty key segment is rejected rather than silently normalized away (which
// would let a typo target a different key on Set/Delete).
func TestDotPathToYqPath_EmptySegments(t *testing.T) {
	reject := []string{"a..b", "a.", "a...b", "a..", "vars..region"}
	for _, in := range reject {
		t.Run("reject/"+in, func(t *testing.T) {
			_, err := DotPathToYqPath(in)
			require.Error(t, err, "input %q should error", in)
			assert.True(t, errors.Is(err, ErrInvalidYAMLExpression), "got: %v", err)
		})
	}

	// The empty buffer that legitimately follows a [N] index must still parse.
	accept := []string{"sources[0].version", "a[0].b", `metadata."weird.key"`}
	for _, in := range accept {
		t.Run("accept/"+in, func(t *testing.T) {
			_, err := DotPathToYqPath(in)
			require.NoError(t, err, "input %q should parse", in)
		})
	}
}

func TestQuotePathSegment(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"vpc", "vpc"},
		{"tgw-hub", "tgw-hub"},
		{"my_component", "my_component"},
		{"vpc.prod", `"vpc.prod"`},
		{"foo[0]", `"foo[0]"`},
		{"vpc/prod", `"vpc/prod"`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, QuotePathSegment(tt.in))
		})
	}
}

// TestQuery_EmptyScalarVsMissing verifies Query distinguishes a legitimate empty
// scalar value from a missing match.
func TestQuery_EmptyScalarVsMissing(t *testing.T) {
	content := []byte("vars:\n  empty: \"\"\n  region: us-east-1\n")

	// An explicit empty string scalar is a valid value, not "not found".
	got, err := Query(content, `.vars.empty`)
	require.NoError(t, err)
	assert.Equal(t, "", got)

	// A non-matching select() yields no output and must be reported as not found.
	_, err = Query(content, `.vars[] | select(. == "nope")`)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrYAMLPathNotFound), "got: %v", err)
}

// TestSetFile_ReplacesExistingFile is a regression test for the atomic write
// path: editing an already-existing file must replace it in place (the previous
// os.Rename implementation could not replace an existing file on Windows).
func TestSetFile_ReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))

	// First edit replaces the existing file.
	require.NoError(t, SetFile(file, "vars.region", "eu-west-1"))
	// Second edit replaces it again, proving repeated in-place replacement works.
	require.NoError(t, SetFile(file, "vars.region", "ap-south-1"))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(got), "ap-south-1")
	assert.NotContains(t, string(got), "eu-west-1")
	assert.NotContains(t, string(got), "us-east-1")
	assert.Contains(t, string(got), "# Header comment.", "comments survive replacement")
}
