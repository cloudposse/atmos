package yaml

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestSetWithType(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		valueType string
		want      string
	}{
		{"default string", "hello", "", "region: hello"},
		{"explicit string escapes", `foo"bar`, TypeString, `region: foo"bar`},
		{"int", "42", TypeInt, "count: 42"},
		{"float", "3.14", TypeFloat, "count: 3.14"},
		{"bool", " TRUE ", TypeBool, "enabled: true"},
		{"null", "ignored", TypeNull, "region: null"},
		{"yaml", "[1, 2, 3]", TypeYAML, "    - 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "vars.region"
			if strings.Contains(tt.want, "count:") {
				path = "vars.count"
			}
			if strings.Contains(tt.want, "enabled:") {
				path = "vars.enabled"
			}

			out, err := SetWithType([]byte(fixtureWithComments), path, tt.value, tt.valueType)
			require.NoError(t, err)
			assert.Contains(t, string(out), tt.want)
		})
	}
}

func TestSetWithType_InvalidValues(t *testing.T) {
	for _, tt := range []struct {
		name      string
		value     string
		valueType string
	}{
		{"bad int", "nope", TypeInt},
		{"bad float", "nope", TypeFloat},
		{"bad bool", "maybe", TypeBool},
		{"unknown type", "x", "object"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SetWithType([]byte(fixtureWithComments), "vars.region", tt.value, tt.valueType)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidTypedValue)
			assert.NotErrorIs(t, err, ErrInvalidYAMLExpression,
				"a value/type validation failure is not a path/expression problem -- must not share a headline with those")
		})
	}
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

	_, err = GetTyped[int]([]byte(fixtureWithComments), "vars.region")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrParseYAML)
}

func TestGetType(t *testing.T) {
	typ, ok := GetType([]byte(fixtureWithComments), "vars.enabled")
	assert.True(t, ok)
	assert.Equal(t, TypeBool, typ)

	typ, ok = GetType([]byte(fixtureWithComments), "vars.count")
	assert.True(t, ok)
	assert.Equal(t, TypeInt, typ)

	typ, ok = GetType([]byte(fixtureWithComments), "vars.region")
	assert.True(t, ok)
	assert.Equal(t, TypeString, typ)
}

// TestGetType_ListAndMap is a regression test: GetType used to fall through
// to (TypeString, true) for a !!seq or !!map tag, which callers read as a
// confidently resolved scalar inference -- letting --type=auto silently
// replace an existing list/map with a plain string. It must report
// (TypeYAML, true) instead, so callers can tell "there's a typed answer, but
// it isn't a scalar" apart from "this really is a string".
func TestGetType_ListAndMap(t *testing.T) {
	typ, ok := GetType([]byte(fixtureWithComments), "sources")
	assert.True(t, ok)
	assert.Equal(t, TypeYAML, typ)

	typ, ok = GetType([]byte(fixtureWithComments), "components.terraform.vpc.vars")
	assert.True(t, ok)
	assert.Equal(t, TypeYAML, typ)
}

func TestGetType_NotFound(t *testing.T) {
	_, ok := GetType([]byte(fixtureWithComments), "vars.does_not_exist")
	assert.False(t, ok)
}

// TestGetType_ExplicitNull is a regression test: an explicit YAML null is a
// real, present value -- distinct from a missing path -- so GetType must
// report (TypeNull, true) for it rather than folding it into the same
// ok=false result used for "nothing to infer from".
func TestGetType_ExplicitNull(t *testing.T) {
	content := []byte("vars:\n  explicit_null: null\n  region: us-east-1\n")

	typ, ok := GetType(content, "vars.explicit_null")
	assert.True(t, ok)
	assert.Equal(t, TypeNull, typ)
}

// TestGetType_RawPathExplicitNull is a regression test for a CodeRabbit
// finding: pathIsExplicitlyPresent fell back to Get's collapsed null/missing
// check for any raw yq path (one starting with "."), even a plain dot-path
// like ".vars.explicit_null" that could have been decomposed into segments
// like its non-raw counterpart. That made an explicit null under a raw path
// look "missing" (ok=false) instead of reporting (TypeNull, true).
func TestGetType_RawPathExplicitNull(t *testing.T) {
	content := []byte("vars:\n  explicit_null: null\n  region: us-east-1\n")

	typ, ok := GetType(content, ".vars.explicit_null")
	assert.True(t, ok, "a raw dot-path pointing at an explicit null must report present")
	assert.Equal(t, TypeNull, typ)

	// A genuinely missing raw path must still report absent.
	_, ok = GetType(content, ".vars.does_not_exist")
	assert.False(t, ok)
}

// TestGetType_MissingNestedPath covers a path whose ancestor segment doesn't
// exist at all (as opposed to a leaf segment that's explicitly null),
// exercising the has()-on-null-parent short-circuit in
// pathIsExplicitlyPresent.
func TestGetType_MissingNestedPath(t *testing.T) {
	content := []byte("vars:\n  region: us-east-1\n")

	_, ok := GetType(content, "vars.deeply.nested.missing")
	assert.False(t, ok)
}

// TestGetType_ArrayIndexOutOfBounds covers an array-index path segment past
// the end of the array, another shape of "missing" that pathIsExplicitlyPresent
// must not confuse with an explicit null.
func TestGetType_ArrayIndexOutOfBounds(t *testing.T) {
	_, ok := GetType([]byte(fixtureWithComments), "sources[5].component")
	assert.False(t, ok)
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
	// Regression for a field-test finding: the error must name the practical
	// workaround (an explicit override key), not just "restructure".
	assert.Contains(t, err.Error(), "add an explicit key at this path to override",
		"error should point at the actionable fix, not just say to restructure")
}

func TestSet_EditAnchorDefinitionIsRejected(t *testing.T) {
	// Editing the anchor definition itself changes every aliasing location, so the
	// strict contract rejects it too.
	_, err := Set([]byte(fixtureWithAnchors), "vars.tags.Team", "networking")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrYAMLAnchorAltered), "got: %v", err)
}

const fixtureWithScalarAlias = `components:
  vpc:
    region: &shared_region "us-east-1"
  rds:
    region: *shared_region
`

func TestSet_EditAliasUseSiteIsRejected(t *testing.T) {
	// Regression for a field-test finding: replacing an alias node itself
	// (components.rds.region IS *shared_region, not a child key inside it,
	// and not the anchor definition) flattens the alias to a literal -- a
	// distinct code path (aliasCount changed, not content changed) from
	// TestSet_EditThroughAliasIsRejected / TestSet_EditAnchorDefinitionIsRejected
	// above, previously untested through a real Set call.
	_, err := Set([]byte(fixtureWithScalarAlias), "components.rds.region", "us-west-2")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrYAMLAnchorAltered), "got: %v", err)
	assert.Contains(t, err.Error(), "alias references changed from 1 to 0")
	assert.Contains(t, err.Error(), "edit the anchor definition explicitly",
		"error should explain the real options, not stay silent")
	assert.NotContains(t, err.Error(), "add an explicit key at this path",
		"that hint is for the content-diff branch and is misleading here -- attempting it is exactly what triggers this branch")
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
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "original mode preserved")
	}
}

func TestGetFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))

	got, err := GetFile(file, "components.terraform.vpc.vars.cidr")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.0/16", got)
}

func TestGetFileType(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))

	typ, ok := GetFileType(file, "vars.enabled")
	assert.True(t, ok)
	assert.Equal(t, TypeBool, typ)

	_, ok = GetFileType(file, "vars.does_not_exist")
	assert.False(t, ok)

	_, ok = GetFileType(filepath.Join(dir, "missing.yaml"), "vars.enabled")
	assert.False(t, ok)
}

func TestFileWrappers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))

	got, err := QueryFile(file, `.vars.region`)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", got)

	require.NoError(t, EvalFile(file, `.vars.region = "us-west-1"`))
	got, err = GetFile(file, "vars.region")
	require.NoError(t, err)
	assert.Equal(t, "us-west-1", got)

	require.NoError(t, SetFileRaw(file, "vars.count", "7"))
	got, err = GetFile(file, "vars.count")
	require.NoError(t, err)
	assert.Equal(t, "7", got)

	created, err := SetFileWithType(file, "vars.enabled", "false", TypeBool)
	require.NoError(t, err)
	assert.False(t, created, "vars.enabled already existed in the fixture")
	got, err = GetFile(file, "vars.enabled")
	require.NoError(t, err)
	assert.Equal(t, "false", got)

	existed, err := DeleteFile(file, "vars.enabled")
	require.NoError(t, err)
	assert.True(t, existed)
	_, err = GetFile(file, "vars.enabled")
	require.ErrorIs(t, err, ErrYAMLPathNotFound)

	require.NoError(t, FormatFile(file))
}

func TestSetFileWithType_ReportsCreatedVsUpdated(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))

	created, err := SetFileWithType(file, "vars.timeout", "30", TypeInt)
	require.NoError(t, err)
	assert.True(t, created, "vars.timeout is new")

	created, err = SetFileWithType(file, "vars.timeout", "60", TypeInt)
	require.NoError(t, err)
	assert.False(t, created, "vars.timeout now exists")

	got, err := GetFile(file, "vars.timeout")
	require.NoError(t, err)
	assert.Equal(t, "60", got)
}

func TestDeleteFile_ReportsExistedVsNoOp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))

	before, err := os.ReadFile(file)
	require.NoError(t, err)

	existed, err := DeleteFile(file, "vars.missing")
	require.NoError(t, err)
	assert.False(t, existed, "vars.missing was never set")

	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a no-op delete must not rewrite the file")

	existed, err = DeleteFile(file, "vars.enabled")
	require.NoError(t, err)
	assert.True(t, existed, "vars.enabled was present")

	existed, err = DeleteFile(file, "vars.enabled")
	require.NoError(t, err)
	assert.False(t, existed, "vars.enabled was already removed")
}

func TestFileWrappers_ReadAndValidationErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	_, err := QueryFile(missing, ".")
	require.ErrorIs(t, err, ErrReadFile)
	_, err = GetFile(missing, ".")
	require.ErrorIs(t, err, ErrReadFile)
	assert.ErrorIs(t, SetFile(missing, "vars.region", "x"), ErrReadFile)
	assert.ErrorIs(t, SetFileRaw(missing, "vars.region", `"x"`), ErrReadFile)
	_, err = DeleteFile(missing, "vars.region")
	assert.ErrorIs(t, err, ErrReadFile)
	assert.ErrorIs(t, FormatFile(missing), ErrReadFile)

	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fixtureWithComments), 0o644))
	_, err = SetFileWithType(file, "vars.region", "not-bool", TypeBool)
	assert.ErrorIs(t, err, ErrInvalidTypedValue)
	assert.ErrorIs(t, EvalFile(file, "bad["), ErrInvalidYAMLExpression)
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
		{`foo"bar`, `"foo\"bar"`},
		{`foo\bar`, `"foo\\bar"`},
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

// TestSetRawAndDelete_InvalidDotPathErrors verifies that an invalid dot-path
// surfaces as ErrInvalidYAMLExpression through both the SetRaw and Delete call
// sites (setRawWithOptions and deleteWithOptions), not just at the
// DotPathToYqPath unit level already covered by TestDotPathToYqPath_Errors and
// TestDotPathToYqPath_EmptySegments.
func TestSetRawAndDelete_InvalidDotPathErrors(t *testing.T) {
	invalid := []string{"a..b", "a.", `b."x`, "a[0"}
	for _, path := range invalid {
		t.Run(path, func(t *testing.T) {
			_, err := SetRaw([]byte(fixtureWithComments), path, "1")
			require.Error(t, err, "SetRaw with path %q should error", path)
			require.ErrorIs(t, err, ErrInvalidYAMLExpression)

			_, err = Delete([]byte(fixtureWithComments), path)
			require.Error(t, err, "Delete with path %q should error", path)
			require.ErrorIs(t, err, ErrInvalidYAMLExpression)
		})
	}
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
