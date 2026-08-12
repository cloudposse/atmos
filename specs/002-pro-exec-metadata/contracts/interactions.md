# Contract: POST /v1/atmos/exec (9th Pact interaction)

**Feature**: 002-pro-exec-metadata
**Date**: 2026-08-11
**Extends**: `specs/001-pact-consumer-contracts/contracts/interactions.md` (interactions 1–8)

This interaction is added to the same local-only Pact consumer suite
(`pkg/pro/consumer_pact_test.go`, `//go:build pact`), regenerating the same
`pacts/atmos-AtmosPro.json` file (now with 9 interactions) via
`go test -tags pact ./pkg/pro/...`. Consumer/provider names, matcher strategy, and
output location are unchanged from the existing suite (see `research.md` Decision 9).

---

### 9. UploadExecMetadata

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts execution metadata"` |
| Description | `"a request to upload command-execution metadata"` |
| Method | `POST` |
| Path | `/api/v1/atmos/exec` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `atmos_pro_run_id` (string), `atmos_version` (string), `atmos_os` (string), `atmos_arch` (string), `command` (string), `args` (array of string, may be empty), `exit_code` (integer), `git_sha` (string), `repo_url`/`repo_name`/`repo_owner`/`repo_host` (strings), `metrics` (object — see below), `data` (object, optional/nullable), `data_items` (array of object, optional — see below), `batch_id` (string, optional), `batch_index` (integer, optional), `batch_total` (integer, optional) |
| Response Status | `200` |
| Response Body | `{ "success": true }` |

#### `metrics` object

| Field | Type | Matcher |
|-------|------|---------|
| `wall_time_ms` | integer | `Like(1234)` |
| `user_cpu_time_ms` | integer | `Like(800)` |
| `system_cpu_time_ms` | integer | `Like(150)` |
| `max_rss_bytes` | integer, optional | `Like(52428800)` |
| `minor_page_faults` | integer, optional | `Like(120)` |
| `major_page_faults` | integer, optional | `Like(0)` |
| `in_block_ops` | integer, optional | `Like(4)` |
| `out_block_ops` | integer, optional | `Like(2)` |
| `vol_ctx_switches` | integer, optional | `Like(30)` |
| `invol_ctx_switches` | integer, optional | `Like(5)` |

#### `data` object (example: `terraform plan`/`apply` structured summary payload)

Present only for `terraform plan`/`apply` interactions in the pact test; other
example interactions in the same consumer test file may omit `data` entirely
(`null`) to cover the "no structured data" case per spec Acceptance Scenario
US3.4. Always sent in full — never split across requests.

| Field | Type | Matcher |
|-------|------|---------|
| `resource_counts` | object `{create, change, replace, destroy}` (integers) | `Like` per field |
| `outputs` | object, values `{value, type, sensitive}` | `Like` per key |
| `warnings` | array of string | `EachLike(...)` |

#### `data_items` array (example: `terraform plan`/`apply` per-resource change list)

The potentially large, chunkable portion of the structured payload — one entry per
created/updated/deleted/replaced/moved/imported resource. Present only for
`terraform plan`/`apply` interactions; absent/`null` for commands with no bulk
structured data.

| Field | Type | Matcher |
|-------|------|---------|
| `action` | string, one of `created`/`updated`/`replaced`/`deleted`/`moved`/`imported` | `Like("created")` |
| `address` | string | `Like("aws_s3_bucket.example")` |

#### Batch correlation fields (`batch_id`, `batch_index`, `batch_total`)

Present only when `data_items` was split across multiple requests because the full
`ExecutionRecord` (envelope + `metrics` + `data` + that chunk's `data_items`) would
otherwise exceed the payload size limit — mirrors the existing
`UploadAffectedStacks`/`UploadInstances` batch-correlation shape
(`pkg/pro/chunked_upload.go`). Absent/`null` when the whole record fit in a single
request (the common case for most commands, and for small plans/applies). One example
interaction in the pact test covers a 2-chunk `terraform plan` to validate the shape;
the base envelope, `metrics`, and `data` fields are identical (repeated) on both
chunk requests, only `data_items`/`batch_index` differ.

| Field | Type | Matcher |
|-------|------|---------|
| `batch_id` | string (uuid) | `Like("b3b1...-uuid")` |
| `batch_index` | integer, 0-based | `Like(0)` |
| `batch_total` | integer | `Like(2)` |

---

## Validation Rules (this interaction)

| Rule | Detail |
|------|--------|
| Authorization header | MUST be present and match `Bearer <token>` pattern |
| Content-Type header | MUST be `application/json` |
| Response `success` field | MUST be `true` in the 200 response body |
| `data`/`data_items` nullability | The contract MUST cover a present-`data`/`data_items` interaction (terraform plan), an absent/`null` interaction (non-terraform command), and a chunked (`batch_id`/`batch_index`/`batch_total` present) interaction, since FR-005 and FR-011 require all three to be valid |
| No truncation | `data_items` entries in the contract MUST NOT be marked/expected as truncated or dropped — the contract only ever models full delivery, split across requests when large, never partial data (FR-011) |
| Numeric metrics | All `metrics` fields MUST use `Like()` (never exact literals) since resource usage is inherently non-deterministic across machines/runs |
