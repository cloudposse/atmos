// Package local is the filesystem implementation of [blob.Store].
//
// Objects are plain files under a root directory, so a directory written here is readable with
// ordinary tools. Writes stream to a
// temporary file in the destination directory and are renamed into place, so readers never see a
// partial object. Content type, user metadata, and tags live in an optional "<key>.atmos-blob.json"
// attribute file that is written only when an attribute is supplied.
//
// Managed reads, writes, and deletes hold a per-root process and file lock. A recoverable journal
// protects content and attributes across interrupted commits; recovery runs before the next managed
// operation. Direct filesystem edits and mixing [WithPlainFileCompatibility] with managed stores
// on one root are outside this coordination. Plain-file mode preserves the legacy CI layout and
// unlocked unconditional operations. The journal recovers interrupted processes, not power loss.
//
// A local object cannot share a path with a key prefix: "a" and "a/b" cannot both exist. ETags are
// opaque content-based SHA-256 tokens. Identical contents have the same token, including after an
// intervening write. Get and Stat stream the entire file to compute the token, using bounded memory;
// listings do not populate ETag.
package local
