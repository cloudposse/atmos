package digest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func TestModuleDigests(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ source, want string }{
		// Known answers from FIPS 180 and RFC 1321.
		{`digest.sha256("")`, `"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`},
		{`digest.sha256("abc")`, `"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"`},
		{`digest.sha256(data="abc")`, `"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"`},
		{`digest.sha512("abc")`, `"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"`},
		{`digest.sha1("abc")`, `"a9993e364706816aba3e25717850c26c9cd0d89d"`},
		{`digest.md5("abc")`, `"900150983cd24fb0d6963f7d28e17f72"`},
		// Non-ASCII text hashes its UTF-8 bytes.
		{`digest.md5("\u00ff")`, `"f3f7437e8c1f303fc6742d9368b36cf6"`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			result, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "digest.star", tc.source, starlark.StringDict{"digest": New()})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.String())
		})
	}
}

func TestModuleRejectsInvalidCalls(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`digest.sha256()`, `digest.sha256(1)`, `digest.sha256(None)`, `digest.sha256("a", "b")`, `digest.md5(text="a")`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "digest.star", source, starlark.StringDict{"digest": New()})
			require.Error(t, err)
		})
	}
}
