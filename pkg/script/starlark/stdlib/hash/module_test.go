package hash

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
		{`hash.sha256("")`, `"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`},
		{`hash.sha256("abc")`, `"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"`},
		{`hash.sha256(data="abc")`, `"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"`},
		{`hash.sha512("abc")`, `"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"`},
		{`hash.sha1("abc")`, `"a9993e364706816aba3e25717850c26c9cd0d89d"`},
		{`hash.md5("abc")`, `"900150983cd24fb0d6963f7d28e17f72"`},
		// Non-ASCII text hashes its UTF-8 bytes.
		{`hash.md5("\u00ff")`, `"f3f7437e8c1f303fc6742d9368b36cf6"`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			result, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "hash.star", tc.source, starlark.StringDict{"hash": New()})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.String())
		})
	}
}

func TestModuleRejectsInvalidCalls(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`hash.sha256()`, `hash.sha256(1)`, `hash.sha256(None)`, `hash.sha256("a", "b")`, `hash.md5(text="a")`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "hash.star", source, starlark.StringDict{"hash": New()})
			require.Error(t, err)
		})
	}
}
