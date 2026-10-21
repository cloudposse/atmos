# Policy Capabilities

Reference for implemented policy capabilities. The Go evaluator accepts supplied snapshots;
command integrations collect specific inputs and enforce their decisions.

## Command integrations

| Input | Collector | Enforcement |
|---|---|---|
| `file` | Literal YAML/JSON documents selected by globs | `atmos policy check`, with per-file JSON diagnostics |
| `function` | YAML/template function name, engine, and available provenance, without secret arguments | Before config/stack/registry/template handler execution |
| `source` | Normalized destination, scheme, hostname, canonical path and provenance | Before imports/cache reuse, downloads, includes, and guarded template I/O |
| `command` | Canonical runnable CLI command path | Before argument validation and pre-run hooks |
| `auth` | Resolved provider and complete identity chain, without credentials | Before authentication and credential cache access |
| `component.command` | Shared stack resolution, inherited literal selector metadata, capability list, captured agent names | Before component templates/functions and tool execution |
| `terraform.plan` | Native JSON extracted from a private saved Terraform-compatible plan | After successful planning and before applying the evaluated private copy |
| `operation` | Terraform, Helmfile, Helm, and Kubernetes lifecycle metadata | Denial and governing hook requirements at the current lifecycle event |
| `atmos.affected` | Unfiltered static inventories from clean Git worktrees, file changes, and structured differences | Denial before affected output filtering or scheduling |
| GitHub `ci` context | Allowlisted environment variables and supplied event file through Native CI | `required_ci` validates provider, schema, availability, and completeness before a predicate |

Rules load from `policy.d/`, overridden by `ATMOS_POLICY_DIR`. See [Configuration](https://atmos.tools/policies/configuration),
[Inputs](https://atmos.tools/policies/inputs), and [CI Context](https://atmos.tools/policies/ci) for the public contracts.

Plan and lifecycle rules support literal instance selectors over resolved metadata. Affected rules
inspect separate before/after values. The affected collector uses active discovery settings on both
revisions, not independent historical discovery; protect those settings. See
[Affected Changes](https://atmos.tools/policies/affected) for the collection scope and strict failures.

## CEL operations

The adapter enables the standard CEL environment with no custom extension libraries.

| Operation | Supported forms |
|---|---|
| Boolean logic | `&&`, `\|\|`, `!` |
| Comparisons | Equality, ordering, membership with `in` |
| Collections | Map/list access, `exists`, `all`, `filter`, `map`, `size` |
| Strings | Standard CEL methods, including `startsWith` and regular-expression matching |
| Time | Timestamp operations over supplied values; no clock access |
| Presence | `has(object.field)`; explicit checks for null and unknown values |

Expressions are boolean predicates. Matching selects the rule's static effects; it does not
compute arbitrary findings, hook payloads, or recipients. See [Inputs and Expressions](https://atmos.tools/policies/inputs)
for limits and error behavior.

## Core and executor responsibilities

| Layer | Implemented responsibility |
|---|---|
| `pkg/policy` | Input validation, CEL/Starlark/OPA/JSON Schema evaluation, structured diagnostics, rule dispatch, decision precedence, and typed obligations |
| `pkg/policy/planfile` | Private artifact extraction, plan validation, and binding evaluation to saved-plan apply |
| `pkg/ci` | Provider detection and optional snapshot collection |
| `pkg/hooks` | Governing hook resolution, execution, skip protection, and failure propagation |

The pure core returns outstanding approval requirements. Native CI verifies them with fresh,
paginated GitHub reviews bound to the current PR and checkout. Manifests accept `require_approval`
with explicit reviewer logins and a minimum count; no organization-team lookup is performed.
See [approval verification](https://atmos.tools/policies/approvals).

## Starlark operations

Restricted Starlark supports loops, comprehensions, helper functions, recursion, and custom
aggregation over read-only snapshots. The function body must return a boolean; findings, hook
payloads, and recipients remain static effects. It shares the existing configuration evaluator's
compiler and value conversion, without the automation host or lazy configuration reads.
Source is limited to 64 KiB and execution to 100,000 interpreter steps; this is not a memory sandbox.
See [Starlark policies](https://atmos.tools/policies/starlark) for bindings and restrictions.

## Enforcement boundary

These gates govern the current Atmos invocation. They do not enforce cluster admission or
intercept another client calling a platform API. Required hooks run external tools through the
existing hook executor; their success cannot override another policy's denial.

Protect policy sources, workflow configuration, binaries, and credentials. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement) and
[the engine API](policy-engine.md).

Offline authoring commands: `atmos policy check`, `list`, `schema <input-kind>`, and `test <cases.json>`.
Operational `--explain` reports reached gates and decisions on stderr without changing enforcement.
See [source boundaries](https://atmos.tools/policies/sources) for guarded transport limitations.

## Validation engines

OPA evaluates boolean predicates or violation collections using embedded Rego. JSON Schema selects
effects when validation fails. Both retain structured diagnostics and accept existing input kinds,
including Terraform plans. File checks also support any JSON value as input. Definitions and local
resources are loaded before pure evaluation. See [OPA, JSON Schema, and file checks](../website/docs/policies/validation-engines.mdx)
for configuration and runtime limits.
