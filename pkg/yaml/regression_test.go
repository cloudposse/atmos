package yaml

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file holds regression tests from the YAML-engine audit: empty-document
// editing, multi-document rejection, duplicate-anchor guarding, null-string
// disambiguation, leading-index paths, quoted-segment round-trips, merge-key
// tag suppression, indent-width preservation, CRLF preservation, and symlink
// handling.

// --- Empty documents ----------------------------------------------------------

func TestSet_EmptyDocumentCreatesContent(t *testing.T) {
	for _, in := range []string{"", "\n", "  \n\n"} {
		out, err := Set([]byte(in), "a.b", "v")
		require.NoError(t, err, "input %q", in)
		assert.Contains(t, string(out), "a:", "input %q", in)
		assert.Contains(t, string(out), "b: v", "input %q", in)
	}
}

func TestEval_EmptyDocumentCreatesContent(t *testing.T) {
	out, err := Eval([]byte(""), `.name = "x"`)
	require.NoError(t, err)
	assert.Contains(t, string(out), "name: x")
}

func TestSetFile_EmptyFileCreatesContent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(file, []byte(""), 0o644))

	require.NoError(t, SetFile(file, "vars.region", "us-east-1"))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(got), "region: us-east-1")
}

func TestDelete_EmptyDocumentIsNoOp(t *testing.T) {
	out, err := Delete([]byte(""), "a.b")
	require.NoError(t, err)
	assert.Empty(t, string(out))
}

func TestFormat_EmptyDocumentIsNoOp(t *testing.T) {
	out, err := Format([]byte("\n"))
	require.NoError(t, err)
	assert.Equal(t, "\n", string(out), "format must not materialize a null scalar")
}

func TestGet_EmptyDocumentNotFound(t *testing.T) {
	_, err := Get([]byte(""), "a.b")
	require.ErrorIs(t, err, ErrYAMLPathNotFound)

	_, err = Query([]byte(""), ".a")
	require.ErrorIs(t, err, ErrYAMLPathNotFound)
}

// --- Multi-document streams ---------------------------------------------------

func TestMultiDocumentStreamsAreRejected(t *testing.T) {
	multi := []byte("---\nname: one\n---\nname: two\n")

	_, err := Get(multi, "name")
	assert.ErrorIs(t, err, ErrYAMLMultiDocUnsupported, "Get")

	_, err = Query(multi, ".name")
	assert.ErrorIs(t, err, ErrYAMLMultiDocUnsupported, "Query")

	_, err = Set(multi, "name", "changed")
	assert.ErrorIs(t, err, ErrYAMLMultiDocUnsupported, "Set")

	_, err = Delete(multi, "name")
	assert.ErrorIs(t, err, ErrYAMLMultiDocUnsupported, "Delete")

	_, err = Format(multi)
	assert.ErrorIs(t, err, ErrYAMLMultiDocUnsupported, "Format")

	_, err = Eval(multi, `.name = "changed"`)
	assert.ErrorIs(t, err, ErrYAMLMultiDocUnsupported, "Eval")
}

func TestSingleDocumentWithLeadingSeparatorIsAllowed(t *testing.T) {
	out, err := Set([]byte("---\nname: one\n"), "name", "changed")
	require.NoError(t, err)
	assert.Contains(t, string(out), "name: changed")
}

func TestEnsureSingleDocument_InvalidYAMLDefersToEvaluator(t *testing.T) {
	// Invalid YAML must not be reported by the document counter; the yq
	// evaluation surfaces the parse error with better context.
	_, err := Set([]byte("a: [\n"), "a", "x")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrYAMLMultiDocUnsupported)
}

// --- Duplicate anchors ---------------------------------------------------------

func TestSet_DuplicateAnchorNamesAreRejected(t *testing.T) {
	// Aliases before and after the redefinition of &x resolve to different
	// values; editing such a document risks silent shared-value mutation.
	in := "a: &x 1\nb: *x\nc: &x 2\nd: *x\n"
	_, err := Set([]byte(in), "a", "MUTATED")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrYAMLDuplicateAnchor)
}

// --- Null-string disambiguation -----------------------------------------------

func TestGet_StringNullValueIsFound(t *testing.T) {
	content := []byte("region: \"null\"\nempty: null\n")

	got, err := Get(content, "region")
	require.NoError(t, err, `a quoted "null" string is a real value`)
	assert.Equal(t, "null", got)

	// Explicit YAML null is still treated as "not present" for addressing.
	_, err = Get(content, "empty")
	assert.ErrorIs(t, err, ErrYAMLPathNotFound)

	_, err = Get(content, "missing")
	assert.ErrorIs(t, err, ErrYAMLPathNotFound)
}

func TestQuery_StringNullValueIsFound(t *testing.T) {
	content := []byte("region: \"null\"\n")

	got, err := Query(content, ".region")
	require.NoError(t, err)
	assert.Equal(t, "null", got)

	_, err = Query(content, ".missing")
	assert.ErrorIs(t, err, ErrYAMLPathNotFound)

	// A trailing comment in the expression swallows the tag-check wrapper's
	// closing paren; the disambiguation must fail closed (not found), not panic.
	_, err = Query(content, ".missing # note")
	assert.ErrorIs(t, err, ErrYAMLPathNotFound)
}

// --- Leading-index paths --------------------------------------------------------

func TestSet_TopLevelSequenceIndex(t *testing.T) {
	yq, err := DotPathToYqPath("[0]")
	require.NoError(t, err)
	assert.Equal(t, ".[0]", yq, "leading index needs an explicit dot")

	out, err := Set([]byte("- a\n- b\n"), "[0]", "z")
	require.NoError(t, err)
	assert.Contains(t, string(out), "- z")
	assert.Contains(t, string(out), "- b")

	got, err := Get([]byte("- a\n- b\n"), "[1]")
	require.NoError(t, err)
	assert.Equal(t, "b", got)
}

// --- Quoted-segment round-trips --------------------------------------------------

func TestQuotePathSegment_RoundTripsThroughDotPath(t *testing.T) {
	for _, key := range []string{`foo"bar`, `foo\bar`, `foo\"bar`, "dotted.key", "a[0]b"} {
		t.Run(key, func(t *testing.T) {
			path := "vars." + QuotePathSegment(key)
			yq, err := DotPathToYqPath(path)
			require.NoError(t, err, "path %q must parse", path)
			assert.True(t, strings.HasPrefix(yq, ".vars"), "got %q", yq)
		})
	}
}

func TestSetGet_KeyWithEmbeddedQuote(t *testing.T) {
	key := `foo"bar`
	path := "vars." + QuotePathSegment(key)

	out, err := Set([]byte("vars: {}\n"), path, "v")
	require.NoError(t, err)

	got, err := Get(out, path)
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

// --- Merge-key tag suppression ---------------------------------------------------

const mergeKeyFixture = `defaults: &d
  retries: 3
web:
  <<: *d
  port: 8080
list:
  - <<: *d
    name: item
`

func TestSet_MergeKeysAreNotRewrittenToExplicitTag(t *testing.T) {
	out, err := Set([]byte(mergeKeyFixture), "web.port", "9090")
	require.NoError(t, err)
	s := string(out)
	assert.NotContains(t, s, "!!merge", "encoder's explicit !!merge tag must be suppressed")
	assert.Equal(t, 2, strings.Count(s, "<<: *d"), "both merge keys preserved")
}

func TestFormat_MergeKeyDocumentIsStable(t *testing.T) {
	once, err := Format([]byte(mergeKeyFixture))
	require.NoError(t, err)
	assert.NotContains(t, string(once), "!!merge")

	twice, err := Format(once)
	require.NoError(t, err)
	assert.Equal(t, string(once), string(twice), "Format must stay idempotent with merge keys")
}

// --- Indent-width preservation ----------------------------------------------------

func TestSet_PreservesFourSpaceIndent(t *testing.T) {
	in := "top:\n    child:\n        leaf: 1\nother: 2\n"
	out, err := Set([]byte(in), "other", "3")
	require.NoError(t, err)
	assert.Contains(t, string(out), "    child:", "4-space indent preserved")
	assert.Contains(t, string(out), "        leaf: 1", "nested 4-space indent preserved")
}

func TestDetectIndent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"two space", "a:\n  b: 1\n", 2},
		{"four space", "a:\n    b: 1\n", 4},
		{"flat document", "a: 1\nb: 2\n", DefaultIndent},
		{"empty", "", DefaultIndent},
		{"comments only", "# one\n# two\n", DefaultIndent},
		{"comment then nested", "# header\na:\n    b: 1\n", 4},
		{"block scalar content ignored", "script: |\n        echo hi\nother:\n  b: 1\n", 2},
		{"implausibly wide falls back", "a:\n            b: 1\n", DefaultIndent},
		{"single space clamps to default", "a:\n b: 1\n", DefaultIndent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectIndent([]byte(tt.in)))
		})
	}
}

// --- Line-ending preservation ------------------------------------------------------

func TestSetFile_PreservesCRLF(t *testing.T) {
	file := filepath.Join(t.TempDir(), "crlf.yaml")
	require.NoError(t, os.WriteFile(file, []byte("a: 1\r\nb: 2\r\n"), 0o644))

	require.NoError(t, SetFile(file, "a", "x"))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "a: x\r\nb: 2\r\n", string(got))
}

func TestRestoreLineEndings(t *testing.T) {
	tests := []struct {
		name     string
		original string
		out      string
		want     string
	}{
		{"lf original untouched", "a: 1\n", "a: 2\n", "a: 2\n"},
		{"crlf original expanded", "a: 1\r\nb: 2\r\n", "a: 2\nb: 2\n", "a: 2\r\nb: 2\r\n"},
		{"mixed endings stay lf", "a: 1\r\nb: 2\n", "a: 2\nb: 2\n", "a: 2\nb: 2\n"},
		{"already crlf not doubled", "a: 1\r\n", "a: 2\r\n", "a: 2\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := restoreLineEndings([]byte(tt.original), []byte(tt.out))
			assert.Equal(t, tt.want, string(got))
		})
	}
}

// --- Symlink handling ----------------------------------------------------------------

func TestSetFile_FollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "real.yaml")
	link := filepath.Join(dir, "link.yaml")
	require.NoError(t, os.WriteFile(target, []byte("a: 1\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, SetFile(link, "a", "x"))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "link must remain a symlink")

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Contains(t, string(got), "a: x", "edit must land in the symlink target")
}
