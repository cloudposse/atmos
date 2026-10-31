// Package blob is the transport-only object I/O layer shared by artifact repositories and the
// legacy CI artifact adapters.
//
// A [Store] reads and writes opaque byte streams addressed by slash-separated keys. It knows
// nothing about artifact records, CI metadata, signatures, or key layouts: callers own those and
// pass fully formed keys. The package defines streaming put/get, head, conditional write and
// delete (if-absent and if-match), prefix listing with pagination, object tags, user metadata,
// capability reporting, and a small set of classified errors.
//
// Conditional operations carry an ETag. ETags are opaque concurrency tokens and are never payload
// checksums. A store that cannot enforce a requested condition fails that operation with
// [errors.ErrBlobConditionalUnsupported] instead of silently overwriting. A mutation whose result
// was not observed (for example a connection reset after the request was sent) fails with an error
// for which [IsAmbiguous] is true and whose Temporary method reports true; callers reconcile by
// reading the object back.
//
// Implementations live in subpackages: blob/local (filesystem) and blob/s3 (AWS SDK v2). The
// blobtest subpackage holds the shared conformance suite every implementation must pass.
//
// This package must not import pkg/ci or any CI types. Dependencies point from CI adapters to blob.
package blob
