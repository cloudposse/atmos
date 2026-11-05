package engine

import (
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestTargetReferencesFileContext covers every parsed-node shape
// nodeReferencesFileContext's switch handles, using real template strings
// (not hand-built AST) so each case documents actual target: syntax an
// author could write -- see TargetReferencesFileContext's own doc comment
// for why this walks the AST instead of a substring search.
func TestTargetReferencesFileContext(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "plain text with no template action at all",
			target: "out/static.txt",
			want:   false,
		},
		{
			name:   "direct .file.Path field access",
			target: "out/{{ .file.Path }}",
			want:   true,
		},
		{
			name:   "direct .file.RelPath field access",
			target: "out/{{ .file.RelPath }}",
			want:   true,
		},
		{
			name:   "unrelated field access does not match",
			target: "out/{{ .matrix.env }}",
			want:   false,
		},
		{
			name:   "invalid .file.Unknown field never matches",
			target: "out/{{ .file.Unknown }}",
			want:   false,
		},
		{
			name:   "reference piped through a function still matches (multi-command pipe)",
			target: "out/{{ .file.RelPath | lower }}",
			want:   true,
		},
		{
			name:   "reference used only as an if condition still matches (BranchNode.Pipe)",
			target: "out/{{ if .file.RelPath }}main.tf{{ end }}",
			want:   true,
		},
		{
			name:   "reference in an if's true branch matches (BranchNode.List)",
			target: "out/{{ if true }}{{ .file.RelPath }}{{ end }}",
			want:   true,
		},
		{
			name:   "reference only in an if's else branch matches (BranchNode.ElseList)",
			target: "out/{{ if false }}fixed{{ else }}{{ .file.RelPath }}{{ end }}",
			want:   true,
		},
		{
			name:   "if with no reference anywhere does not match",
			target: "out/{{ if true }}fixed{{ else }}also-fixed{{ end }}",
			want:   false,
		},
		{
			name:   "reference inside a range condition matches (RangeNode)",
			target: "out/{{ range .file.RelPath }}{{ . }}{{ end }}",
			want:   true,
		},
		{
			name:   "range with no reference does not match",
			target: "out/{{ range .matrix.list }}{{ . }}{{ end }}",
			want:   false,
		},
		{
			name:   "reference inside a with condition matches (WithNode)",
			target: "out/{{ with .file.RelPath }}{{ . }}{{ end }}",
			want:   true,
		},
		{
			name:   "with no reference does not match",
			target: "out/{{ with .matrix.env }}{{ . }}{{ end }}",
			want:   false,
		},
		{
			name:   "template invocation with an argument pipe referencing it matches (TemplateNode.Pipe)",
			target: `out/{{ template "x" .file.RelPath }}`,
			want:   true,
		},
		{
			name:   "template invocation with no argument pipe does not match (TemplateNode nil Pipe)",
			target: `out/{{ template "x" }}`,
			want:   false,
		},
		{
			name:   "chained field access off a parenthesized pipeline matches (ChainNode.Field)",
			target: "out/{{ (now).file.Path }}",
			want:   true,
		},
		{
			name:   "chained field access with an unrelated selector recurses into the chain's own node and finds nothing",
			target: "out/{{ (now).matrix.env }}",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TargetReferencesFileContext(tt.target, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTargetReferencesFileContext_MalformedTemplateErrors covers
// TargetReferencesFileContext's own parse-error path (the corresponding
// success path is exercised by every case in TestTargetReferencesFileContext
// above).
func TestTargetReferencesFileContext_MalformedTemplateErrors(t *testing.T) {
	_, err := TargetReferencesFileContext("out/{{ .file.RelPath", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}

// TestTargetReferencesFileContext_CustomDelimiters proves the AST walk
// respects a scaffold's own spec.delimiters override, not just the default
// "{{"/"}}" -- mirroring ExpandMatrix's own custom-delimiter test coverage.
func TestTargetReferencesFileContext_CustomDelimiters(t *testing.T) {
	got, err := TargetReferencesFileContext("out/[[ .file.RelPath ]]", []string{"[[", "]]"})

	require.NoError(t, err)
	assert.True(t, got)
}

// TestNodeReferencesFileContext_NilGuards directly exercises
// nodeReferencesFileContext's two defensive nil-node guards (*parse.ListNode
// and *parse.PipeNode), which a real parsed template never actually
// produces -- calling the function directly with an explicit nil node is
// the only way to exercise these lines.
func TestNodeReferencesFileContext_NilGuards(t *testing.T) {
	assert.False(t, nodeReferencesFileContext((*parse.ListNode)(nil)))
	assert.False(t, nodeReferencesFileContext((*parse.PipeNode)(nil)))
}

// TestNodeReferencesFileContext_UnhandledNodeKindsReturnFalse covers the
// switch's implicit default (a node kind not explicitly handled always
// contributes false, never panics) -- e.g. an argument that's a bare
// number, string, or boolean literal rather than a field access.
func TestNodeReferencesFileContext_UnhandledNodeKindsReturnFalse(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "number literal argument", target: "out/{{ 42 }}"},
		{name: "string literal argument", target: `out/{{ "literal" }}`},
		{name: "boolean literal argument", target: "out/{{ true }}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TargetReferencesFileContext(tt.target, nil)
			require.NoError(t, err)
			assert.False(t, got)
		})
	}
}
