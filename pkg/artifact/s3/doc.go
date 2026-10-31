// Package s3 implements the aws/s3 artifact repository behind [artifact.Repository]. It stores the
// same record layout as the local repositories (package layout) and follows the same publication
// lifecycle, over a transport-only [blob.Store]. Production repositories build the AWS SDK store
// lazily from configuration; tests inject any store, typically the in-process fake S3 server.
//
// # Object layout
//
// Below the configured bucket and prefix the managed records live in the reserved
// ".atmos-artifacts/v1/" namespace: reference, Record, representation, operation journal, detached
// evidence, and lock-claim objects. The payload lives outside it at the location rendered once, at
// publication, from the key_pattern.
//
//   - packaging none stores one object per file below <location>/, with the distribution documents
//     and manifest.json adjacent at <location>/.atmos/artifact/. Directories, symlinks, and modes
//     are not S3 concepts: the Record inventory carries them and materialization restores them.
//   - packaging tar, tgz, or zip stores one archive object at <location> with adjacent
//     <location>.metadata.json and <location>.docs/README.md|CHANGELOG.md objects.
//
// The configured S3 object tags are written on every object a publication creates (payload,
// documents, records, journals, claims, evidence). Portable labels stay in the Record and are
// never copied into tags.
//
// # Publication and conditional writes
//
// A publication reserves its location and records a pending operation journal, marks the
// reference pending, uploads the payload and documents, writes the immutable Record and the
// representation, and only then flips the reference to completed. Reference ownership is a
// conditional write: If-None-Match for a first publication and If-Match with the observed ETag for
// every later state change, so a concurrent writer loses with a conflict instead of overwriting.
// ETags are concurrency tokens only; every consumed byte is verified against the digest in the
// Record. A response that was lost after the request was sent is reconciled by reading the
// reference back, never assumed to have failed. An endpoint that cannot enforce a required
// condition fails the operation with an unsupported-capability error and never overwrites.
// Streaming uploads use bounded multipart transfers and abort the upload on cancellation.
//
// An interrupted or failed force replacement leaves the reference pending, and so unavailable to
// readers, until a retry with the same intent repairs it; there is no rollback. A pending
// operation of a different intent is never stolen.
//
// # Locks and recovery
//
// Lock provides repository-scope mutation coordination through owner claim objects under the
// managed namespace. A claim is created with If-None-Match, carries an ownership token, and is
// released with a token check and a conditional delete. V1 claims never expire and are never
// stolen: a paused or crashed writer cannot become an unfenced stale owner. S3 Object Lock
// retention is unrelated and is not used.
//
// A writer that crashes while holding a claim leaves it behind. Every later attempt then fails
// with a recovery diagnostic that names the claim object, its resource, the owner, and the
// operation. The operator confirms the owning writer has stopped and deletes that one claim object
// (for example with the AWS console or "aws s3 rm"); nothing else needs to change. The brief
// repository-wide claim taken while a location is reserved is recovered the same way.
//
// # Signatures
//
// Evidence is stored as digest-named objects below the signature directory of the canonical Record
// digest. Adding evidence never changes the Record, the reference, or the payload. Cryptographic
// verification is delegated to an injected [artifact.Verifier]; without one the repository
// advertises no evidence formats.
//
// # Identity
//
// A configured identity is resolved lazily on first use, through the injected resolver, and an
// identity that cannot be resolved fails closed: there is no fallback to ambient credentials.
// Construction, capability description, and dry runs perform no network access.
package s3
