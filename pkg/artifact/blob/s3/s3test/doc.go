// Package s3test is an in-process, S3-compatible HTTP server for tests. It speaks the S3 REST
// protocol over a real listener, so the AWS SDK exercises its real request and response handling
// against it, and it models the behavior publication code depends on rather than always succeeding:
//
//   - ListObjectsV2 pagination (MaxKeys, ContinuationToken, delimiter roll-up, a configurable page cap).
//   - Conditional PUT, multipart completion, and DELETE with If-None-Match and If-Match, answering
//     412 on a lost race and 404 for If-Match on a missing key, plus endpoints that ignore the
//     headers or answer 501.
//   - ETags (MD5 for single puts, MD5-of-MD5s with a part count for multipart uploads).
//   - Multipart create, upload-part, complete, and abort, with tracking of open and aborted uploads
//     and of concurrent part uploads.
//   - Object tags (the x-amz-tagging header and GetObjectTagging) and user metadata.
//   - Byte ranges, HEAD, and zero-length objects.
//   - Fault injection: fail after N requests, drop the response after the mutation committed
//     (response ambiguity), drop it before the commit, and delay.
//
// Every request is recorded so tests can assert ordering and headers. Signatures are not verified.
package s3test
