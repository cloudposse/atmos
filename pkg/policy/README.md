# Policy evaluation scaffold

This is an experimental Go API and opt-in Terraform saved-plan integration, not a complete
security boundary. It implements the first slices of [the Policies PRD](../../docs/prd/policies.md).
The narrow directory manifest and command gate are documented in
[Saved Plan Policies](../../website/docs/policies/terraform-plan.mdx).

See the [contributor guide](../../docs/policy-engine.md) for a runnable Go example,
the CEL input reference, decision semantics, affected inventories, and adapter integration guidance.
The [capability reference](../../docs/policy-capabilities.md) describes implemented language,
result-contract, and integration boundaries without treating supplied-data tests as working CI support.

## Implemented

- An instance-local `Engine` with explicit language registration through `Evaluator`.
  `NewEngine()` installs CEL and restricted Starlark. No engines register themselves globally.
- Explicit input-kind dispatch. An `atmos.affected` rule cannot run on an `operation`
  request, and evaluating a rule never computes affected data or loads configuration.
- Materialized JSON inputs and detached copies per evaluator. No lazy configuration
  resolution, process, network, filesystem, clock, or automation functions are exposed.
- Boolean predicates selecting typed effects. False is `not_applicable`; true selects
  denial, approval requirements, hook requirements, or allowance. All findings survive
  aggregation. Precedence is error, deny, awaiting approval, allow, not applicable.
- Provider-native CI envelopes: provider and versioned schema identifier, independent
  metadata/event/fact datasets, availability, completeness, and collection provenance.
  Native JSON fields are retained. Required datasets are checked before the predicate;
  known-empty data is distinct from unavailable, incomplete, or absent data.
- Affected payload contracts for files, components, instances, stacks, and structured
  fields. Before/after objects stay separate; presence flags distinguish absent fields
  from explicit null. Explicit completeness and all inventory arrays are required.
  The caller must actually establish completeness; the engine cannot verify a claim.
- CEL cost bounds and Starlark step bounds, source-size limits, cancellation, and integer-preserving JSON decoding.
- Strict native Terraform plan validation, `LoadPlanDirectory` compatibility API, and `LoadDirectory`
  for mixed inline CEL/Starlark plan/operation/affected bundles with governing hook definitions/tool pins.
- `planfile.Guard` binds evaluation to private binary plan artifacts. Terraform commands opt in
  through `policy.d/` or `ATMOS_POLICY_DIR`, check after plan, and recheck before applying the evaluated copy.
  Gate failures are separate from Terraform exit codes. Implicit apply/destroy are rejected.

## Adapter contract

`Evaluator.Evaluate` accepts one `Evaluation` and returns a predicate match or an
error. Adapters are trusted Go code: they must be concurrency-safe, honor cancellation,
and perform no I/O. The registry is not a sandbox for arbitrary Go plugins.

CEL exposes `input`, `params`, `ci`, and `request` (kind and revisions). Type-specific
root aliases shown in the PRD are not installed yet. Missing CI is null; a supplied
snapshot exposes native values under `ci.metadata.data`, `ci.event.data`, and
`ci.facts.<provider-fact-name>.data`. Missing fields are evaluation errors unless a
policy explicitly handles them. This activation is separate from legacy `when: ci`.

Starlark implements `Evaluator` using shared restricted compilation and read-only value conversion
from `pkg/script/starlark/value`. It exposes `ctx.input`, `ctx.params`, `ctx.ci`, and `ctx.request`;
requires a boolean return; and performs no logging or host calls. Rego and declarative adapters
are not implemented in this package; selecting an unregistered language is an error. Existing Rego/JSON-Schema
`settings.validation` execution remains unchanged.

`pkg/ci.CollectPolicySnapshot` uses the existing provider registry and optional
`PolicySnapshotProvider` capability. GitHub Actions collects allowlisted native environment metadata
and the supplied event file, without API calls or token permissions. Fresh API facts/approvals remain
unimplemented. Snapshots are collected outside evaluation and supplied to plan/lifecycle requests.

## Remaining enforcement boundaries

Callers must supply trusted, resolved policy definitions and distinguish governing,
base, and head revisions. Local discovery and literal instance selectors are implemented; inline
policy inheritance, remote libraries, legacy migration, source trust, general command admission,
and repository-level CI gate orchestration are not wired.
The discovered directory is not automatically trusted and can be bypassed by a caller
who controls the invocation. Protect the workflow/environment and governing files externally.

Approvals always remain outstanding in this slice; there is no review verifier. The core returns
hook requirements; `pkg/hooks` resolves governing definitions and executes them at existing Terraform,
Helmfile, Helm, and Kubernetes lifecycle points. Other component admission remains unwired. An
`allow` decision is authorization only, never evidence of approval or hook completion.
Each finding retains its own requirements, so a caller cannot lose obligations through
deduplication. See [the sample library](../../examples/policies/README.md) for runnable examples,
per-policy tests, and current lifecycle/CI limitations.

Run `go test -race ./pkg/policy/... ./pkg/condition` for the isolated core and compatibility
tests. These tests require no CI credentials, network calls, stacks, or hook execution.
