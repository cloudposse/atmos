// Package local implements the local directory (local/dir) and local archive (local/archive)
// artifact repositories behind [artifact.Repository]. They share one record layout (package
// layout), one publication lifecycle, one lock implementation, and one detached-evidence store;
// they differ only in how a payload is stored.
//
// # Directory repositories
//
// A local/dir payload is a tree of ordinary copied files below its resolved location, with the
// distribution documents and manifest.json adjacent at <location>/.atmos/artifact/. Managed
// readers omit that distribution directory when they materialize user content.
//
// # Archive repositories
//
// A local/archive payload is one tar, tgz, or zip file. The repository stores adjacent
// <payload>.metadata.json and <payload>.docs/README.md|CHANGELOG.md sidecars, so metadata and
// documents are readable without opening the archive, while the archive itself embeds portable
// copies for transport. The configured quoted-octal permissions (default 0600) apply to the
// archive file and its sidecars only; they are distinct from member modes inside the archive. An
// explicit request the host cannot enforce fails with an unsupported-capability error instead of
// silently weakening the mode.
//
// # Publication and recovery
//
// A publication follows the contracts sequence: reserve the resolved location and record a pending
// operation journal, mark the reference pending, stage the payload on the same filesystem, write
// the immutable Record, install the payload and sidecars, write the representation, and only then
// flip the reference to completed. Pending references are unavailable to readers and invisible to
// listing. Readers recheck the reference generation after reading from the mutable payload
// location, so replaced bytes are never served under an old digest.
//
// An interrupted or failed force replacement leaves the reference unavailable until a retry with
// the same intent repairs it; there is no rollback. A pending operation of a different intent is
// never stolen: publication fails with a recovery diagnostic naming the operation ID.
//
// # Locks
//
// Lock returns an OS-backed file lock (host scope only) with an ownership token. It is held for
// the reference and, during publication, for the resolved location. The OS releases the lock when
// its owner exits. Acquisition is cancel-safe: a canceled wait leaks no ownership.
//
// # Signatures
//
// Signature evidence is stored as detached files below the reserved namespace and is bound to the
// canonical Record digest. Adding evidence never changes the Record, the reference, or the
// archive: evidence added after publication does not claim that the original archive bytes were
// signed, only the Record that describes them. Cryptographic verification is delegated to an
// injected [artifact.Verifier]; without one, the repository advertises no evidence formats.
package local
