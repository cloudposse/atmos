// Package local is the filesystem implementation of [blob.Store].
//
// Objects are plain files under a root directory, so a directory written here is readable with
// ordinary tools and matches the legacy CI "local/dir" layout byte for byte. Writes stream to a
// temporary file in the destination directory and are renamed into place, so readers never see a
// partial object. Content type, user metadata, and tags live in an optional "<key>.atmos-blob.json"
// attribute file that is written only when an attribute is supplied.
//
// Conditional writes and deletes are emulated: the condition is checked and the rename performed
// while holding a per-root lock (an in-process mutex plus an OS file lock), so concurrent
// conditional operations from any process are linearizable. Unconditional writes do not take the
// lock unless [WithLockAllWrites] is set; callers that mix conditional and unconditional writers
// on one key must enable it.
//
// A local object cannot share a path with a key prefix: "a" and "a/b" cannot both exist. ETags are
// opaque tokens: the SHA-256 of the content for small objects and a weak size/mtime token for
// objects over the content-hash limit. Listings do not populate ETag; call Stat for a token.
package local
