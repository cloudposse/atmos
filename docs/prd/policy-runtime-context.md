# PRD: Shared Policy Runtime Context

## Problem and scope

Policies need facts about the caller, executable, host, clock, and CI evidence, not only
component or Terraform documents. Implement one common surface across CEL, restricted
Starlark, OPA, and JSON Schema. This is native Atmos policy functionality for the next
release; do not market legacy `settings.validation` as this feature or publish a 1.233.0
social example. Publish the replacement only after verifying the released binary.

## Shared surface

The engine materializes one runtime snapshot per evaluation and isolates each adapter's
copy. `ContextProvider` and `FactCollector` are injectable host interfaces; adapters remain
pure, bounded, and free of I/O. Offline `policy test` uses fixture context, never live host
facts. Request documents cannot overwrite context fields. At CLI admission, capture process
environment before configuration evaluation and reuse it through the invocation.

| Field | Contract |
| --- | --- |
| `env` | Process environment string map; absent variables remain absent. Contains secrets; never log the snapshot. |
| `atmos.version` | Running binary's version, including development/prerelease identifiers. |
| `atmos.fips` | Active `crypto/fips140.Enabled()` state, matching `atmos version`. |
| `runtime.os`, `runtime.arch` | Executable OS/architecture; emulated architecture may differ from physical hardware. |
| `runtime.user.uid`, `.gid`, `.username` | Effective OS identity; unavailable fields are null. Numeric IDs are unsupported on Windows. |
| `runtime.memory.available_bytes`, `.total_bytes`, `.scope` | Host capacity, constrained by readable conventional Linux cgroup limits; scope identifies host/container. |
| `runtime.disk.path`, `.available_bytes`, `.total_bytes` | Current working directory's filesystem and caller-available capacity; does not imply space on component or temp volumes. |
| `time.now`, `.date`, `.weekday`, `.timezone` | One captured UTC instant, RFC3339 timestamp, ISO date, English weekday, UTC timezone. |
| `ci.labels` | Sorted labels from a complete supported PR event; null for missing/unsupported evidence, empty list for known absence. |
| `ci.approvers` | Sorted normalized logins from fresh, complete, checkout-bound reviews; excludes author and stale/dismissed approvals. |
| `checks` | Named boolean results from shared, engine-independent operations. |

Memory/disk values are observations, not reservations. Null differs from zero. Cgroup
collection is best effort for conventional v1/v2 mounts; it must report host scope when
no finite usable container limit is discovered. A policy needing a container guarantee
must check scope. Disk location is explicit and does not silently assume a component path
before stack resolution. Environment markers are forgeable guardrails, not authentication.

## Language bindings and compatibility

CEL uses `env`, `atmos`, `runtime`, `time`, `ci`, and `checks`. Starlark uses the existing
`ctx` surface, e.g. `ctx.env["CLAUDECODE"]`. OPA uses `data.atmos_context` for metadata and
retains its original `input` document. JSON Schema defaults to its original input and opts
into the full envelope with `schema.target: context`. That envelope additionally contains
`input`, `params`, and `request`. Standard JSON Schema format assertions are enabled.

Missing required data fails closed. All engines see the same values/types and shared check
results; no engine-specific environment readers, clock access, or date helpers are needed.
Preserve existing boolean/diagnostic Rego behavior, schema violation effects, and selectors.

## Common operations

Each rule may declare `checks` keyed by identifier. A check chooses `kind`, a JSON-pointer
`path` into the envelope, and static operation parameters. Checks cannot reference other
checks. A missing/null source is an input-unavailable error; a present value that does not
satisfy the constraint yields false. Invalid configuration is rejected while loading.

- `semver`: strict version parsing (optional `v` prefix) and a semantic `constraint`, using
  the existing Masterminds constraint implementation. Development versions return false;
  prerelease treatment follows the constraint library, never lexicographic ordering.
- `date`: `format: date`, `date-time` (default), or `time`; validate actual calendar values
  and optional `start`/`end` bounds. Date is ISO YYYY-MM-DD, date-time is RFC3339, time is HH:MM:SS.
- `schedule`: timestamp defaults to `/time/now`; optional IANA `timezone` (UTC default),
  English `weekdays`, timestamp bounds, and `start_time`/`end_time` in HH:MM. Start is
  inclusive, end exclusive. Split overnight windows into two named checks and combine
  results in the engine. Embedded timezone data makes behavior portable. Convert an
  unambiguous timestamp into local time; DST gaps have no instant and repeated hours
  match their local wall-clock window on both occurrences.

Shared checks are predicates, not implicit effects. The engine rule chooses how to combine
booleans and whether to deny, require hooks, or request approval. JSON Schema can require
`checks.<name>` to be true and select effects on violations.

## CI evidence

Preserve native `ci.metadata`, `ci.event`, and `ci.facts` with provider/schema, status,
completeness, and provenance. GitHub Actions is implemented first. Other providers leave
unsupported summaries null, never invent facts. Local execution keeps `ci: null`; CEL uses
`ci != null`, not a boolean/object overload of `ci`. This change does not add `when`.

Approver expression access requires `required_ci` with `dataset: facts`, `fact: approvers`.
Operational Native CI gates collect only when an applicable selected rule explicitly requests the fact,
using `pull-requests: read`. Reuse the approval resolver's paginated reviews, open PR and
checkout checks, before/after revision checks, latest decisive review handling, and author
exclusion. No comment, label, environment variable, or webhook alone grants approval.
`action: require_approval` remains the normal way to request approval obligations. Offline file
checks can read the local CI snapshot but never fetch reviews; required approver evidence is
unavailable there. Use `policy test` fixtures to test approver rules offline.

## Acceptance criteria

- One fixture produces equivalent decisions across all four actual adapters.
- Context is collected once per evaluation and cannot be modified by adapters or payloads.
- The agent-production library policy uses `env` and is exercised through real admission.
- UID/GID/null capacity, version ordering/prerelease, invalid dates, inclusive/exclusive
  bounds, timezone/DST, missing evidence, and cancellation have focused tests.
- CI labels preserve unknown versus empty; approvers use verified current-head evidence.
- Offline fixtures can inject context and do not collect host/CI facts.
- Existing direct-input OPA and JSON Schema policies remain compatible.
- Reference documentation, manifest schema, CLI checks, PRD status, and tests agree.
