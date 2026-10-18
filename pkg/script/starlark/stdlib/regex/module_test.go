package regex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func TestModuleExpressions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ source, want string }{
		{`regex.search(pattern="world", text="hello world")`, `True`},
		{`regex.search("^world", "hello world")`, `False`},
		{`regex.search("(?im)^ready$", "waiting\nREADY")`, `True`},
		{`regex.replace("(secret)", "$1", "secret secret")`, `"$1 $1"`},
		{`regex.findall("v([0-9]+)", "v12 v3")`, `["v12", "v3"]`},
		{`regex.findall("x", "abc")`, `[]`},
		{`regex.findall("\\p{L}+", "café 42 日本")`, `["café", "日本"]`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			result, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "regex.star", tc.source, starlark.StringDict{"regex": New()})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.String())
		})
	}
}

func TestModuleRejectsInvalidCalls(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`regex.search("[", "text")`, `regex.replace("[", "", "text")`, `regex.findall("[", "text")`,
		`regex.search("(?<=a)b", "ab")`, `regex.search(1,"text")`, `regex.replace("a","b")`, `regex.findall("a",None)`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "regex.star", source, starlark.StringDict{"regex": New()})
			require.Error(t, err)
		})
	}
}
