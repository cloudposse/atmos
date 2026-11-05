// Package s3 is the AWS SDK v2 implementation of [blob.Store], plus the shared S3 client
// construction used by the legacy CI artifact adapter.
//
// Writes stream: a body that fits one part is sent with a single PutObject, a larger or
// unknown-length body is sent as a multipart upload whose parts are uploaded by a bounded worker
// pool (default 4) from a bounded buffer pool, so memory use is independent of object size. A
// multipart upload owned by a failed or canceled operation is aborted. Conditional writes use
// If-None-Match and If-Match; conditional requests are never retried by the SDK so a lost response
// surfaces as an ambiguous error instead of a misleading precondition failure. An endpoint that
// cannot enforce conditions fails the operation with a clear error, and an optional probe detects
// endpoints that silently ignore the headers.
//
// Credentials are resolved lazily on first use. The generic store fails closed: a configured
// identity that cannot be resolved is an error, never a silent fall back to ambient credentials.
package s3
