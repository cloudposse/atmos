# Fix: Pack Atmos Pro uploads by bytes and recover from HTTP 413

**Date:** 2026-10-07

## Summary

Affected-stack and instances uploads now pack components by serialized size and
restart rejected uploads with a smaller byte budget. Large repositories with
uneven component sizes no longer require manual payload-budget tuning.

## Context

The previous chunker divided the byte budget by the average item size, then split
items into fixed-count chunks. Heavy items concentrated near the start could
produce a request above the server limit. The 4 MiB default also left little
headroom below the gateway's approximately 4.5 MB request-body limit.

Gateway rejections can arrive as plain-text HTTP 413 responses before the API
handler runs. The client classified those bodies as unexpected response formats,
obscuring the size problem. HTTP retries correctly excluded 413, but there was no
whole-upload recovery.

## Changes

- Measure each item once for packing and reuse its size across recovery attempts.
  Preserve order and account for metadata, array punctuation, and batch fields.
  Determine the complete chunk list before sending the first batch request.
- Default to 3 MiB (3,145,728 bytes). Explicit configuration remains the initial
  budget; recovery never changes the client configuration. Successful small
  uploads keep their single request without batch fields.
- Classify HTTP 413 as `APIError` wrapping `ErrPayloadTooLarge`, regardless of the
  response body, including the separate exec-data response parser. Keep HTTP-level
  413 retries disabled.
- Restart the whole upload with a fresh batch UUID after a multi-item rejection,
  halving the budget up to three times with a 64 KiB floor. A configured budget
  below the floor is never raised. Debug logs report budget reductions and batch
  creation; abandoned batches never resume.
- Send oversized items individually. A rejected singleton fails immediately with
  its stack, component, serialized byte count, and guidance to reduce large
  settings or other uploaded data.
- Keep request DTOs, batch fields, and Pact consumer contracts unchanged. Update
  the configuration documentation to describe the new default and recovery.

## Validation

- Before the implementation change, `TestSendChunked_SkewedSizes` failed: a
  10,000-byte budget produced a 10,771-byte request. It passes with byte packing.
- `go test ./pkg/pro/... ./errors/... -count=1 -timeout 5m` passed.
- Local `httptest` endpoints exercise both upload APIs with plain-text gateway
  rejections, initial unbatched rejection, rejection after an accepted chunk,
  complete ordered final reassembly, fresh UUIDs, configured-budget preservation,
  oversized singletons, three-reduction exhaustion, and the budget floor.
- Each API uploads 600 components with front-loaded heavy settings against a
  4,500,000-byte mock limit using default configuration. Every multi-item request
  is checked against the active byte budget, including during recovery.
- Focused chunking, endpoint, and HTTP 413 tests passed under the race detector.
- `go tool mage lint:changed` passed with zero issues.
- `go test -tags pact ./pkg/pro/... -run TestPact -count=1 -timeout 2m` passed
  with the pinned native Pact library installed locally and supplied through
  `CGO_LDFLAGS`. Generated contracts are unchanged after restoring the generator's
  omitted final newline.
- The repository-wide `go test -short ./... -timeout 5m` run failed; the `tests`
  package exceeded its five-minute timeout. The focused suites above passed.

## Follow-ups

[#3313](https://github.com/cloudposse/atmos/issues/3313) tracks the exec-data protocol
compatibility gap documented in the [engineering support artifact](https://atmos-pro.com/artifacts/ca_fsZlCPo0Xd1-j7LDjISkv/pdf).
`UploadExecData` still sends one complete JSON body through the API function. The
server's existing `batch`/`chunk`/`chunk_total` assembler concatenates top-level
arrays but rejects multiple object or string parts; CLI execution data uses object
wrappers. Lossless multipart object uploads require a coordinated server extension.
This PR improves exec-data 413 reporting but does not implement that extension.
The shared 3 MiB default also lowers the existing exec-metadata offload threshold.
