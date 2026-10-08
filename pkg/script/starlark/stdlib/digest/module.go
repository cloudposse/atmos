// Package digest exposes message digests to the Atmos Automation Language.
package digest

import (
	"crypto/md5"  //nolint:gosec // md5 is offered for checksums and interoperability (S3 ETags), not for security.
	"crypto/sha1" //nolint:gosec // sha1 is offered for checksums and interoperability (Git blob ids), not for security.
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/perf"
)

// digest returns a builtin that hashes its single string argument and returns lowercase hex.
// Starlark strings are byte strings, so binary file contents from fs.read_file hash correctly.
func digest(name string, constructor func() hash.Hash) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var data string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "data", &data); err != nil {
			return nil, err
		}
		h := constructor()
		// hash.Hash.Write never returns an error.
		_, _ = h.Write([]byte(data))
		return starlark.String(hex.EncodeToString(h.Sum(nil))), nil
	})
}

// New returns the stateless digest module.
func New() starlark.Value {
	defer perf.Track(nil, "hash.New")()

	return &starlarkstruct.Module{Name: "digest", Members: starlark.StringDict{
		"md5":    digest("digest.md5", md5.New),
		"sha1":   digest("digest.sha1", sha1.New),
		"sha256": digest("digest.sha256", sha256.New),
		"sha512": digest("digest.sha512", sha512.New),
	}}
}
