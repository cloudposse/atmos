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
| Request Body Fields | `atmos_pro_run_id` (string), `atmos_version` (string), `atmos_os` (string), `atmos_arch` (string), `command` (string), `args` (array of string, may be empty), `exit_code` (integer), `git_sha` (string), `repo_url`/`repo_name`/`repo_owner`/`repo_host` (strings), `metrics` (object — see below), `data` (object, optional/nullable) |
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

#### `data` object (example: `terraform plan`/`apply` structured payload)

Present only for `terraform plan`/`apply` interactions in the pact test; other
example interactions in the same consumer test file may omit `data` entirely
(`null`) to cover the "no structured data" case per spec Acceptance Scenario
US3.4.

| Field | Type | Matcher |
|-------|------|---------|
| `resource_counts` | object `{create, change, replace, destroy}` (integers) | `Like` per field |
| `created_resources` | array of string | `EachLike("aws_s3_bucket.example")` |
| `updated_resources` | array of string | `EachLike(...)` |
| `replaced_resources` | array of string | `EachLike(...)` |
| `deleted_resources` | array of string | `EachLike(...)` |
| `outputs` | object, values `{value, type, sensitive}` | `Like` per key |
| `warnings` | array of string | `EachLike(...)` |

---

## Validation Rules (this interaction)

| Rule | Detail |
|------|--------|
| Authorization header | MUST be present and match `Bearer <token>` pattern |
| Content-Type header | MUST be `application/json` |
| Response `success` field | MUST be `true` in the 200 response body |
| `data` nullability | The contract MUST cover both a present-`data` interaction (terraform plan) and an absent/`null`-`data` interaction (non-terraform command), since FR-005 requires both to be valid |
| Numeric metrics | All `metrics` fields MUST use `Like()` (never exact literals) since resource usage is inherently non-deterministic across machines/runs |
