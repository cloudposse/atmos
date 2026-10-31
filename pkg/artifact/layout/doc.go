// Package layout defines the backend-neutral record layout shared by the local directory, local
// archive, and S3 artifact repositories. It is pure data and logic: it performs no filesystem,
// network, or AWS work, so every backend stores the same documents and agrees on names, paths,
// state transitions, and collision rules.
//
// Below a repository root (a directory, or a bucket prefix) the managed records live in the
// reserved ".atmos-artifacts/v1/" namespace:
//
//	.atmos-artifacts/v1/refs/<reference-key>.json            stable-root reference record
//	.atmos-artifacts/v1/records/<record-digest-hex>.json     immutable canonical Record
//	.atmos-artifacts/v1/representations/<reference-key>.json mutable storage description
//	.atmos-artifacts/v1/operations/<operation-id>.json       publication journal (recovery only)
//	.atmos-artifacts/v1/signatures/<kind>/<subject-hex>/<evidence-hex>.json detached evidence
//	<resolved-payload-path>                                  payload (outside the namespace)
//
// The reference key is the SHA-256 of "name:version", so names containing slashes can never
// collide with control paths. A reference record binds a name and version to the digest of its
// immutable Record and carries the pending or completed publication state. The Record never holds
// a mutable storage locator: the resolved payload location is rendered from the repository
// key_pattern exactly once, at publication, from the captured creation time, and is persisted in
// the representation record. Reads, deletes, and retries use the persisted location and never
// render the pattern again, so a date rollover cannot move an artifact.
//
// Publication order: write the pending operation journal and mark the reference pending, stage
// and install the payload, write the Record, the representation, and the adjacent documents, and
// only then flip the reference to completed. Pending references are unavailable to readers and
// absent from search results; readers recheck the reference generation after reading from a
// mutable payload location.
package layout
