// Package snapshot captures a local source file or directory into bounded disk staging and
// produces a canonical inventory of it.
//
// Capture walks the source with lstat, copies regular file bytes into a private staging directory
// while hashing them, and then rechecks every source entry (type, size, modification time, and
// digest) so that an inventory is never published for bytes other than the ones staged. Files that
// change during capture fail the operation with ErrArtifactSourceChanged.
//
// Entry rules follow the artifact contracts: directories, file bytes, ordinary permission bits and
// contained relative symlinks are preserved; absolute or escaping symlinks, devices, sockets, and
// other special files are rejected; hard links become independent regular files; privileged mode
// bits are cleared; owner, group, and ACLs are never recorded. Owned temporary data is removed on
// success (via Close), error, and cancellation.
package snapshot
