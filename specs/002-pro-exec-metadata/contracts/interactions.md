# Contract: POST /v1/atmos/exec and POST /v1/atmos/exec/data (Pact interactions 9-15)

**Feature**: 002-pro-exec-metadata
**Date**: 2026-08-11 (revised 2026-08-19 — ExecutionID, Data blob-upload redesign; revised
2026-08-20 — `version` field, per-shape Pact coverage for `describe affected`/`list instances`,
research.md Decisions 22-25)
**Extends**: `specs/001-pact-consumer-contracts/contracts/interactions.md` (interactions 1-8)

These interactions are added to the same local-only Pact consumer suite
(`pkg/pro/consumer_pact_test.go`, `//go:build pact`), regenerating the same
`pacts/atmos-AtmosPro.json` file via `go test -tags pact ./pkg/pro/...`. Consumer/provider
names, matcher strategy, and output location are unchanged from the existing suite (see
`research.md` Decision 9).

**Revision note**: The original single "9. UploadExecMetadata" interaction (multi-chunk
`DataItems`/`BatchID`/`BatchIndex`/`BatchTotal` shape) is replaced by three interactions
below, per the 2026-08-19 "redo batch uploading" clarification (research.md Decision 16):
one `/exec` case with `Data` inline (small record), one `/exec` case with `Data` as a
blob URL (record was at/over 4 MB), and one new `/exec/data` interaction for the blob
upload itself.

---

### 9. UploadExecMetadata — inline `Data` (single mode)

The common case: the whole record (envelope + `metrics` + `data`) is under 4 MB, so `data`
is sent inline as a JSON structure in the same request. No `/exec/data` call precedes this
interaction.

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts execution metadata"` |
| Description | `"a request to upload command-execution metadata with inline data"` |
| Method | `POST` |
| Path | `/api/v1/atmos/exec` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `execution_id` (string, UUID v4), `atmos_pro_run_id` (string), `atmos_version` (string), `atmos_os` (string), `atmos_arch` (string), `command` (string, subcommand path with the `atmos` root stripped, e.g. `"terraform plan"`), `args` (array of string, positional arguments only, e.g. `["cdn"]`, may be empty), `flags` (array of string, masked CLI flags actually passed, canonical long-form names, CLI framework's own flag-iteration order, e.g. `["--stack", "plat-use2-dev", "--upload-status"]`, may be empty), `exit_code` (integer), `git_sha` (string), `repo_url`/`repo_name`/`repo_owner`/`repo_host` (strings), `metrics` (object — see below), `data` (object or array, optional/nullable — see below) |
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

#### `data` object — inline mode (example: `terraform plan`/`apply` structured payload)

Present only for `terraform plan`/`apply` interactions in the pact test; other example
interactions in the same consumer test file omit `data` entirely (`null`) to cover the "no
structured data" case per spec Acceptance Scenario US3.4. Sent in full, inline, since the
whole record fits under 4 MB in this example.

| Field | Type | Matcher |
|-------|------|---------|
| `version` | integer | exact literal `1` |
| `resource_counts` | object `{create, change, replace, destroy}` (integers) | `Like` per field |
| `outputs` | object, values `{value, type, sensitive}` | `Like` per key |
| `warnings` | array of string | `EachLike(...)` |
| `changes` | array of `{action, address}` | `EachLike({action: Like("created"), address: Like("aws_s3_bucket.example")})` |
| `has_changes` | boolean | `Like(true)` |
| `has_errors` | boolean | `Like(false)` |
| `errors` | array of string | `EachLike(...)`, MAY be empty |
| `component` | string, single-component invocations only | `Like("vpc")` |
| `stack` | string, single-component invocations only | `Like("plat-use2-dev")` |

`version` (research.md Decision 24) is an exact-literal match, not `Like`, since it is a fixed
shape identifier the provider branches on — an unexpected value here is itself the signal a
schema mismatch occurred, not noise a matcher should absorb.

`outputs[*].value` is `"<MASKED>"` (a literal string, never the real value) whenever
`outputs[*].sensitive` is `true` (data-model.md Decision 19 / FR-010a) — this is independent
of, and in addition to, the general Gitleaks-pattern secret masking (FR-010) already applied
to the whole `data` blob before upload. The pact test's example fixture MUST include at least
one sensitive output to assert this at the contract level (`Like("<MASKED>")` when
`sensitive` is `Like(true)`).

`has_changes`/`has_errors`/`errors` (data-model.md Decision 20) are sourced from the same
parse result that already produces `resource_counts`/`outputs`/`warnings` — no second parse.
`component`/`stack` (data-model.md Decision 21) are present only for a single-component
invocation and are omitted entirely (not sent as empty strings) when the invoking command's
identity is not known at the point the record is built.

`execution_id` is `Like("b3b1e2b0-....-....-....-............")` (a UUID-v4-shaped string) —
never an exact literal, since it is freshly generated per invocation.

---

### 10. UploadExecMetadata — blob-URL `Data` (batched/out-of-band mode)

The record (envelope + `metrics` + `data`) is at/over 4 MB (e.g. a `terraform plan`
touching thousands of resources, or a large `--affected`/`--all` multi-component run). This
interaction MUST be preceded, in the same test scenario, by interaction 11
(`UploadExecData`) using the same `execution_id`, whose response's `url` becomes this
request's `data` value.

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts execution metadata"` |
| Description | `"a request to upload command-execution metadata with out-of-band data"` |
| Method | `POST` |
| Path | `/api/v1/atmos/exec` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | Same as interaction 9, except `data` is a **JSON string** (a blob URL), not an inline object |
| Response Status | `200` |
| Response Body | `{ "success": true }` |

#### `data` field — blob-URL mode

| Field | Type | Matcher |
|-------|------|---------|
| `data` | string (URL) | `Like("https://blob.vercel-storage.com/atmos-exec/b3b1e2b0.../data.json")` |

The `execution_id` in this request body MUST be identical to the `execution_id` sent in the
corresponding interaction 11 (`UploadExecData`) request, since both requests represent the
same invocation — the Pact test asserts on this literal equality within the scenario, not
via a matcher.

---

### 11. UploadExecData

**New endpoint.** Uploads a command's structured `data` out-of-band, in a single request
(never chunked — the blob store handles arbitrarily large single uploads), when the parent
`ExecutionRecord` would otherwise be at/over 4 MB. Always exactly one request per invocation
that needs it; always precedes the corresponding interaction 10 request in the same
invocation.

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts execution metadata"` |
| Description | `"a request to upload out-of-band command-execution structured data"` |
| Method | `POST` |
| Path | `/api/v1/atmos/exec/data` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `execution_id` (string, UUID v4 — `Like(...)`), `data` (object or array — the structured payload that would otherwise be inline) |
| Response Status | `200` |
| Response Body | `{ "success": true, "url": "<blob-url>" }` |

#### Response body

| Field | Type | Matcher |
|-------|------|---------|
| `success` | boolean | exact literal `true` |
| `url` | string (URL) | `Like("https://blob.vercel-storage.com/atmos-exec/b3b1e2b0.../data.json")` |

---

### 12. UploadExecMetadata — `describe affected`, inline `Data`

Per-shape coverage for `AffectedStacksExecData` (data-model.md, research.md Decision 22/25).
Same request shape as interaction 9 (`command` = `Like("describe affected")`), but `data` is
this shape instead of the terraform one.

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts execution metadata"` |
| Description | `"a request to upload describe-affected execution metadata with inline data"` |
| Method | `POST` |
| Path | `/api/v1/atmos/exec` |
| Request Body Fields | Same envelope fields as interaction 9; `data` per the table below |

#### `data` object — `describe affected` shape

| Field | Type | Matcher |
|-------|------|---------|
| `version` | integer | exact literal `1` |
| `stacks` | array of `schema.Affected` | `EachLike({component: Like("vpc"), component_type: Like("terraform"), stack: Like("plat-use2-dev"), affected: Like("component"), ...})` — at least `component`, `component_type`, `stack`, `affected` asserted per entry |

---

### 13. UploadExecMetadata — `describe affected`, blob-URL `Data`

Pairs with interaction 14 (`UploadExecData`), same `execution_id`, same structure as
interaction 10 but carrying the `describe affected` shape.

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts execution metadata"` |
| Description | `"a request to upload describe-affected execution metadata with out-of-band data"` |
| Method | `POST` |
| Path | `/api/v1/atmos/exec` |
| Request Body Fields | Same as interaction 12, except `data` is a JSON string (URL), not inline |

---

### 14. UploadExecData — `describe affected` payload

Same request/response shape as interaction 11, carrying interaction 13's `data` out-of-band.

| Field | Value |
|-------|-------|
| Method | `POST` |
| Path | `/api/v1/atmos/exec/data` |
| Request Body Fields | `execution_id` (`Like(...)`), `data` (the interaction-12-shaped object) |
| Response Body | `{ "success": true, "url": "<blob-url>" }` |

---

### 15. UploadExecMetadata — `list instances`, inline and blob-URL `Data` (with paired `UploadExecData`)

Per-shape coverage for `InstancesExecData` (data-model.md, research.md Decision 23/25). Two
sub-cases, mirroring interactions 9/10 and 12/13's inline/blob-URL pairing, plus the paired
`UploadExecData` interaction (same shape as 11/14) for the blob-URL sub-case:

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts execution metadata"` |
| Description | `"a request to upload list-instances execution metadata with inline data"` / `"...with out-of-band data"` |
| Method | `POST` |
| Path | `/api/v1/atmos/exec` (`data` inline or as a URL, per sub-case) |
| Request Body Fields | Same envelope fields as interaction 9 (`command` = `Like("list instances")`); `data` per the table below |

#### `data` object — `list instances` shape

| Field | Type | Matcher |
|-------|------|---------|
| `version` | integer | exact literal `1` |
| `instances` | array of `dtos.UploadInstance` | `EachLike({component: Like("vpc"), stack: Like("plat-use2-dev"), component_type: Like("terraform"), settings: Like({})})` |

This shape is only ever present when `--upload` was passed for the invocation — the pact test's
"no structured data" example case (interaction 9's sibling, `data: null`) already covers
`list instances` without `--upload`, so no additional `null`-`data` interaction is needed for
this command specifically.

---

## Validation Rules (these interactions)

| Rule | Detail |
|------|--------|
| Authorization header | MUST be present and match `Bearer <token>` pattern on both `/exec` and `/exec/data` |
| Content-Type header | MUST be `application/json` on both endpoints |
| Response `success` field | MUST be `true` in the 200 response body for both endpoints |
| `data` shape coverage | The contract MUST cover, per FR-005/FR-011: a present-inline-`data` interaction, an absent/`null`-`data` interaction (non-terraform command with no structured-data extension, included in the test suite alongside interaction 9's sibling case), and a blob-URL-`data` interaction pair. Per FR-005a/FR-013 (Assumptions, 2026-08-20 clarification, research.md Decision 25), this coverage MUST be repeated **per structured-`Data` shape**, not only for one representative shape: `TerraformExecData` (9 + 10/11), `AffectedStacksExecData` (12 + 13/14), `InstancesExecData` (15 + its paired `/exec/data` interaction) — 3 shapes × 2 delivery modes = 6 `/exec` interactions total, each blob-URL sub-case constructed directly rather than routed through the real size-threshold decision code |
| `version` field | Every structured `Data` shape's example in this contract MUST include `version` as an exact-literal integer (`1` for every shape today), never `Like()`-matched, since it identifies the shape itself (FR-005a, research.md Decision 24) |
| No truncation | The contract MUST NOT model `data` as truncated or dropped in any interaction — either the full structure is inline (9), or it is fully represented via the out-of-band blob referenced by URL (10+11), never a partial/truncated subset (FR-011) |
| No chunking | Unlike the retired multi-chunk model, no interaction in this contract includes `batch_id`/`batch_index`/`batch_total` fields — `UploadExecData` is always exactly one request (research.md Decision 16) |
| `execution_id` presence | MUST be present (a UUID-v4-shaped string, `Like(...)`) on every `/exec` and `/exec/data` request; MUST be literally identical between a paired interaction 10 + interaction 11 request within the same test scenario |
| Numeric metrics | All `metrics` fields MUST use `Like()` (never exact literals) since resource usage is inherently non-deterministic across machines/runs |
| `command` shape | MUST NOT include the `atmos` root segment (e.g. `Like("terraform plan")`, not `"atmos terraform plan"`) |
| `args`/`flags` separation | `args` MUST contain only positional arguments (e.g. `EachLike("cdn")`); `flags` MUST contain only CLI flags, canonical long-form (e.g. `EachLike("--stack")`, `EachLike("plat-use2-dev")`); the two MUST NOT be combined into a single array in any example interaction |
