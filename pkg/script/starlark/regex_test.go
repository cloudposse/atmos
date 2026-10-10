package starlark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRegex(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, want string }{
		{"search anywhere", `output = regex.search(pattern="world", text="hello world")`, `true`},
		{"search absent", `output = regex.search("^world", "hello world")`, `false`},
		{"inline flags", `output = regex.search("(?im)^ready$", "waiting\nREADY")`, `true`},
		{"strip ANSI", `output = json.encode(regex.replace("\x1b\\[[0-9;]*m", "", "\x1b[32mready\x1b[0m"))`, `"ready"`},
		{"literal replacement", `output = json.encode(regex.replace("(secret)", "$1", "secret secret"))`, `"$1 $1"`},
		{"full matches", `output = regex.findall("v([0-9]+)", "v12 v3")`, `["v12","v3"]`},
		{"no matches", `output = regex.findall("x", "abc")`, `[]`},
		{"unicode", `output = regex.findall("\\p{L}+", "café 42 日本")`, `["café","日本"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := runSource(t, tc.source)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, result.Value)
		})
	}
}

func TestRegexInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`regex.search("[", "text")`, `regex.replace("[", "", "text")`, `regex.findall("[", "text")`,
		`regex.search("(?<=a)b", "ab")`,
	} {
		_, err := runSource(t, source)
		require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument, source)
	}
	for _, source := range []string{`regex.search(1, "text")`, `regex.replace("a", "b")`, `regex.findall("a", None)`} {
		_, err := runSource(t, source)
		require.ErrorIs(t, err, errUtils.ErrStarlark, source)
	}
}
