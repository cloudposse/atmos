# PRD: Policies

## Executive Summary

Atmos needs a **general policy engine** — not just a stack/component validator or a security bolt-on.
"No remote `!exec`," "require review when protected labels change," "always run TFLint after plan,"
and "send an HTTP callback after apply" belong to one product: policies decide what is permitted,
what approval is required, and what work must run at a particular event or lifecycle point.

The existing per-component `settings.validation` mechanism (OPA/Rego + JSON-Schema) runs *after*
config is rendered and can only check the resolved result. The first opt-in saved-plan integration
adds plan-time checks; function/source security gating and general operational guardrails remain gaps.

This PRD defines **Atmos Policies**: one engine, one authoring surface, a **taxonomy of policy types**,
with a clean split between open-source CLI/Native CI execution and Atmos Pro. Policies are
authored declaratively, in CEL, OPA/Rego, JSON Schema, or restricted Starlark; made **reusable** through a parameterized policy
library (define once, instantiate many); attached inline on components/stacks *and* in a trusted
out-of-band directory; and rolled out in phases (warn → opt-in strict → secure-by-default) so nothing
breaks on upgrade. Explicitly required CI gates and hooks are enforced from their first opt-in; the
advisory rollout does not silently make a configured approval requirement optional.

Repository and CI-event policies work **without stacks, components, Terraform, or `atmos.yaml`**.
Native CI abstracts collection and reporting, not every provider's vocabulary: GitHub Actions, GitLab,
and Azure DevOps adapters own their metadata schemas and expose native payloads when available.
GitHub is the first concrete integration; GitLab and Azure DevOps are extension targets, not promised
implementations in this release. Open-source GitHub features must work with **`GITHUB_TOKEN` alone**,
without a PAT or separately installed GitHub App. Custom App integrations, organization identity
enrichment, and durable webhook orchestration belong to Atmos Pro.

**Status:** this is a proposed design, not documentation of a fully shipped policy engine.
`pkg/policy` provides the evaluator scaffold. The first command integration described below is an
opt-in saved-Terraform-plan gate using CEL/Starlark and `policy.d/` (overridden by `ATMOS_POLICY_DIR`). The directory also
supports operation rules selecting existing hooks at Terraform, Helmfile, Helm, and Kubernetes
lifecycle points. Native CI supplies GitHub Actions environment/event snapshots through `pkg/ci`.
Instance selectors attach plan/lifecycle rules using resolved tags, labels, names, stacks, and types.
An `atmos.affected` gate compares unfiltered static inventories within the active discovery scope
before affected results are consumed. Independent historical discovery is not implemented; see below.
CLI command admission and YAML function admission now run through `pkg/policy/admission`;
`component.command` gates shared stack resolution using inherited literal metadata before
component templates/functions and tool execution. The CLI pins governing definitions before config loading.
Native CI verifies `require_approval` using fresh paginated GitHub reviews, named reviewers,
current PR/checkout revisions, and `pull-requests: read`. Review collection is outside pure evaluation.
Go-template admission and source destination checks now cover guarded imports/downloads, includes,
and template file/DNS/HTTP access. Source policies are opt-in; guarded transport limitations are
documented in the public source-policy reference. Offline `policy check`, `list`, `schema`, and `test`
and operational `--explain` diagnostics are implemented. `policy check` validates configuration;
it does not introduce a repository-only enforcement event. Inline/remote policy attachment inheritance,
trusted-source distribution remain proposed. Shared runtime facts are implemented as described in the runtime-context PRD. Component integrations have different
capabilities; none may pretend to supply a Terraform plan merely to share the engine.
The native plugin pilot also adds `on: plugin` admission through this engine: company registry/package restrictions apply before OCI access, installation, and activation, including automatic restores, cached bytes, local `path` packages, and local layouts. CEL and restricted Starlark share the input; see [Plugin admission policies](plugins/policy.md) for phases, origins, trust boundaries, and acceptance. This is implemented in the working branch, not a claim that all plugin release requirements have shipped.

The current-code and incoming-dependency distinctions apply to all examples outside this first slice.

The security analysis that motivated the effort is documented separately in
[Function Execution Security Analysis](../function-execution-security-analysis.md); its findings become
**two policy types** (`function` and `source`) in this larger system.

## Shared evaluation context

The next native-policy release adds one shared context and operation surface across CEL,
Starlark, OPA, and JSON Schema. See [Policy Runtime Context](policy-runtime-context.md)
for the implementation contract, evidence semantics, and acceptance criteria. This covers
process environment, Atmos version/FIPS state, OS/architecture, effective user identity,
memory/disk capacity, calendar/date checks, CI labels, and verified approvers.
Language adapters consume detached data and shared check results; they do not independently
collect facts or implement conflicting date/version semantics. Native JSON Schema can explicitly
validate the context while existing document schemas retain their direct input contract.

## Concepts

### Component capability contract and policy library

The product target includes **all component types**, with shared command/configuration capabilities
and optional provider-native artifact capabilities. Component adapters must advertise their input
kinds, lifecycle admission points, artifact schemas, and completeness guarantees. CI provider
capabilities are independent of component capabilities. An unsupported required capability is an
evaluation error; an unrelated input kind is not applicable. Lifecycle policies run without
`--affected`; only affected policies require explicit affected analysis.

Current: Terraform-compatible saved plans plus the existing Terraform/Helmfile/Helm/Kubernetes
lifecycle entry points. Proposed: Packer, Ansible, container, emulator and custom-component admission,
externally validated preview artifacts, scanner/cost reports, and repository-only command integration.
Ansible check mode is not a complete saved plan; unavailable or skipped predictions must remain
explicit. Legacy validation behavior remains unchanged during this incremental rollout.

Ship a **tested, browsable policy library** (`policy-library/`) independently of remote library
resolution. Each entry contains one `policy.yaml`, a technical `README.md`, and colocated input/expected
decision fixtures in `tests/cases.json`. The website presents a compact searchable list at
`/policies/library`, retaining Reference navigation. Individual pages show the complete policy YAML
first, with copy controls, configuration, usage commands, and selectable tested examples. Reuse source
scanning, not the repository-browsing interface. No installer or featured promotion. End-to-end demonstrations
remain separate under `examples/policies/`.

The directory manifest supports optional `metadata.description`: plain-English document purpose,
preserved on resolved rules independently of the CEL predicate and denial `reason`. Every catalog
entry supplies a description. Descriptions are literal text, never evaluated or templated.

The first 12 catalog entries cover resource deletion/replacement/count/type/tag restrictions, fork-event selection,
mandatory TFLint, production scanning, post-apply HTTP callbacks, failure cleanup, and Helm operation
denial. Every distributed rule has allowing/non-applicable and blocking/selecting cases against the
actual YAML. Add boundary, null/unknown, and missing-input cases as appropriate; a sample without tests
fails the suite. Hook selection tests are supplemented by executor tests (dry-run, skip bypass,
failure propagation, event mismatch, and no replay during unrelated lifecycle events).

Website documentation and catalog entries describe implemented behavior only. Unimplemented
integrations, research candidates, and rollout proposals belong exclusively in this PRD. A policy
must not appear as executable merely because CEL can express its condition.

The current hook slice resolves only governing `spec.hooks`, optionally pins `spec.tools`, preserves
existing kind/step execution and retries, and distinguishes required execution from required success.
It rejects nested skip/background controls and does not render hook templates/YAML functions yet.
Broader trusted rendering, typed hook results, pre-operation validation of future obligations, and
durable duplicate handling remain design work. Post-apply failure cannot undo completed changes.

The `atmos-policy` consumer skill must track these implementation boundaries and catalog tests.

### Policy responsibility boundary

Atmos owns decisions about its execution and change workflow: whether an operation may proceed,
which saved plan is acceptable, what review evidence is required, and which checks/hooks must run.
It does not replace platform-native admission or runtime security enforcement.

Do not build a parallel Kubernetes security catalog for privileged Pods, seccomp/non-root settings,
image trust, resource quotas, or persistent-volume protection. Those controls belong in the platform
policy layer and must apply to clients other than Atmos. An Atmos policy can require an external
validator to run and succeed; that tool or the cluster retains ownership of its resource-security
rules. Optional preflight checks do not establish authoritative cluster enforcement.

The library can include checks on artifacts Atmos actually consumes (such as Terraform saved plans),
but their documentation must identify the invocation boundary rather than claim continuous
infrastructure compliance. Ansible recap and image-provenance processing are tool/report integrations,
not grounds for growing a second resource-security engine in the CLI.

### Deferred integration research (PRD only)

The following are design candidates, not runnable catalog entries. Add an entry only after its
collector/executor and enforcement tests exist. These govern one invocation or complete repository
analysis before fanout; cross-run aggregation and durable coordination remain Pro responsibilities.

| Proposed example | Required input / integration | Minimum acceptance cases |
|---|---|---|
| Protect labels while permitting additions | Structured before/after metadata and reviewer verifier | New ordinary label allows; protected remove/modify awaits review; absent versus null differs |
| Detect indirect policy detachment | Complete attachment inventories and governing revision | Selector/import/discovery/parameter change requires review; policy deletion caught; ambiguous move blocks |
| Protect workflow permission changes | Structured Actions workflow diff | Unchanged/read-only allows; write grant/unsafe event change requires review; renamed workflow still detected |
| Require immutable Action references | Parsed workflow steps and reusable workflows | Commit SHA allows; mutable reference denies; local actions handled separately |
| Require fresh designated reviewer approval | Current paginated PR review facts | Current head/non-author allows; dismissed/stale/self review blocks; partial/API-denied results error |
| Request a reviewer or update one PR comment | Approval resolver and typed GitHub action hook | Pending selects action; satisfied selects none; insufficient permission errors; duplicate event updates same comment |
| Limit changed/affected instances before fanout | Complete files/components/instances inventory | Boundary count allows; excess blocks; deletes included; incomplete analysis errors |
| Enforce a current-component cost budget | Cost artifact bound to evaluated plan | Below cap allows; above denies; missing estimate/currency mismatch/zero baseline handled |
| Require scanner completion and cap findings | Scanner report bound to current input | Clean completed scan allows; severe findings deny; empty/missing/stale/failed scan is not clean |
| Approved deployment window | Trusted captured timestamp/timezone | In-window allows; boundary/DST tested; missing time errors; evaluator does not read clock |

Additional language work may include pure aggregation, recursive traversal, CIDR containment, and
explicit sanitized-value helpers. Richer results need typed dynamic findings and hook payloads;
another boolean evaluator alone does not provide them. Named-reviewer verification requires fresh,
complete evidence, while live organization-team enrichment remains outside the token-only model.
Do not expose these candidates as supported settings in website documentation.

### Terminology

- **Policy** — a rule that allows/denies an action, requires approval, or selects/requires a hook.
- **Policy type** — *what* a policy inspects and *when*: `validation`, `plan`, `operation`, `function`,
  `source`, `change`, `event`, and `approval` (CLI/Native CI); durable run approval, access, and
  server-side orchestration remain Pro responsibilities. Hook execution is an effect, not a new engine.
- **PolicyDefinition** — a reusable, **parameterized** rule authored once (Rego/CEL/declarative) with a
  parameters schema and registered input kind (`spec.on`, e.g. `terraform.plan` or `github.event`).
  Input kind, component type, and CI provider are separate concepts; dispatch is not inferred from names.
- **PolicyLibrary** — a named, **pinnable collection of definitions** from a remote repo (or local path).
  The distribution mechanism: Atmos ships a built-in `atmos/*` library; orgs publish and pin their own.
- **Policy instance** — an entry that **references a definition by name** (`use:`), supplies parameters
  (`with:`), sets an action, and **targets by `match:` selector**. How policies are attached without
  knowing file paths.
- **Selector (`match:`)** — a tag/label expression (over component `metadata`, or a CEL expression) that
  decides which components a policy applies to. For changes, test the before and after objects
  independently and retain the rule if either matches; do not merge their metadata.
- **Governing source & evaluate-from-base** — policy is always read from the **base** revision (the
  protected target revision) and evaluated against the proposed changes. This governing revision is
  distinct from the Git comparison base; protecting future policy requires review of policy changes too.
- **Provider snapshot** — a small common envelope around provider-owned metadata, native events, and
  collected facts, with schema version, provenance, and explicit availability/completeness.
- **Approval requirement** — a conditional obligation whose current satisfaction is evaluated from
  collected evidence. GitHub open source supports named reviewers/trusted reviewer lists, not live teams.
- **Hook requirement** — work selected by policy and executed by the existing hook/step executor.
  "Must execute" and "must succeed" are separate requirements; neither implies approval of other actions.
- **Provenance** — whether a request originates from a locally-authored file (`local`) or a remotely
  imported one (`remote`). Policies are provenance-aware.
- **Attachment surface** — *where a policy comes from*: inline `policy:` on components/stacks, or the
  trusted out-of-band `policy.d/` directory.
- **Enforcement level** — `advisory` (warn only), `deny` (soft — a flag can override), or `enforced`
  (hard — no flag can override). Only trusted org/machine/CI tiers may set `enforced`. Mirrors the conventional
  advisory / soft-mandatory / hard-mandatory model.
- **Mode** — run-level posture: `off` (engine disabled), `warn`, or `strict`. Overridable per run with
  `--policy`, bounded by any org-set floor.

## Problem Statement

### User Needs

- **Platform / security teams** need org-wide guardrails enforced uniformly across laptops and CI —
  change freezes ("no prod deploys on Tuesday"), destructive-change limits ("never destroy prod"),
  supply-chain limits ("no `!exec` from remote imports") — that individual repos cannot weaken.
- **Component / stack owners** need to attach their own rules (required tags, plan checks, config
  validation) to the things they own, reviewed in pull requests, reusing shared rules rather than
  copy-pasting Rego files.
- **Individual developers** need to relax or disable policy on their own machine without weakening org
  policy, and without editing shared config.
- **Everyone** needs *one* policy system — one contract, one CLI, one test command — spanning config
  validation, plan checks, operational guardrails, and security, not a different mechanism for each.
- **Repository owners** need change-based ownership and CI-event policies without installing an
  infrastructure stack or a GitHub App. Provider-native information must remain available to authors.
- **Platform teams** need required scanners, callbacks, and cleanup expressed using existing hooks,
  without copying those hooks into every component or allowing a PR to skip mandatory work.

### Current State

- **The legacy policy mechanism is narrow.** `settings.validation` runs post-render and
  can only validate resolved config. It is untyped (decoded ad hoc), points at `.rego` files by path,
  and has no notion of plan-time, operation-time, or pre-execution rules.
- **Initial plan policy integration:** the saved-plan gate evaluates native plan JSON after plan and
  again before saved-plan apply. Operation policies now run at the existing hook-enabled lifecycle
  points. Command admission covers runnable CLI commands; instance admission uses shared stack resolution.
- **Function admission:** canonical YAML names are checked before execution, including early `!exec`;
  Go-template calls are checked with `engine: go-template`. Source destination checks and provenance
  attribution are implemented; trusted-source distribution and trust beyond attribution remain proposed.
- **Initial library.** Parameterized CEL examples and per-policy tests are available locally;
  remote library distribution and share-by-reference resolution remain proposed.
- **Native CI and hooks integration:** `pkg/ci/registry_provider.go` registers CI
  providers; the current `Context` has limited run/PR metadata, not a public complete event/facts
  snapshot. `pkg/hooks` supports conditional hooks, TFLint and other tool kinds, and registered step
  types via `kind: step`/`steps`. The optional provider snapshot capability now collects local GitHub
  Actions metadata/events; operation policies select governing hooks through the existing executor.
- **Initial scaffold and plan integration (this workspace):** `pkg/policy` supplies typed
  requests/decisions/effects, affected inventory contracts, provider-native CI snapshots and a
  collector interface, an explicit evaluator registry, and bounded CEL and Starlark predicate adapters.
  Required inputs fail closed; approvals remain outstanding until Native CI verification; pure evaluation returns hook obligations
  that the lifecycle integration executes. An explicit-directory loader supports inline CEL/Starlark plan and
  operation/admission rules and GitHub approval requirements. There is no general manifest/library resolver, general live API-fact collector, trust resolver,
  or Rego adapter in this new package. Existing validation and hook/CI reporting is unchanged.
  See [the scaffold contract](../../pkg/policy/README.md) for its narrower current API.
- **Merged dependency:** [restricted Starlark value evaluation](https://github.com/cloudposse/atmos/pull/3286)
  and the automation runtime are available on main. Policy evaluation now reuses their compiler and
  read-only value conversion through `pkg/script/starlark/value`, without importing the automation host.
  `engine: starlark` function bodies return a strict boolean over materialized `ctx.input`, `ctx.params`,
  `ctx.ci`, and `ctx.request`. Source is bounded to 64 KiB and execution to 100,000 interpreter steps.
  Printing is disabled and runtime diagnostics omit source/input values. Limits are not a memory sandbox.

## Goals

### Primary Goals

- **One engine, many policy types** — evaluate infrastructure, repository-change, approval, and event policies
  through a single engine, contract, and CLI.
- **Reusable by reference** — a parameterized policy library (built-in + vendored) that users
  instantiate by name, so most users never author or manage policy files.
- **A first-class, typed `policy:` section** on components/stacks that supersedes `settings.validation`,
  plus a **trusted out-of-band `policy.d/`** for org-imposed, tamper-resistant guardrails.
- **Four authoring styles** — declarative, CEL, OPA/Rego, restricted Starlark — one `atmos policy test`.
- **Provider-native inputs** — pluggable schemas and collection, without forcing different SCM/CI
  concepts into a GitHub-shaped universal vocabulary.
- **Policy-driven hooks** — reuse all supported hook kinds and registered step types, subject to their
  execution context; enforce required execution and success without bypassing admission policies.
- **Discoverable & AI-authorable (required)** — generated input schemas, real-input capture, a
  machine-readable library catalog, and the test loop, exposed as **MCP tools** on the existing Atmos MCP
  server so assistants can discover the data model, compose policies, and self-verify. This is a
  first-class requirement, not an add-on — it is what makes the rich language usable.
- **Open-source/Pro boundary** — CLI/Native CI evaluates available snapshots with native credentials;
  GitHub open source requires only `GITHUB_TOKEN`. Pro owns custom Apps and durable orchestration.
- **Zero-breakage phased rollout** — warn → opt-in strict → secure-by-default, announcing each step.

### Non-Goals

- A hosted GitHub App/webhook service, live organization-team membership resolution, or private
  cross-repository access beyond `GITHUB_TOKEN` in open source. Public or vendored libraries remain usable.
- Implementing GitLab/Azure DevOps adapters in this release, or inventing equivalent approval states
  across providers. The extension contract must permit their native semantics.
- Sandboxing arbitrary hook binaries or guaranteeing exactly-once external callbacks.
- Sandboxing the shell interpreter (a policy decides *whether* a function runs, not *how*).
- Cryptographic signing of policy (optional, may be deferred — see Open Questions).

## Plugin governance

Plugin registry and package restrictions are first-class consumers of the existing evaluator, distinct from future plugin-provided policy validators. `on: plugin` uses source/package phases, normalized registry/repository identity, explicit origin, and extension-registry capability IDs. Protected governing definitions apply to direct/catalog installs, automatic restores, cached activation, local conformance, publication/signing, and registry access/configuration/login. No component `match` selector applies to this input. A denial or error fails closed; general policy success cannot weaken mandatory signature or trusted publisher checks.

The [plugin policy PRD](plugins/policy.md) defines the company-only registry example and full contract. The `allowed-plugin-registries` and `allowed-plugin-packages` library entries include CEL and Starlark variants with shared fixtures. Protect runtime, policy directory, environment, and lockfile independently of candidate edits; admission is not an OS sandbox or remote policy distribution service.

## Policy-Type Taxonomy

One engine evaluates registered input kinds. The table groups capabilities; a definition's `on:`
selects its actual input contract. Effects (deny, require approval, request/require hooks) are separate
from the subject being inspected.

| Family | Example input kind | Inspects / evaluation point | Home |
|---|---|---|---|
| validation | `validation` | resolved component config, before plan | CLI / CI |
| plan | `terraform.plan`, `plan` | native Terraform plan, or explicitly normalized change preview; before apply | CLI / CI |
| operation | `operation` | command, target, and actual lifecycle outcome | CLI / CI |
| function | `function` | YAML/template invocation, before execution | CLI / CI |
| source | `source` | imports/vendor/remote sources, before fetch | CLI / CI |
| plugin | `plugin` | actual OCI destination before access; inspected manifest before install/activation, with cache/development/layout origin | CLI / CI |
| change | `git.change`, `atmos.affected` | file/field changes and affected stacks, components, instances, after analysis | CLI / CI |
| event | `github.event` | GitHub-native event and available collected facts | Native CI, GitHub first |
| approval | `github.approval` | GitHub review evidence satisfying outstanding requirements | Native CI, GitHub first |
| durable run approval / access / orchestration | separate Pro contracts | service-owned identity, locks, run creation, webhook handling | Atmos Pro |

**Input kind is not CI provider or component type.** `terraform.plan` identifies the Terraform plan
schema; `ci.provider: github-actions` identifies the collecting adapter; `component_type: terraform`
identifies an Atmos component. There is no overloaded `spec.provider` dispatch field. Definitions
using `on: terraform.plan` are eligible only for that registered input; their names and `use:`
references do not control dispatch. A Terraform input can arrive locally or under any CI provider.

`on: plan` is reserved for an explicitly normalized infrastructure change model
(`changes[].{address,kind,action}`), not a promise that every component produces a plan or every CI
provider shares a domain model. Label selectors narrow eligible subjects; they cannot make an
incompatible input compatible. A missing required adapter is an error, not a silently skipped gate.

A GitLab adapter may register its own event/approval inputs; Azure DevOps can do likewise. Neither
must translate its native vocabulary into GitHub's. Cross-provider helpers are opt-in library
abstractions only where their semantics are documented to be equivalent.

**Approvals and hooks are not exclusively Pro features.** A CLI invocation can evaluate fetched
GitHub reviews and execute existing hooks. What it cannot supply is an always-on event receiver,
organization membership unavailable to its token, or durable deployment authorization. That is the
boundary, rather than whether a policy involves a person or a side effect.

## Use Cases

### 1. Change freeze — no production deploys on Tuesday

A platform team enables the built-in `atmos/operation/deploy-window` policy with
`{ stages: [prod], deny_weekdays: [Tuesday] }`. `atmos terraform deploy` against a `prod` stack on a
Tuesday is denied before the command runs.

### 2. Deny destroying stateful resources (plan policy)

A component owner attaches `atmos/plan/no-destroy` (provider-agnostic, on the normalized change model:
`deny if action in {delete, replace}` for the given `kinds`). After the change preview is produced,
Atmos evaluates it and denies before `apply`. For Terraform-specific checks (raw `resource_changes`,
resource attributes, cost), a **Terraform plan policy** (`atmos/terraform/…`) is used instead — a
provider-bound policy that reads Terraform's model and only ever applies to Terraform components.

### 3. Require tags on every component (validation)

A stack-level policy uses `atmos/validation/require-tags` with `{ keys: [Team, Environment] }`, scoped
to all terraform components by label — no per-component copy-paste.

### 4. Org denies remote `!exec` everywhere (function, enforced)

`/etc/atmos/policy.d/org.yaml` denies `exec` from remote provenance with `enforced: true`. Within the
trusted Atmos execution path, an `!exec` inside a remotely-imported stack is denied, and neither
`--with-exec` nor `--policy=off` can override it. Local `!exec` still runs. This does not prevent a
machine administrator from replacing the binary or policy files.

### 5. Developer relaxes / disables policy on their own machine

With no org policy present, a developer runs `--with-exec` to allow one function, or `--policy=off`
to disable the engine for a run. Both are bounded by any org-set floor.

### 6. Author, test, and share a custom policy

An engineer writes a `PolicyDefinition` (CEL or Rego) with parameters, adds `PolicyTest` cases, runs
`atmos policy test`, and vendors it so other repos `use:` it by name.

### 7. Preview what strict mode would block

`atmos policy explain` (or `atmos validate`) reports every rule that *would* deny under strict, built
from warn-mode data, so a team can adopt strict safely.

### 8. A PR cannot weaken the policy that judges it

A PR edits or deletes a `policy:` block to bypass a rule. A trusted CI evaluator uses the policy from
the protected target revision, not the head revision. A separate change-review policy and protected
review workflow govern that edit's effect on future PRs; evaluate-from-base alone is insufficient.

### 9. Central, label-targeted policy library

An org publishes a pinned `kind: PolicyLibrary` (`github.com/acme/atmos-policies@v2`) and an enforced
policy that `use:`s `acme/plan/no-public-ingress`, targeted by `match: { labels: { compliance: pci } }`.
Every component labeled `compliance: pci` — across every repo — is governed, with no per-component
config. Complete before/after analysis retains an existing attachment when either version matches;
separate rules govern detachment and cover new unlabeled instances. Remote libraries must be public,
vendored, or otherwise accessible without requiring extra GitHub credentials in open source.

### 10. Deny imports and vendored sources from untrusted domains

A `source`-type policy (`use: atmos/source/trusted-domains` with an allowlist of hosts/orgs and denied
schemes) denies any config **import** or component **vendor** fetch whose domain isn't trusted, at the
go-getter download chokepoint before anything is fetched. One rule covers both mechanisms.

### 11. Draft a policy with an AI assistant

An engineer asks their AI assistant (via the Atmos policy agent-skill) to "block public S3 buckets in
prod"; the skill produces a correct `kind: Policy` using the right type (`plan`), a library `use:` or
CEL/Rego/Starlark body, a `match:` selector, and `PolicyTest` cases — then `atmos policy test` verifies it.

### 12. Require review for types of changes

Adding an ordinary metadata label is allowed. Modifying/removing a protected label requires an
affirmative current-head review from a named reviewer in the trusted `platform-reviewers` list.
That list is repository-maintained policy data, not a claim about live GitHub team membership.

### 13. Evaluate the affected set at the command that computes it

`atmos describe affected` evaluates policies over its computed changed files, components, stacks,
and instances before returning the result. `atmos terraform plan --affected` evaluates the same input
kind once after analysis and before filtering/scheduling any component plans. A rule can cap affected
production instances or require review before removing an instance. A normal deployment without
affected analysis does not trigger an affected-set policy; its operation and applicable plan policies
still run. Required CI workflows explicitly invoke affected analysis when they need this gate.

### 14. Require existing hooks through policy

Require TFLint after every successful Terraform plan, add production-only scanners, invoke an HTTP
callback after apply, and clean up after a failed operation. These use existing hook kinds and step
types, not policy-only actions. The lifecycle binding and required/success semantics are defined below.

### 15. Govern a repository without infrastructure configuration

A repository containing only source code and trusted `policy.d/` rules evaluates GitHub-native events,
file changes, and review requirements without loading `atmos.yaml` or fabricating stack/component data.

## Solution Architecture

### The engine and the enforcement point

Pure language adapters in `pkg/policy` evaluate trusted definitions against detached requests.
They never run hooks, fetch API data, or perform the governed operation. The engine host uses
injectable context and fact collectors before evaluation. An orchestrator collects facts,
evaluates policy, executes permitted work through existing executors, and publishes the final result.

Proposed public contract (illustrative Go types):

```go
type Decision struct {
    Status       DecisionStatus // allow | deny | awaiting_approval | not_applicable | error
    Mode         Mode           // off | warn | strict; bounded by trusted enforcement
    Findings     []Finding      // rule, reason, subject, source location
    Approvals    []ApprovalRequirement
    Hooks        []HookRequirement
}

type EvaluationResult struct {
    Decision    Decision
    HookResults []HookResult    // execution evidence, absent for pure evaluation/dry-run
}

type Enforcer interface {
    Evaluate(ctx context.Context, req Request) Decision
}
```

The request identifies its input kind and carries that kind's data plus the shared `ci` snapshot
envelope. Existing validation/plan payload fields remain available to their registered adapters;
a Terraform policy still reads `input.resource_changes`. Operation input includes
`operation.command`, lifecycle event/outcome, target and fixed evaluation time. Function/source
inputs include invocation/source provenance and available source locations; stack/component fields
can be unavailable during bootstrap. Parameters remain `input.parameters` in Rego and `params` in
CEL. Starlark receives equivalent frozen input and parameters.

A rule condition selects effects; it is not synonymous with denial. A matched rule can require a
review or a hook without denying the underlying operation. Aggregate all applicable requirements:
an allow from one rule cannot erase another rule's denial, missing approval, or required work.
Any error collecting/evaluating required inputs blocks an enforced gate. Findings retain all
causes even when an error takes precedence over a denial in the aggregate status.

`allow` from pure evaluation authorizes the next permitted work; it is **not evidence that required
hooks completed**. The orchestrator may publish final success only after outstanding approval and
hook requirements are satisfied. It can execute explicitly selected reporting hooks while a gate
is blocked, but cannot use a hook to perform the denied operation under another name.

### Evaluation points (existing seams and proposed additions)

| Family | Seam | Implementation status / reuse |
|---|---|---|
| validation | component validation | existing OPA/JSON-Schema code; proposed delegation to policy |
| plan | after change preview, before apply | reuse Terraform JSON generation; policy admission is new |
| operation | command dispatch and before/after lifecycle | component restrictions exist; general command/custom-command/MCP gating is new |
| function | YAML dispatch and template FuncMap | implemented checks before execution, including `atmos.Resolve` |
| source | import, downloader, include, and guarded template access | implemented destination checks before access |
| change / affected | Git comparison and describe-affected analysis | reuse analysis functions; complete enforcement inventory and policy adapter are new |
| event / approval | Native CI snapshot collection and reevaluation | provider registry exists; policy collectors/adapters are new |
| hook requirements | lifecycle orchestrator → `pkg/hooks` → kind/step executor | executors exist; trusted requirements and execution evidence are new |

Function/source admission happens **before** fully resolved component configuration exists. It cannot
depend on an inline policy that requires executing the very configuration being governed. Common
CI collection must not call describe/render functions or evaluate YAML functions during bootstrap.
MCP and custom-command entry points need the same admission checks; a separate interactive permission
or YOLO setting must not bypass enforced policy.

### Provider-native context on every request

Extend the existing provider registry through an optional snapshot-collection capability rather than
making every provider implement GitHub APIs. Keep the existing `ci.Context` contract and workflow
`when:` variables compatible (in particular, existing boolean `ci` must not become an object there).
Policy inputs use their own schema/activation.

Illustrative envelope; keys inside `metadata`, `event`, and `facts` belong to the named adapter:

```yaml
ci:
  provider: github-actions
  schema: atmos/ci/github-actions/v1
  provenance:
    source: github-actions-runtime
    collected_at: "2026-10-10T12:00:00Z"
  availability:
    metadata: available
    event: available
    facts: not_requested
  completeness:
    metadata: complete
    event: complete
    facts: unknown
  metadata: {}   # GitHub-specific run metadata, not a universal PR/run vocabulary
  event: {}      # actual native JSON payload; example contents omitted
  facts: {}      # API-collected evidence, with per-collection status/provenance
```

Each adapter publishes versioned schemas, supported collections, required permissions, and fixtures.
Preserve native event fields, including fields unknown to the current normalized run context.
Do not manufacture a GitLab/Azure webhook when only environment metadata exists. Local/no-CI is
explicitly unavailable, not an empty GitHub event. Unavailable, not requested, error, partial, and
known-empty data are distinguishable. Bounds on payload size or API pagination must surface
incompleteness; they cannot silently truncate an enforced input.

Collect the bootstrap snapshot once, before configuration evaluation; collect requested facts outside
the engine and freeze a new snapshot before a later evaluation. Record each collection's source and
time. The triggering event is historical evidence, not necessarily current state. For a
`workflow_run` relay, the actual event is `workflow_run`; a PR-provided artifact cannot masquerade
as an authenticated original review event. Resolve and verify current repository/PR/head via the API.

Never inject credentials or the entire `github` context containing tokens into CI input.
The separate shared `env` context intentionally exposes process environment to trusted policies;
it must not be logged or automatically exported. Native payloads may themselves contain sensitive user data: do not log them by
default, and redact captured test/debug exports with explicit redaction metadata. Redaction for
exports must not silently change the facts used by a live enforcement decision.

### Change and affected inputs — policies over describe affected

`on: atmos.affected` makes **the information behind `atmos describe affected` a first-class policy
input**, not a shell command a policy must invoke. Reuse the shared analysis and existing
`schema.Affected` concepts (stack, component, component type/path, affected reasons, dependents,
deletion information). Extend them additively for policy needs; do not break existing CLI output.

**Triggering is command-driven, not ambient.** The affected policy runs when command execution
actually computes an affected set. The command's ordinary pre-execution `operation` policy may run
first; the `atmos.affected` policy runs after analysis, when the list exists, and before consuming it.

| Invocation | Affected-policy evaluation |
|---|---|
| `atmos describe affected` | evaluate the complete analysis result before output |
| `atmos terraform plan --affected` | evaluate once before fan-out/scheduling; individual operation/plan policies still run |
| Any other command supporting `--affected` | evaluate through the same selection seam before consuming that set |
| Ordinary Helm/other deployment without affected analysis | do not compute or evaluate `atmos.affected`; other applicable policies still run |
| Explicit policy evaluation with an affected fixture/snapshot | evaluate the supplied input; do not implicitly execute describe or hooks |

Registering an affected policy must not make all commands scan Git or resolve every stack. Include
the originating command/lifecycle in the request so authors can distinguish describe from plan.
Do not parse a subprocess's `describe affected` stdout; dispatch from the shared analysis result.
`--affected` must not bypass policy by filtering/scheduling before the evaluation point. A missing
adapter or incomplete analysis is an error **when affected evaluation is requested**, not when an
unrelated command never produces that input kind.

An organization wanting every PR covered configures a required workflow that explicitly executes
affected analysis (for example, `atmos describe affected`) as part of its policy gate. An organization
wanting every deploy to use it additionally needs an operation rule requiring that execution path;
an affected policy alone is not a universal deployment restriction.

The proposed `affected` input provides:

| Collection | Meaning |
|---|---|
| `files` | changed repository paths, add/modify/delete status, old/new paths for rename candidates |
| `components` | changed component implementations, keyed by type/source identity; distinguish shared implementation changes from instance-only config changes |
| `instances` | affected configured instances, keyed by stack + component type + component name, with before/after identity and config where available |
| `stacks` | changed/affected stack identities and their related instances |
| `changes` | structured field paths (JSON Pointers), subject identity, before/after values, presence flags, and add/modify/remove actions |
| reasons/dependencies | direct versus inherited/dependency impact, related files, existing affected reasons, and dependents |

A changed component implementation may affect many instances; an instance variable edit need not
change that implementation. These are different sets, not aliases for one list. Missing fields and
explicit null values are distinct. New objects have no before value; deleted objects have no after
value. Rename similarity is evidence, not guaranteed semantic identity: ambiguous moves remain
remove/add changes with diagnostics. Git file diffs and field changes are data, never executable
patches. Raw file content/patches are fetched only when needed and remain subject to completeness and
secret-handling rules.

Record comparison SHAs, analysis options, filters, dependency expansion, errors, and completeness in
the input. Repository-only `git.change` works without loading Atmos configuration; missing Atmos
inventory is `not_requested`/unavailable, not a fabricated empty inventory.

**Reuse is not blind trust in default CLI filtering.** Existing affected processing skips some
abstract/disabled/locked objects and deletion handling enumerates particular component types; some
base-load errors can become empty results. The enforcement collector must analyze complete
before/after inventories with each revision's discovery configuration, preserve deleted/disabled/type-
changed/excluded objects and relevant abstract inheritance sources, and reject unresolved imports or
unsupported component coverage. It must not repoint head configuration into base and assume that
produces a complete historical inventory. Safe shared machinery may be factored out; an enforcement
mode must establish completeness before any optional presentation filters.

**Implemented boundary:** the current gate takes unfiltered, static merged inventories from both
clean worktrees, including abstract/disabled instances, rejects missing imports and unsupported
types, and evaluates `atmos.affected` before the legacy display/scheduling analysis. It does not
execute component YAML functions/templates to collect policy input. Both inventories use the
active discovery configuration. Completeness is therefore relative to that protected scope, not
an independent reconstruction of each revision's historical discovery. Git file changes remain
visible to policies, including discovery-file changes. Fully independent discovery, indirect
attachment-loss analysis, and transitive dependency graph expansion remain PRD work; do not claim
the current gate detects all evasion through changed discovery configuration.

An intentionally scoped rule may select from a complete inventory. A caller's `--stack` or affected
filter must not narrow a repository-level required gate. API file-list limits require a complete Git
comparison or a blocking error, not partial success.

Proposed example predicates over this input (effect configured separately):

```yaml
# Cap the number of affected production instances.
on: atmos.affected
action: deny
rule: |
  affected.instances.filter(i, i.after_present &&
    has(i.after.metadata) && has(i.after.metadata.labels) &&
    has(i.after.metadata.labels.stage) && i.after.metadata.labels.stage == 'prod').size() > 20
```

Also include library examples for review on removed instances, changes to component sources, and
protected metadata fields. The collector must supply schema-valid optional-field handling; a missing
stage label must be handled explicitly by the definition rather than causing accidental approval.

### Conditional ownership and GitHub approval evidence

Change-based ownership means "this class of change requires approval," not "labels can never change."
A reusable protected-label rule distinguishes:

| Change | Example policy effect |
|---|---|
| Add an ordinary label | no review requirement from this rule |
| Change/remove an existing protected label | require a configured platform reviewer |
| Add/change a label that controls access or policy scope | configurable review requirement; additions are not universally safe |
| Delete/move/disable an instance, alter inherited metadata or discovery | evaluate corresponding change rules, including possible attachment loss |
| Edit policy definitions, parameters, sources, hooks, or reviewer lists | require policy-owner review under governing policy |

GitHub PR labels are not Atmos `metadata.labels`; their event/fact schemas stay distinct.
A new unlabeled instance has no previous attachment: a baseline required-label/coverage policy is
needed if all instances must be governed. Do not claim semantic analysis can automatically prove
whether arbitrary Rego/CEL/Starlark edits weaken policy; review such edits conservatively.

Proposed binding example: the affected analysis describes changes; the GitHub adapter supplies review
evidence. The users below are an illustrative, explicit reviewer list, not an organization-team query.

```yaml
apiVersion: atmos/v1
kind: Policy
metadata: { name: protected-metadata }
spec:
  policies:
    platform-review:
      on: atmos.affected
      engine: cel
      rule: |
        affected.changes.exists(c,
          c.path == '/metadata/labels/compliance' &&
          c.action in ['modify', 'remove'])
      approvals:
        - provider: github-actions
          users: [platform-owner-a, platform-owner-b]
          count: 1
          allow_author: false
          require_current_head: true
```

The collector expands removal of a parent metadata object into relevant field removals (or the
definition must explicitly match the ancestor removal); deleting the parent cannot bypass this rule.
Whole-instance deletion is separately visible and should be governed by a deletion policy when
required. Adding `compliance` is not matched in this example; organizations can include `add` or use a
baseline rule. A matching local request with no GitHub review evidence is unresolved/error when
enforced, not satisfied. The same rule can be simulated with supplied review fixtures.

In open source, requirements name GitHub users or **static reviewer lists maintained in trusted
policy**, not live GitHub teams. Fetch all current reviews with `pull-requests: read`, bind evidence
to repository, PR, and head SHA, and count distinct eligible identities. Default to one non-author
affirmative review covering the current head. Comments/reactions are not approval; requested reviewers
and author association are not membership proof. A later decisive review supersedes that reviewer's
previous decision; dismissal does not resurrect an earlier approval for that requirement. Comment-only
reviews do not supply approval. New head commits require fresh approval; do not estimate validity from
author-controlled commit timestamps. Current-head review coverage and latest-push approval are distinct
guarantees; document GitHub branch protections needed for the latter.

All independently applicable requirements must be met. Preserve denial and error findings alongside
approval state; approval cannot override them. Reviewer-list changes are governed by the old list.
If evidence changes during collection/publication, discard stale work and reevaluate. Unsupported
team requirements or insufficient permissions are errors in enforced mode, never implicit permission
to continue. Review collection is read-only: evaluation does not approve or dismiss reviews.

### Saved Terraform plan enforcement

The policy artifact is the **saved binary planfile**, not Terraform's human-readable stdout or a
CI change-count summary. Convert a private snapshot with the selected Terraform executable's
`show -json`, then expose the complete native JSON object as `input` for `terraform.plan`.
Preserve resource addresses/types, ordered actions (including replacements), before/after values,
`after_unknown`, sensitive-value markers, outputs, configuration, and forward-compatible fields.
Do not substitute a lossy summary or fall back to parsing console output on extraction failure.

The local execution sequence is:

1. `plan` writes a fresh private artifact; exits 0 and 2 are successful plan outcomes, not verdicts.
2. Decode and evaluate that artifact. Publish the requested saved plan only if evaluation passes.
3. Before `apply --from-plan` / `apply --planfile`, privately snapshot the selected binary again,
    evaluate it against the current governing rules, and apply **that snapshot**, not a mutable source
    path. No persisted “passed” flag or result from another job authorizes a different plan.

Reject malformed, missing, state-only, unsupported-major-format, errored, or explicitly incomplete
plans. A no-change plan is still evaluated. Unknown values remain unknown: authors must explicitly
deny unknown required attributes rather than treating absence as compliant. Missing-field expression
errors block. Plan JSON contains secrets even when Terraform marks them sensitive: extraction must
bypass console/CI captures, masking transformations, and diagnostic recording; evaluation errors
must not echo input values. Temporary artifacts are private and removed on all exit paths.

**Current implementation:** `ATMOS_POLICY_DIR` overrides the default `policy.d/` directory (found at the same project root `atmos.yaml` discovery selects: config flags and variables, `ATMOS_CLI_CONFIG_PATH`, the working directory, then the Git root), captured before stack
configuration processing. It loads strict `atmos/v1`, `kind: Policy` documents with named inline
`spec.policies` entries (`on: terraform.plan`, `engine: cel`, `rule`, `action: deny`, optional `reason`
and `with`). Instance `match` supports literal tags/labels, component names, stacks, and component types.
An optional static Markdown `reason` explains a denial and can suggest an alternative. The CLI displays it
with the blocking rule ID, and JSON findings retain it in `effects.reason`. Reasons do not expand
templates or interpolate evaluated input; omitting them preserves the default denial message.
No policy templating/imports, remote library resolution, tier discovery, or trust inference.
Unknown fields, unsupported kinds/engines/actions and empty configured directories fail closed.
Matching rules govern plan and saved-plan apply independently of `--affected`; unmatched instances
are not subject to saved-plan restrictions from those rules.
These command gates are mandatory once opted in; no warn/off bypass is provided in this slice.

Implicit apply/deploy and direct destroy are rejected when plan rules are configured. Use explicit
`plan` (or `plan -destroy`) followed by saved-plan apply. Automatically splitting an interactive apply
into plan/approval/apply is future work: do not silently remove Terraform's confirmation prompt.
Planfile-disabled/remote-only planning and implicit Terraform argument environment variables for
plan/apply/show are rejected rather than allowing an artifact-selection bypass. Dry-run runs no
extraction/evaluation or infrastructure action and clearly reports that policies were not evaluated.

The loader is an opt-in execution mechanism, **not** a machine security boundary: a trusted workflow
must pin the binary, executable/providers, governing policy directory and environment. A mutable
workspace policy directory or unsetting the environment variable is not tamper-resistant enforcement.
This gate does not authorize state editing, imports, refresh, arbitrary hooks, or direct Terraform
invocations; broader operation/function/source guards remain future work. Rules requiring collected
CI facts, approval verification, or hook completion cannot be satisfied by this first implementation.

### Two attachment surfaces

Policies feed one engine from two places:

**1. Inline `policy:` on components/stacks** — first-class, typed, inherited; the home for
**self-imposed** validation/plan/operation rules and hook selections that belong with the component
and are reviewed in PRs. In CI their governing versions can be protected by the trusted runner.

**2. Trusted `policy.d/`** — loaded out-of-band (never templated/imported/merged), layered
(`ATMOS_POLICY_DIR` → system → user → project) and **tighten-only**; trusted tiers are the only sources that may set
`enforced` and author `function`/`source` admission rules. This is where org guardrails and security policy
live, because they gate execution and must be loaded before governed configuration. The trusted
runner also loads repository-wide event/change/affected policies and required hook bindings here;
these require no component attachment or `atmos.yaml`.

The loader enforces the split: an inline `policy:` may not set `enforced` or author `function`/`source`
admission rules — those are honored only from the trusted tier. `atmos.yaml` itself carries **no**
policy config, so there is never a "which one wins" ambiguity.

### Reusability — a policy library (define once, instantiate many)

Pointing each rule at a `.rego` file path is the least-reusable pattern. Learning from Gatekeeper
(ConstraintTemplate → Constraint), Sentinel (parameterized policies + policy sets), Checkov (a large
built-in named library + a no-code DSL), with label-based attachment and composition:

1. **`kind: PolicyDefinition`** — a reusable, parameterized rule authored once with a parameters schema.
2. **`kind: PolicyLibrary`** — a named, pinnable **collection of definitions**, sourced from a **remote
    repo** (or local path). This is the distribution mechanism: Atmos ships a built-in `atmos/*` library
    in the binary; Cloud Posse and orgs publish more; a repo declares the libraries it draws from. A
    library resolves the namespace that `use:` references. (Details + example below.)
3. **Instances** reference a definition by name (`use: <library>/<type>/<name>`) and pass parameters
    (`with:`) + `action` — **no file paths** in the common case. Inline `rule:`/`rego:` remains a one-off
    escape hatch.
4. **Targeting by tag/label selector (the attachment mechanism)** — instead of wiring policy into each
    component, a policy declares a `match:` selector over component `metadata.tags`/`metadata.labels` and
    attaches to every matching component (the Atmos-native autoattach). A central policy says "applies to
    everything labeled `compliance: pci`" without touching a single component. **Reuse:** `pkg/tags`
    (`MatchesTags` any/all, `MatchesLabels`, coercion) + the composable `pkg/list/filter` chain; the
    autoattach precedent is `pkg/component/graph.go` `filterGraphByTagsAndLabels` (uniform across all
    component types), reading `metadata` from the resolved section (`internal/exec/yaml_func_tags.go:13`).
    **Rich matching = CEL selectors** (`pkg/condition`): `match:` accepts simple tag/label equality as
    sugar and a CEL expression for anything richer (`'pci' in metadata.tags && metadata.labels.stage ==
    'prod'`), no bespoke operator grammar. **Stack-level targeting rides on global-scope metadata defaults
    (PR #2808)** — a stack-root `metadata.tags`/`labels` inherits to every component, so "all components in
    a `prod` stack" is just a stack-root label; no separate stack-tags concept. **Trustworthy for enforced
    rules:** evaluate the selector independently against before and after objects, retaining a match
    from either version. Do not union metadata. This relies on complete analysis; removing a governing
    label is separately reviewable, and new unlabeled objects need a baseline coverage rule.

A parameterized definition — CEL and its OPA equivalent are near line-for-line; params arrive as
`params` (CEL) / `input.parameters` (Rego):

```yaml
# CEL definition
apiVersion: atmos/v1
kind: PolicyDefinition
metadata: { name: atmos/operation/deploy-window }
spec:
  on: operation
  engine: cel
  parameters:
    stages:        { type: array, items: string }
    deny_weekdays: { type: array, items: string }
  rule: |
    operation.command in ['apply', 'deploy', 'destroy']
      && target.stage in params.stages
      && time.weekday in params.deny_weekdays
```

```yaml
# Rego definition (same policy)
apiVersion: atmos/v1
kind: PolicyDefinition
metadata: { name: atmos/operation/deploy-window }
spec:
  on: operation
  engine: rego
  parameters:
    stages:        { type: array, items: string }
    deny_weekdays: { type: array, items: string }
  rego: |
    package atmos
    import future.keywords.contains
    import future.keywords.if
    import future.keywords.in

    deny contains {"type": "operation", "reason": reason} if {
      input.operation.command in {"apply", "deploy", "destroy"}
      input.target.stage in input.parameters.stages
      input.time.weekday in input.parameters.deny_weekdays
      reason := sprintf("Deployments to %q are frozen on %s",
                        [input.target.stage, input.time.weekday])
    }
```

A **domain-specific** definition selects its registered input kind through `spec.on`.
`deploy-window` above uses `operation`; `no-destroy-resources` uses `terraform.plan`, independently
of which CI provider hosts execution:

```yaml
apiVersion: atmos/v1
kind: PolicyDefinition
metadata: { name: atmos/terraform/no-destroy-resources }
spec:
  on: terraform.plan          # selects the native Terraform plan schema
  engine: rego
  parameters:
    resource_types: { type: array, items: string }
  rego: |
    package atmos
    import future.keywords.contains
    import future.keywords.if
    import future.keywords.in

    deny contains {"type": "plan", "reason": reason} if {
      some rc in input.resource_changes            # ← reads Terraform's plan schema
      rc.type in input.parameters.resource_types
      "delete" in rc.change.actions
      reason := sprintf("%s would be destroyed", [rc.address])
    }
```

Definitions are distributed through a **`kind: PolicyLibrary`** — a named, **pinnable** collection of
definitions from a remote repo (or local path), fetched via `pkg/downloader` (`?ref=` incl. commit SHA
already works). The built-in `atmos/*` library ships in the binary; orgs publish and pin their own:

```yaml
# .atmos/policy.d/libraries.yaml (or declared in the trusted tier for enforced use)
apiVersion: atmos/v1
kind: PolicyLibrary
metadata:
  name: acme                                   # namespace prefix for `use: acme/...`
spec:
  source: github.com/acme/atmos-policies//definitions?ref=v2   # pinned remote repo
  included: ["terraform/**", "operation/**"]   # optional: only import these definitions
  excluded: ["**/experimental/**"]             # optional: minus these
```

`included`/`excluded` are glob filters over the definitions in the source, mirroring `vendor.yaml`'s
`included_paths`/`excluded_paths` — so an org can publish one broad policy repo and each consumer pulls
only the subset it wants.

Then `use: acme/terraform/no-public-s3` resolves against it. **Trust:** for *enforced* policy the library
declaration + pin must live in the trusted tier (a repo mustn't repoint a governing library to a no-op);
for self-imposed policy a repo declares libraries freely. Evaluate-from-base protects the library
*declaration* too (the pin comes from the governing revision). Resolve refs to immutable commits and
record them; a version tag alone can move. Enforced deployments should pin immutable revisions.
Integrity in v1 relies on protected sources/revision pinning, not signing, and private cross-repository
fetches are not a `GITHUB_TOKEN` capability.

Instances reference a definition — no rego, no file paths:

```yaml
components:
  terraform:
    infra/vpc:
      policy:
        require-tags:
          use: atmos/validation/require-tags
          with: { keys: [Team, Environment, CostCenter] }
          action: deny
        # Provider-agnostic — reads only the normalized change model (any component type that has a plan):
        no-destroy-stateful:
          use: atmos/plan/no-destroy
          with: { actions: [delete, replace] }        # optional kinds: [...] — values are provider vocab
        # Terraform policy — the `use:` string is just a reference. What makes it Terraform is the
        # referenced definition's `on: terraform.plan`; the engine runs it ONLY on the terraform
        # components in a (heterogeneous) stack, skipping helm/k8s/container/ansible regardless of labels.
        no-destroy-tf-stateful:
          use: atmos/terraform/no-destroy-resources
          with: { resource_types: [aws_db_instance, aws_s3_bucket, aws_dynamodb_table] }
        prod-change-freeze:
          use: atmos/operation/deploy-window
          with: { stages: [prod], deny_weekdays: [Tuesday] }
        # Inline one-off escape hatch — CEL as a block scalar (no external file):
        custom-guard:
          on: operation
          action: deny
          rule: |
            operation.command in ['apply', 'deploy', 'destroy']
              && target.stage == 'prod'
              && time.weekday == 'Tuesday'
          reason: "Production changes are frozen on Tuesdays"
```

For custom rules without Rego, the no-code path is CEL (`rule: |`); a Checkov-style declarative
attribute/operator/value DSL is an option under consideration (Open Questions).

### Convergence — authoring styles, one decision contract

Declarative, CEL, Rego, and Starlark are adapters to one policy result, not separate enforcement systems:

- **CEL** reuses Atmos's expression implementation for conditions/selectors. Keep its policy
  activation isolated from backward-compatible workflow/hook `when:` activations.
- **Rego** reuses embedded OPA. Existing `data.atmos.deny` findings and validation
  `data.atmos.errors` map to the common result; approval/hook effects use the new typed result adapter,
  not overloaded denial strings.
- **Declarative** library instances express common conditions, parameters, and effects without code.
- **Starlark (implemented)** reuses restricted compilation and value conversion, with read-only ordinary snapshot maps,
  bounded execution/cancellation, and no shell, filesystem, network, CI writers, or automation host.
  Do not pass a lazy `ValueMap` that can execute YAML functions while being read. Imports and any
  future helper modules must follow the restricted evaluator's allowlist, not automation defaults.

A definition's condition selects its configured effect(s). All four adapters produce typed findings,
approval requirements, and hook requirements. Exact language bindings must be schema-tested together;
the existing denial-only Rego path remains compatible. No evaluator performs side effects.

### Shared contract, phases, and modes

- Evaluate all applicable rules against the same snapshot; union findings and requirements. No
  engine's allow cancels another's denial. Errors in required evaluation block; otherwise deny,
  awaiting approval, and satisfied/not-applicable outcomes remain distinguishable.
- `off|warn|strict` controls advisory adoption, bounded by trusted enforcement. Trusted org/machine/CI
  floors take priority over CLI/env choices; inner policy can only tighten them. Mode flags cannot
  skip an enforced hook or approval gate. `--with-*` is not an approval or hook-completion signal.
- An advisory evaluation can report outstanding requirements without blocking, but must label itself
  advisory. Explicitly required repository gates and required hook execution do not inherit warn-only
  rollout defaults. Dry-run reports intended effects without dispatching them.

### Policy-selected and policy-required hooks

**Policy determines which work is required and when; existing hooks perform it.** Reuse
`pkg/hooks` kind registration, `kind: command`, scanner/store kinds, and `kind: step`/`steps`
bridging to `pkg/runner/step`. This includes script/Starlark steps when that incoming stack is
available, and HTTP or any other step registered in the executing build. Do not build a policy-only
action executor or restrict hooks to CI notifications.

The following is **proposed policy binding syntax**, using the existing hook payloads. A trusted
`Policy` document in `policy.d/` may contain named hook definitions and policy instances that bind
them; component-defined hooks keep their existing syntax and lifecycle behavior.

```yaml
apiVersion: atmos/v1
kind: Policy
metadata: { name: platform-lifecycle }
spec:
  hooks:
    lint:
      kind: tflint
      events: [after.terraform.plan]
      on_failure: fail
    callback:
      kind: step
      type: http
      events: [after.terraform.apply]
      on_failure: warn
      with:
        url: https://deployments.example.com/callback
        method: POST
        body: '{"event":"apply-completed"}'
    production-scan:
      kind: trivy
      events: [after.terraform.plan]
      on_failure: fail
    cleanup:
      kind: command
      events: [after.terraform.apply]
      when: failure
      command: /opt/platform/bin/cleanup
      on_failure: fail
  policies:
    lint-every-plan:
      on: operation
      rule: "operation.event == 'after.terraform.plan' && operation.status == 'success'"
      hooks:
        - { use: lint, required: true, require_success: true }
    report-apply:
      on: operation
      rule: "operation.event == 'after.terraform.apply' && operation.status == 'success'"
      hooks:
        - { use: callback, required: true, require_success: false }
    scan-production:
      on: operation
      rule: "operation.event == 'after.terraform.plan' && operation.status == 'success'"
      match: { labels: { stage: prod } }
      hooks:
        - { use: production-scan, required: true, require_success: true }
    cleanup-failed-apply:
      on: operation
      rule: "operation.event == 'after.terraform.apply' && operation.status == 'failure'"
      hooks:
        - { use: cleanup, required: true, require_success: true }
```

Default lifecycle hook success filtering is preserved; failure/always bindings must explicitly select
those outcomes. Early function/source evaluation cannot dispatch component-dependent hooks before
their context exists. Unavailable hook kinds/context produce explicit errors for required work.

**Execution and authority:**

- `required` means the matched hook must execute; `require_success` additionally requires a successful
  outcome. For optional hooks, existing `on_failure` applies. A required-to-execute callback with
  `on_failure: warn` may report a failed attempt without blocking; skip/missing-engine is not an
  attempt. A must-succeed requirement cannot be weakened by `warn`/`ignore`: reject conflicting
  trusted configuration and ignore lower-trust attempts to override it.
- Resolve enforced hook definitions, executable sources, tool versions, and executable configuration
  from trusted sources. A base-loaded command that executes a head-edited script is not protected.
  Scanner plugins/configuration and template/YAML expansion also require provenance checks. Treat
  input-derived values as data, not shell fragments. Arbitrary binaries are not sandboxed by policy.
- Reuse executor ordering, step sequences, retries, outputs, and failure modes; required success must
  inspect actual execution evidence, not an error swallowed by best-effort CI hooks. Persist rule,
  binding, source revision, invocation identity, attempt, and result in the evaluation report.
- Preserve explicitly declared sequences with `kind: steps`; do not infer a dependency ordering from
  unrelated map entries. A requirement that must precede apply has to gate that transition.
- Denied operations cannot be run indirectly by a hook. Hooks remain subject to operation/function/
  source admission. Carry origin/chain metadata and reject recursive policy→hook dispatch cycles,
  with bounded depth, timeouts, and cancellation. Hook success never erases a policy denial.
- CLI `--skip-hooks`, a disabled component hook, or a lower-trust name collision cannot suppress an
  enforced binding. This is a guarantee of the trusted runner, not of an administrator's laptop.
- Run post-operation hooks for the **actual invocation**, not for every PR reevaluation that includes
  an old plan/apply result. Before publishing final success, finish must-succeed post-plan work.
  Failed post-apply work can fail the command/downstream gate, but cannot roll back or relabel a
  successful infrastructure apply as never having happened. Keep both outcomes.
- `policy test`, `explain`, and dry-run/input capture never execute hooks. They return intended
  bindings and can evaluate supplied mock execution results.

**CI-event hooks:** an outstanding approval requirement can select an existing command/script/HTTP
hook to request named reviewers or upsert a PR comment. Such bindings use provider-native facts;
there is no claim that a built-in review-request hook already exists. On GitHub these optional writes
require `pull-requests: write`; they neither grant approval nor prove GitHub-team membership.
Publishing the mandatory policy gate is enforcement infrastructure, not an optional hook that can
be removed by policy authors.

**Delivery:** deduplicate bindings within an invocation. Event retries need keys containing repository,
PR/head, governing revision, rule/binding, and event/invocation identity as appropriate. Use upserts for
comments, check existing reviewer requests, and expose idempotency keys to callbacks. Open-source
Actions has no universal durable exactly-once store; arbitrary external callbacks may be delivered
more than once and must be documented as such. Re-running a lifecycle command is a new invocation.

GitHub-specific hook calls share the `GITHUB_TOKEN` boundary. Ordinary TFLint, HTTP, or other hooks
retain their existing credentials and execution model; policy does not mint credentials, authorize
new endpoints, or imply that every external callback can authenticate with a GitHub token.

## Enforceability — trusted revisions and complete analysis

Local enforcement is bypassable by an administrator who controls the binary, filesystem, or process.
CI enforcement is conditional on a trusted evaluator, protected sources/workflows, required checks,
and available evidence. Neither a fresh clone nor a `policy.d/` filename alone establishes trust.

### Separate the governing revision from the comparison base

The governing policy is loaded from the protected target revision, pinned for an evaluation, and
judges the proposed content. Do not equate it with `ci.ResolveBase()`: that API resolves an
**affected-detection base**, which may be a merge-base, push-before SHA, or fallback useful for a diff
but inappropriate as the source of current governance.

Record separately: trusted policy source/revision, comparison base, target revision, head revision,
event/run identity, and input-collection time. For PRs, resolve the target through the trusted CI/API
context and pin its commit; resolve both sides of comparison independently. For pushes, policy
enforcement is not retroactive prevention of a push that already happened. Define the governing
source independently of `event.before`; local mode is advisory and reports its selected revisions.

Reuse Git materialization/fetch utilities where safe, but isolate worktrees and caches per revision.
Load each revision's discovery/import configuration, not head paths/filters repointed into base.
Do not execute untrusted configuration or head code in a privileged collector. If complete semantic
resolution cannot be performed under the admission/sandbox constraints, report unavailable analysis
and block rules that require it; never treat resolution failure as an empty stack.

### Scope retention and policy lifecycle review

Use a trusted rule's selector on each version independently:

```text
applies = matches(rule@governing, object@before)
        OR matches(rule@governing, object@after)
```

Handle absent versions explicitly. This retains coverage for known objects without the incorrect
semantics of unioning metadata (especially for negation). It does not solve ambiguous identity,
missing inventory, newly unlabeled objects, or future policy deletion by itself.

A PR cannot replace the policy judging the current evaluation. However, deleting a policy that still
passes today can weaken the **next** PR. Require separate change-review rules for definitions,
attachments, reviewer lists, libraries, hooks, and indirect scope-affecting configuration. Complete
affected/attachment analysis reports removals and ambiguous moves; it is not proof of arbitrary code's
semantic equivalence or weakening.

### CODEOWNERS as a compensating control

Recommend required CODEOWNERS review for policy sources, libraries/pins, hook scripts/tool config,
workflow definitions, discovery/import configuration, and CODEOWNERS itself. Example repository
paths (adapt to actual source locations):

```text
/.atmos/policy.d/      @acme/platform
/.github/workflows/   @acme/platform
/.github/CODEOWNERS   @acme/platform
/atmos.yaml           @acme/platform
/scripts/policy/      @acme/platform
```

The repository administrator must separately enable required code-owner review and appropriate
branch/ruleset protections. GitHub, not Atmos, resolves teams for its native CODEOWNERS enforcement.
This does not confer team-membership visibility on `GITHUB_TOKEN` or prove semantic approval for
fields outside those paths. Protect the actual paths that affect scope, including shared stack
catalogs; a narrow example is not an exhaustive security boundary.
[GitHub CODEOWNERS documentation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)

### Required gates and authority

A stable repository-level `atmos/policy` gate covers repository changes and the complete inventory,
even when no component matrix jobs remain. Per-target statuses are supplementary. A deleted component,
zero selected targets, disabled CI cosmetics, or a missing policy file must not silently skip the
required evaluator. Trusted CI configuration establishes which policies/gates must exist.

Where inventory rules are configured, the gate-owning workflow explicitly invokes affected analysis
and aggregates that result with repository/event/approval evaluation. Only that orchestrator publishes
the aggregate success; a later successful `policy check` must not overwrite an earlier failed affected
phase. Individual commands can report their own findings without claiming the entire gate passed.
Repository-only gates do not require an Atmos inventory that the repository does not have.

Protected workflow/binary/source provenance and restricted status writers are prerequisites.
A status name is not authentication: writers can spoof statuses, and another workflow may share
the GitHub Actions app identity. Required checks must be paired with protected workflow controls and
repository permissions; document remaining bypass/admin privileges.
[GitHub required status checks](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches#require-status-checks-before-merging)

Gate failure cannot flow through the current best-effort `pkg/ci` hook/reporting path that warns and
continues. Required evaluation or status-publication errors fail the job. A normal Actions job result
is not automatically a status on the intended PR head; publish and correlate the required verdict
explicitly. Output source/SHA/verdict records are **evaluation metadata**, not cryptographic
attestations.

### Sources and local authority

- Repository policy comes from the trusted target revision, including inline attachments and
  declared library pins. Inline rules remain subject to admission rules loaded before config.
- Enforced central sources can be supplied by a protected runner or vendored/pinned from an
  accessible public source. `GITHUB_TOKEN` cannot generally fetch another private policy repository.
  Private cross-repository integration is not an open-source setup prerequisite.
- Trust is an environment property: a local user can alter their environment or binary. A fresh
  clone improves revision accuracy, not authority. CLI/env switches cannot override a trusted
  runner's floor within that runner; there is no claim of immunity to runner compromise.

## Output & Native CI Integration

Native CI owns provider discovery, snapshot collection, permission-aware enrichment, invocation of
the policy orchestrator, and verdict reporting. Policy evaluation stays provider-independent; input
schemas and evidence semantics stay provider-native. GitHub is the initial concrete adapter.

### Output and gate states

One structured evaluation result renders human output, JSON, job summaries, annotations, and SARIF.
Approval/hook requirements and their satisfaction are first-class JSON/summary fields; not every
pending approval is a source-code vulnerability suitable for SARIF. Optional JUnit is a later format.

The GitHub adapter currently uses **commit statuses**, despite interface names such as
`CreateCheckRun`. Reuse that implementation:

| Evaluation result | Required commit status |
|---|---|
| all requirements satisfied, no enforced denials/errors | success |
| verified no applicable requirements, analysis complete | success with not-applicable explanation |
| awaiting approval or mandatory work still running | pending |
| denied or must-succeed hook failed | failure |
| required collection/evaluation failed | error |

For a one-shot CLI invocation, pending approval returns a nonzero blocking outcome; do not keep a
runner alive waiting for a person. A later event/rerun evaluates current evidence. Advisory runs are
clearly labeled and cannot publish success on behalf of the configured enforced gate.

Use stderr for human diagnostics and stdout for structured results. Native CI may additionally write
summaries and outputs containing governing/comparison/head revisions and verdict. Optional comments,
annotations, or SARIF failures do not erase a mandatory gate failure; failure to publish the mandatory
verdict is itself fatal.

### GitHub permission budget — GITHUB_TOKEN only

No PAT, custom GitHub App installation, or extra organization-scoped secret is required for supported
open-source GitHub features. The token is repository-scoped. API requirements are documented per
capability; optional capabilities do not enlarge the baseline.
[GitHub token scope](https://docs.github.com/en/actions/concepts/security/github_token)

| Capability | Job permission | Requirement |
|---|---|---|
| Read supplied event JSON; emit local JSON, workflow annotations, job summaries | no additional API permission | payload must already be available |
| Read repository policy/config and Git content | `contents: read` | baseline repository analysis |
| Fetch PR details, changed files, reviews | `pull-requests: read` | baseline PR/approval analysis |
| Publish the Native CI commit-status gate | `statuses: write` | trusted reporting job |
| Inspect workflow runs for relay/evidence | `actions: read` | only when used |
| Inspect check runs | `checks: read` | only when a rule requires check evidence |
| Post PR comments or request reviewers | `pull-requests: write` | optional hook action |
| Upload SARIF to code scanning | `security-events: write` | optional; repository/code-scanning eligibility and uploader prerequisites also apply |
| Query live organization-team membership | unavailable through workflow `permissions` | not an open-source guarantee |

The workflow permission keys come from
[GitHub's permissions reference](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions).
Review reads require [Pull requests: read](https://docs.github.com/en/rest/pulls/reviews#list-reviews-for-a-pull-request);
status publishing requires [Commit statuses: write](https://docs.github.com/en/rest/commits/statuses#create-a-commit-status).
Team membership needs [organization Members: read](https://docs.github.com/en/rest/teams/members#get-team-membership-for-a-user),
which cannot be added to `GITHUB_TOKEN` using `permissions:`. Do not suggest `read:org`, a PAT, or
`members: read` as an open-source workflow fix. Static reviewer lists are not synchronized teams.

Minimal **job fragment** for repository/PR evaluation and required status publishing (not a complete
secure workflow):

```yaml
permissions:
  contents: read
  pull-requests: read
  statuses: write
steps:
  # A pinned Atmos binary and trusted policy source must already be provisioned.
  - name: Evaluate repository policy
    env:
      GITHUB_TOKEN: ${{ github.token }}
    run: atmos policy check
```

An optional reporting/evidence job replaces that permissions block with only the features it uses:

```yaml
permissions:
  contents: read
  pull-requests: write  # includes reads; enables comments / reviewer-request hooks
  statuses: write
  actions: read        # workflow-run inspection; also needed by some SARIF upload setups
  checks: read         # omit unless policies inspect check-run evidence
  security-events: write  # omit unless uploading SARIF
```

Unspecified permissions become `none`. Do not request `checks: write` for the current commit-status
reporter, or `contents: write`, `id-token: write`, or `write-all` for basic evaluation. Generating a
SARIF file needs no API permission; upload is separate. Document the selected pinned uploader's
requirements (including `actions: read` for private-repository cases where required) and repository
code-scanning availability.
[SARIF upload documentation](https://docs.github.com/en/code-security/how-tos/find-and-fix-code-vulnerabilities/integrate-with-existing-tools/upload-sarif-file)

Fork/Dependabot runs may receive a read-only token regardless of requested writes. Repository/org
settings and reusable-workflow restrictions further constrain effective permissions. Keep a read-only
evaluation path, but never claim it published a required verdict when writes were refused. Report
capability errors without printing tokens, and do not automatically escalate credentials or change
repository protections. Optional unused enrichment is not an error; unavailable evidence required
by an enforced rule is.

### GitHub event routing, freshness, and limits

The documented Actions integration must cover PR opening/reopening/head updates and relevant metadata
changes, reviews submitted/edited/dismissed, explicit trusted reruns, and `merge_group` when merge
queues are used. Review events are not `pull_request_target` activity types. Policies depending on
workflow/check results additionally need corresponding reevaluation triggers and self-event loop
suppression; input availability alone does not subscribe a workflow to events.

A reference Actions-only topology separates unprivileged event notification from a trusted evaluator:
a review workflow can signal a `workflow_run` workflow on the protected default branch. The latter
uses its own `GITHUB_TOKEN`, verifies repository/PR/head association through the API, and fetches
fresh facts. Treat relay outputs/artifacts as untrusted hints, not approvals or executable code.
If PR association is ambiguous/missing (including fork cases), require an explicit trusted rerun;
do not guess or publish success. `pull_request_target` may be used for appropriate PR events only
with the same no-head-code execution constraints.
[GitHub event/trigger documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)

Privileged evaluators must not check out/run head workflows, scripts, actions, or configuration
functions. Materialize candidate data without executing it; isolate any required untrusted analysis
from write credentials and verify its inputs/provenance. Do not treat an untrusted artifact's
"allow" result as an authoritative policy decision.
[GitHub secure-use guidance](https://docs.github.com/en/actions/reference/security/secure-use)

Serialize/coalesce evaluations per PR, bind results to governing revision and current head, and
recheck relevant facts before publishing. Base updates require reevaluation too; use appropriate
up-to-date-branch/merge-queue protections and a documented trusted refresh path. Merge-group gating
must resolve constituent PRs and recheck their requirements rather than assume the synthetic commit
has reviews. Unsupported/incomplete group analysis blocks, not silently skips.

**Limit of the claim:** these mechanisms are not an atomic merge-time authorization transaction.
Suppressed/modified relay workflows, missed events, permission failures, base changes, or review
dismissals can leave a previously successful status stale until reevaluation. A scheduled/manual
refresh is recovery, not proof of continuous enforcement. Document required repository controls and
residual races; do not claim a GHA-only setup is equivalent to a durable webhook service. Such a
service/custom App integration is Atmos Pro scope. Even Pro must document its own freshness guarantees.

## Design Precedent — Lessons from Palantir policy-bot

[Policy-bot](https://github.com/palantir/policy-bot) informs this design; it is not a dependency, a
manifest compatibility promise, or the open-source deployment architecture.

| Lesson | Atmos decision |
|---|---|
| Separate applicability from approval requirements | A protected-field change creates a review obligation; it need not be permanently forbidden |
| Load policy from the target branch | Adopt trusted governing revisions; separately govern deletion/weakening for future PRs |
| Compose approval rules | Preserve independently applicable obligations and explain what remains unmet |
| Approval comments and push invalidation are configurable | Do not copy defaults: use affirmative current-head reviews in enforced examples |
| GitHub App deployment | Do not assume its identity/organization permissions exist in `GITHUB_TOKEN` |

Additional lessons from the implementation:

- Its [result model](https://github.com/palantir/policy-bot/blob/develop/policy/common/result.go)
  preserves pending/approved/disapproved/skipped states and requirement details. Atmos needs similarly
  explanatory results, extended with hook requirements and execution evidence.
- Its [review handler](https://github.com/palantir/policy-bot/blob/develop/server/handler/pull_request_review.go)
  reevaluates reviews/dismissals and performs post-evaluation actions separately. Adopt the separation,
  while documenting the weaker event-delivery guarantees of Actions-only hosting.
- Its [file predicates](https://github.com/palantir/policy-bot/blob/develop/policy/predicate/file.go)
  cover path-based changes. Atmos adds structured field changes and `describe affected` inventories:
  shared components, concrete instances, inherited effects, and semantic attachment changes.

Borrow the conditional-review model and lifecycle discipline; extend with provider-native inputs
and existing hooks. Do not copy implementation code or claim feature parity with a hosted bot.

## Component `policy:` Section

A first-class, **typed** component/stack section (sibling of `settings`), superseding
`settings.validation`. Schema on the manifest:

```
policy: map[string]PolicyItem
PolicyItem: { use?, with?, on?, engine?, rule?/rego?/starlark?/source?, module_paths?, action?, approvals?, hooks?, description?, disabled?, timeout? }
```

`use:` + `with:` is the common (library) form; `on` + an engine body is the inline escape
hatch. `enforced` is **rejected** here (inline is repo-resident) — it is honored only from the trusted
`policy.d/`.

**Supersession with backward compatibility (not a transparent alias).** The canonical new form is
`policy:` with `on: validation`, not a sibling `validation:` section. During deprecation, existing
`settings.validation` definitions retain their current behavior and failure semantics. Migrate only
where equivalent behavior is verified; preserve the legacy execution path otherwise, never silently
drop an unsupported rule. Emit a deprecation warning with migration guidance. Define deterministic
coexistence without accidentally skipping or duplicating validation, and cover it with compatibility
tests before wiring the new section. The initial scaffold does not change legacy execution. Ship a
migration note (optionally a rewrite helper); removal requires a future major release.

**Plumbing** (`settings.validation` is untyped today — decoded ad hoc by `FindValidationSection`,
`validate_component.go:320`; the new section is typed). A new top-level component section touches the
same ~4 spots as `settings`/`hooks`:

1. **Name constant** — `PolicySectionName = "policy"` in `pkg/config/const.go` (near line 95).
2. **Collect/extract** — `ComponentPolicy`/`BaseComponentPolicy`/`ComponentOverridesPolicy` on
    `ComponentProcessorResult` (`stack_processor_process_stacks_helpers.go:61`); extract in
    `extractComponentSections` (`…_extraction.go:14`), overrides (`…_overrides.go:56`), inheritance
    (`stack_processor_utils.go:2440`).
3. **Deep-merge + assemble** — a `policy` merge block in `mergeComponentConfigurations`
    (`stack_processor_merge.go:68`, order global→base→component→overrides) and
    `comp[cfg.PolicySectionName] = finalPolicy` (`…_merge.go:480-517`) + `GlobalPolicy` wiring. Unlike
    `validation` (nested in `settings`), a sibling `policy:` does **not** inherit for free — it needs its
    own block.
4. **Surface in describe** — `extractDescribeComponentSections`
    (`describe_stacks_component_processor.go:546`) and the `ComponentSection` literal (`…:641-654`); plus
    the section reader in `internal/exec/utils.go:180`.

## Testing

`atmos policy test` runs **two** mechanisms:

**1. Native Rego tests** — `*_test.rego` with `test_*` rules using `with input as {...}`, executed via
the OPA Go SDK `tester` package. Full power (partial sets, helper rules, edge cases). The loader already
excludes `*_test.rego` from evaluation; nothing runs them today — that is the gap.

```rego
package atmos_test
import data.atmos
import future.keywords.if

params := {"stages": ["prod"], "deny_weekdays": ["Tuesday"]}

test_deny_prod_deploy_on_tuesday if {
  r := atmos.deny with input as {
    "operation": {"command": "deploy"}, "target": {"stage": "prod"},
    "time": {"weekday": "Tuesday"}, "parameters": params,
  }
  count(r) == 1
}
test_allow_dev_on_tuesday if {
  r := atmos.deny with input as {
    "operation": {"command": "deploy"}, "target": {"stage": "dev"},
    "time": {"weekday": "Tuesday"}, "parameters": params,
  }
  count(r) == 0
}
```

**2. `kind: PolicyTest` harness (engine-agnostic)** — declarative cases that drive a definition through
whatever engine it uses. **This is how CEL, Starlark, and declarative policies get tests**, and a simpler option
for Rego. It maps `with:` → `input.parameters` and `given:` → the rest of `input` (identical wiring to a
real instance), so a passing test proves real behavior.

```yaml
apiVersion: atmos/v1
kind: PolicyTest
metadata: { name: deploy-window-tests }
spec:
  target: atmos/operation/deploy-window
  with:   { stages: [prod], deny_weekdays: [Tuesday] }
  cases:
    - name: deny prod deploy on Tuesday
      given: { operation: { command: deploy }, target: { stage: prod }, time: { weekday: Tuesday } }
      expect: deny
      reason: "frozen on Tuesday"          # optional substring assertion
    - name: allow dev deploy on Tuesday
      given: { operation: { command: deploy }, target: { stage: dev }, time: { weekday: Tuesday } }
      expect: allow
    - name: allow prod deploy on Wednesday
      given: { operation: { command: deploy }, target: { stage: prod }, time: { weekday: Wednesday } }
      expect: allow
```

The harness asserts observable status (`allow`, `deny`, `awaiting_approval`, `not_applicable`, `error`),
findings, outstanding approvals, and intended hook bindings. Supplied mock hook results test final
gate behavior without execution. Fixtures include provider schema/version, availability, and
affected-analysis completeness. Native Rego tests remain available for internal helper assertions.
No network collection or hooks run in `policy test`; live capture is a separate explicit operation.

## Conformance & the Public Library

We prove the engine solves *meaningful* policies by **authoring a curated set — clean-room — from the
underlying published standards**, each with `PolicyTest` cases, so the library doubles as a **conformance
suite** that runs in CI. Exercise every implemented input family, including GitHub events, affected
inventories, and hook effects. Add Kubernetes coverage when that adapter exists; do not imply parity
with the initial Terraform integration.

**Copyright posture (firm): steer clear of any potential infringement.** The popular framework libraries
are used **only as coverage checklists** — "which meaningful policies exist, and do we cover them" — never
as a copy source. We do **not** copy their Rego/YAML/Sentinel policy code, even when permissively
licensed, to avoid any attribution-compliance ambiguity. Each Atmos policy is written from the
**published standard** (CIS, AWS FSBP, NIST, provider docs) and the policy's plain-language *intent* (an
idea, not copyrightable), and cites the standard's control ID. Attribute the standard, not the tool.
Legal review before anything is published.

Reference frameworks (as coverage maps, not copy sources), mapped to our types:

| Source | License | Maps to | Value |
|---|---|---|---|
| Palantir policy-bot | Apache-2.0 | `change`, `approval`, `event` | conditional-review and lifecycle design precedent; no code reuse or App requirement |
| OPA Gatekeeper library | Apache-2.0 | `validation`/`plan` (k8s) | validates the ConstraintTemplate→Constraint = PolicyDefinition+parameters model |
| Kyverno policies (CNCF) | Apache-2.0 | `validation` (k8s) | validates declarative YAML authoring + breadth |
| Checkov (1000+ CKV_*) | Apache-2.0 | `validation`/`plan` (multi-IaC) | breadth benchmark; its YAML DSL informs our no-Rego path |
| KICS | Apache-2.0 | `validation`/`plan` (TF/K8s/Ansible/Helm/Docker) | multi-component-type breadth |
| Trivy misconfig / tfsec | verify | `validation`/`plan` (security) | security breadth; SARIF-native |
| Terrascan / Regula | Apache-2.0 | `validation`/`plan` (CIS) | Rego reuse |
| Cloud Custodian | Apache-2.0 | `validation` + DSL/introspection | informs the no-Rego DSL + discoverability |
| Sentinel foundational (archived) | MPL-2.0 | `operation`/`plan` (CIS) | reference only (Sentinel lang; avoid verbatim copy) |
| CIS / AWS FSBP / NIST | standards | all | translate to the **standard**, not a tool — cleanest legally |

**Why clean-room closes both risks:** by never copying tool policy code, there is **no license to
comply with and nothing to infringe** — we avoid copyright *and* license-violation exposure in one move.
The trade is a bit more authoring effort (writing from the standard rather than adapting existing code),
which is worth it for a publicly-shipped library carrying the Atmos/Cloud Posse name. The `License`
column below is informational only; it does not imply we reuse the code.

**Phasing:** first a **conformance set** (~20–30 policies deliberately spanning all types —
infrastructure/change/event/approval/affected inputs and hook effects — all four authoring styles,
GitHub-native fixtures, and supported infrastructure adapters) to validate the engine and surface
gaps. Kubernetes and other adapters join as implemented, not as an initial parity claim. Then grow the full
public `PolicyLibrary` (`cloudposse/atmos-policies`) alongside the built-in `atmos/*` seed.

## Discoverability & AI Authoring

A rich policy language fails if authors — human or AI — can't discover **what data a policy operates
on**. This is the make-or-break for adoption. Learning from OPA (`# METADATA schemas:` input type-check),
per-type schemas and **sampling real inputs**, Sentinel (typed imports + **mocks** from
`show -json`), Cloud Custodian (`schema` **introspection**), Gatekeeper (OpenAPI `parameters`), and
Checkov (**library-first**), the through-line is: *typed schemas + show me a real example + a big library
so I rarely author from scratch*.

**Atmos's advantage — reuse existing domain data.** Preserve familiar command/provider data where
available, and explicitly document new policy-specific enrichments and completeness fields:

- `validation` input = `atmos describe component` (resolved config).
- `plan` input = the normalized change model + `terraform show -json` (per provider).
- `operation`/`function`/`source` input = the request documents (command/target/time; function
  name/args; source url/host).
- `git.change` input = the recorded Git/file/field comparison.
- `atmos.affected` input = the shared describe-affected result plus policy inventory/completeness
  metadata, produced only when a command requests affected analysis.
- `github.event`/`github.approval` input = adapter-owned native payload and collected facts. GitLab and
  Azure DevOps publish their own schemas when implemented, rather than aliasing GitHub fields.

Discoverability combines existing outputs with versioned schemas for the proposed inputs:

- **`atmos policy schema <input-kind>`** — print the versioned JSON Schema of the input you write against
  (generated from the same Go types Atmos already generates config schemas from).
- **`atmos policy inputs`** (and a `--dump-policy-input` flag on producing commands) — capture the
  actual request, including event or affected data without requiring a component. Component inputs
  may select a stack/component. No hooks execute; secrets are redacted in exports, with redaction
  and completeness recorded. A captured snapshot is test data, not authenticated CI evidence.
- **Editor/LSP + SchemaStore** — ship JSON Schemas and wire OPA `# METADATA schemas:` so `input.*`
  autocompletes and type-checks; reuse `atmos-lsp`.

**Make AI generation first-class (the real leverage).** AI struggles to write policy elsewhere because
there's no schema and no feedback loop; Atmos can give both:

- **Machine-readable everything** — input schemas per registered kind/adapter version, the decision/effect contract, the
  manifest kinds, and the built-in **library catalog** (each definition's `parameters` schema) are all
  published JSON Schema. That catalog is the "API" the model targets.
- **Library-first** — the assistant mostly composes named definitions (`use:` + `with:` + `match:`),
  which is far more reliable than emitting Rego; raw CEL/Rego/Starlark is the escape hatch.
- **Closed generate → verify → fix loop** — capture a real input (`atmos policy inputs`), draft the
  policy, run `atmos policy test` (the `PolicyTest` harness), read pass/fail, iterate. Deterministic
  verification is what makes AI-generated policy trustworthy.
- **MCP is a requirement, not an add-on.** The Atmos MCP server (`atmos mcp`, already exists) must expose
  policy tools so an assistant can introspect and self-verify live — without them, AI is back to guessing
  the input shape and can't test. Minimum tool surface:
  - `policy.schema(input_kind[, version])` → the registered input JSON Schema.
  - `policy.catalog()` → the library of definitions with their `parameters` schemas (what to `use:`).
  - `policy.inputs(input_kind[, target])` → a captured input; stack/component target is optional.
  - `policy.test(files)` → run the test loop and return pass/fail (the verify oracle).
  - `policy.explain(input[, target])` → dry-run: findings, approval requirements, and intended hooks.
  The policy agent-skill drives these tools; the same surface serves any external assistant.

## Coverage of the Security Analysis (function & source types)

Every finding in the [Function Execution Security Analysis](../function-execution-security-analysis.md)
is addressed by a `function`- or `source`-type policy at a specific enforcement point — the acceptance
criterion for those types:

| Finding | Type | Enforcement point | Policy control |
|---|---|---|---|
| **F-1** `!exec` runs from `atmos.yaml` itself | function | `pkg/config/process_yaml.go` `processScalarNodeValue` (before `processExecTag`, L453) | trusted `policy.d/` `function` rule loaded **before** any `atmos.yaml` eval; `deny_provenance`, `allow_commands`/`deny_commands`, `enforced` |
| **F-2** Remote imports trusted like local | source | `pkg/downloader/gogetter_downloader.go` (before `client.Get()`) | `source` `trusted_sources` (host/org allowlist) + `allow_schemes`/`deny_schemes`; denied at the chokepoint before fetch |
| **F-3** `!include` no path confinement | function | `pkg/config/process_yaml.go` + stacks include | `function` `include` `confine_to: [base_path]`, `deny_absolute`, `deny_parent_traversal` |
| **F-4** Non-hermetic Sprig / Gomplate / `atmos.Resolve`→`!exec` | function (+ source) | `internal/exec/template_utils.go` `ProcessTmpl` (wrap risky subset only) | `function` `template-func.deny` + datasource `source` scheme/host rules; `atmos.Resolve` wrapper emits `function` name `exec` (transitive) |
| **F-5** No pre-execution policy layer (OPA runs post-render) | function/source | the pre-eval / at-fetch evaluation points | the `function` and `source` types evaluate **before** execution/fetch — the exact gap F-5 names |
| **F-6** Bypassable URI validation; `DisableSymlinks:false` | source | `pkg/downloader/gogetter_downloader.go` | `source` `allow_schemes`/`deny_schemes` become the authoritative allowlist (superseding `ValidateURI`); flipping `DisableSymlinks` under strict tracked in Open Questions |

F-1 and F-5 are load-bearing: F-5 is closed by the pre-eval evaluation point existing; F-1 because the
trusted `policy.d/` loads *before* `atmos.yaml` is evaluated. Provenance (`local` vs `remote`) is the
through-line for F-1/F-2/F-4.

## CLI Usage Examples

The implemented authoring commands are `policy check` (bundle validation), `list`,
`schema <input-kind>`, and `test <cases.json>`. Enforcement stays at existing command/lifecycle
boundaries, with `--explain` adding stderr diagnostics. Standalone `policy inputs`,
`policy explain`, a repository-only enforcement event, and mode overrides below remain future design.

```shell
# Inspect, preview, and test; these do not execute policy-triggered hooks.
atmos policy list
atmos policy explain
atmos policy schema atmos.affected
atmos policy schema github.event
atmos policy inputs
atmos policy test

# Explicit repository/event/approval gate; no stacks required.
atmos policy check

# Compute affected information and evaluate affected policies.
atmos describe affected
atmos terraform plan --affected

# Advisory local relaxation only; cannot lower a trusted runner's floor.
atmos terraform plan --policy=off
ATMOS_POLICY=strict atmos describe stacks
```

`policy check` collects the explicitly selected repository/event inputs and evaluates them; registering
an `atmos.affected` definition alone does not cause implicit stack discovery. A required workflow
needing affected policies must explicitly invoke affected analysis and include its result in the gate.
A policy-input dump or fixture cannot replace live trusted collection for an authoritative status.
For any producing command, dry-run/explain retains findings without dispatching hooks. Normal
`describe affected` retains its machine-readable output when allowed and reports blocking policy
failure through the command's error/exit path, with diagnostics on stderr.

## Rollout Plan

Infrastructure admission defaults retain the staged `warn → opt-in strict → strict in a later major`
migration. This does **not** apply to an explicitly configured required gate, approval obligation, or
required hook: those requirements must work when enabled.

| Stage | Deliverable / behavior |
|---|---|
| Foundation | common decision/effect contract, trusted loading, CEL and Starlark implemented; declarative/Rego adapters proposed; legacy validation compatibility |
| First command slice (implemented in this workspace) | default `policy.d/`, `ATMOS_POLICY_DIR` override, strict inline CEL/Starlark rules and literal instance selectors, saved-plan JSON gate after plan and before saved-plan apply; mandatory for matching instances, without trust-tier discovery |
| Local Native CI integration (implemented) | optional snapshot capability on existing registry; GitHub-native supplied metadata/event; no API calls; required missing evidence blocks |
| Lifecycle hooks (initial slice implemented) | Terraform/Helmfile/Helm/Kubernetes operation rules reuse hook/step engines, governing definitions/tool pins, required execution/success and dry-run; other components and richer result evidence remain work |
| Affected gate (initial slice implemented) | unfiltered static before/after inventories under active discovery, Git files, component-source changes, structured fields; deny before consuming affected output; clean worktrees and strict collection required |
| Further Native CI integration | independent historical discovery, repository-only changes/current approval inputs, token-only permission profiles, stable required gate and documented refresh limitations |
| Further hook integration | preflight future obligations, trusted rendering, typed per-invocation results and event duplicate handling |
| Advisory adoption | existing commands warn by default unless explicitly strict/enforced; optional hook bindings run only when configured, not merely because a warning exists |
| Later major | consider stricter defaults with migration guidance; local opt-outs remain bounded by trusted floors |
| Extension targets | GitLab/Azure DevOps adapters, additional infrastructure inputs, and Pro integrations specified separately; no implicit parity promise |

No automatic repository-protection changes, review submission/dismissal, or credential escalation.
JSON/SARIF capture and warnings preserve stdout/stderr separation. Document added API calls, rate-limit
handling, analysis cost, and skipped optional collections; required incomplete inputs block rather than
downgrade to warn. Preserve existing Native CI `Context`, hook `when:` behavior, and legacy validation
during migration.

## Security Considerations

### Bootstrap and source trust

Load admission policy before any `atmos.yaml`, import, template, or YAML-function evaluation.
Out-of-band means isolated parsing with no configuration execution—not merely a different directory.
Inline policy can contribute self-imposed constraints but cannot disable this admission layer or claim
trusted authority. CI provider collection at bootstrap must not depend on component rendering.

Policy, hook definitions, toolchain/plugin configuration, executable scripts, and reviewer lists all
need protected provenance. Protecting only the policy file leaves indirect code-execution and scope
changes open. Metadata and PR fields are untrusted data even when delivered by a trusted API; never
interpolate them into shell code. Credentials stay in collectors/executors, not evaluator inputs.

### Monotonic constraints, not semantic equivalence proofs

Inner scopes may only add constraints or obligations, not remove trusted ones. Intersect allowlists,
union denials, preserve required approvals/hooks, and prevent lower-trust mode/skip overrides.
Do not attempt to prove an arbitrary program "stricter" through a merge algorithm. Evaluate trusted
rules independently, retain their obligations, and review policy-program changes conservatively.

Avoid duplicate-definition/name shadowing of enforced hooks and policy libraries. Loading failures,
missing trusted sources, API permission errors, truncated inventories, and unsupported required input
kinds must be distinguishable from an empty set of applicable rules.

### Authority and remaining escapes

The guarantee is conditional on the trusted CI execution path and repository controls. Local machine
administrators, compromised runners, status writers, repository administrators with bypass rights,
or editable workflows remain outside a simple in-process policy boundary. CODEOWNERS plus required
review is a compensating control; it neither implements semantic ownership nor eliminates stale-status
races. No unsigned output file is an attestation.

Affected policies run only at affected-analysis execution points. A normal deployment that never
requests affected analysis does not evaluate them; universal restrictions need operation policies or
a required workflow that deliberately computes the inventory. Report this boundary clearly, rather
than silently enabling expensive analysis everywhere.

A privileged collector must not run candidate Terraform, hooks, or config functions to "discover"
its own authorization. Required semantic analysis that cannot be obtained safely fails closed.
Separate evaluator authority from any untrusted analysis environment and treat its output as data
requiring verification, not a trusted verdict. Restrict remote imports/library fetches and credentials
according to the same source-policy rules.

## Boundary with Atmos Pro

| Open source: CLI / Native CI | Atmos Pro |
|---|---|
| Evaluate native event snapshots and available repository facts | Custom GitHub App / other service integrations and durable webhook ingestion |
| GitHub review requirements over named users/trusted static lists with `GITHUB_TOKEN` | Organization-level identity/team enrichment with explicitly granted permissions |
| Required policy-selected lifecycle hooks using existing executors | Durable cross-run scheduling, service-managed notification/trigger orchestration |
| Explicit affected/repository analysis and CI merge-gate verdicts | Durable deployment approvals, access/login, locks, drift and run creation |
| Evaluate the current saved plan and enforce the current command/job | Aggregate outcomes across component runs/jobs/workflows/repositories; global budgets and deployment completeness |
| Public/vendored or repository-local policy libraries | Private cross-repository integrations beyond the native job token's access |

There is no blanket rule that "approval," "notification," or "trigger" implies Pro: the CLI can
inspect reviews and invoke an HTTP hook during a run. Pro owns the service/identity/durability
capabilities the CLI lacks. Do not build a GitHub App into open source or suggest PATs to make an
unsupported team policy work. Reserve extensible manifest/input namespaces; Pro-specific durable
contracts are outside this PRD, and need their own trust/freshness design.

Combining rules within one request, and inspecting a complete affected inventory before execution,
remain local capabilities. Combining results from the subsequent component runs is orchestration
and belongs to Pro. This boundary does not remove existing Native CI aggregate reporting features.

## Testing Strategy (implementation)

These are acceptance requirements for the full implementation. Initial `pkg/policy` unit tests cover
CEL dispatch, decision precedence, data isolation, required-input failures, native JSON preservation,
affected label-change predicates, cancellation/cost bounds, and explicit unsupported-language errors.
Saved-plan integration tests additionally cover private artifact binding, denial, exit 0/2,
reevaluation before apply, secret-safe extraction, and unsupported argument paths; a local Terraform
integration test uses only the built-in `terraform_data` resource. These do not establish provider,
approval-verification, hook-execution, or general command-admission acceptance.

| Area | Required scenarios |
|---|---|
| Engine contract | consistent decisions/effects across declarative/CEL/Rego/Starlark; deny/error dominance; independent requirements; compatibility with `data.atmos.errors` |
| Saved plan gate | fresh artifact after exit 0/2; native JSON with unknown/sensitive fields; no-change and replacement plans; invalid/state-only/incomplete/unsupported inputs; no stdout fallback; extraction secrets absent from console and CI captures |
| Apply binding | reevaluate under current rules; apply the privately evaluated binary; source replacement cannot swap applied bytes; changed private artifact rejected; implicit apply/deploy/destroy and planfile-disabled operation fail closed; failed planning cannot evaluate a stale plan |
| Gate failures | denial/evaluation/extraction failures never swallowed by CI exit-code remapping or best-effort telemetry; dry-run does not claim policy success; canceled operations clean up artifacts |
| Starlark isolation | immutable snapshots; no automation host, network, shell, filesystem, lazy YAML evaluation, or unbounded execution |
| Provider extension | GitHub schema fixtures and synthetic non-GitHub adapter with different fields; native fields preserved; no fabricated event; existing `ci.Context` and boolean `when: ci` unchanged |
| Availability | known-empty versus not-requested/unavailable/error/partial; missing required inputs block, unused optional inputs do not; schema-version errors |
| Repository-only use | no stacks/components/`atmos.yaml`; file/event/approval policies still work |
| Affected command triggers | describe evaluates after analysis/before output; `plan --affected` evaluates once before any fan-out; ordinary Helm deployment does not trigger affected analysis; per-instance operation/plan policies still run |
| Affected data | shared component edit impacts multiple instances; one instance config edit leaves implementation unchanged; files, stacks, components, instances, reasons and dependents remain distinguishable |
| Complete inventories | deletes/moves/type changes/disabled/abstract/inherited/excluded objects; separate base/head discovery; missing imports, file-list caps and unsupported component coverage cannot become success |
| Change ownership | ordinary label addition allowed; protected edits/removals require review; security-sensitive additions configurable; new unlabeled objects covered only by explicit baseline rules |
| Policy lifecycle | deleted/edited rules, attachment changes, libraries, reviewer lists, hook scripts and workflow edits governed by trusted revision; no head-controlled source/discovery narrowing |
| Selector semantics | independent before/after matches, including negative CEL selectors; missing versions and ambiguous identities |
| GitHub credentials | baseline `GITHUB_TOKEN` permissions only; named reviewer approval; organization-team lookup rejected; no PAT/App fallback; optional permission profiles separately exercised |
| Approval lifecycle | distinct non-author current-head approvals; stale/duplicate/comment-only reviews; changed/dismissed reviews; list edits cannot authorize themselves; denial unaffected by approval |
| CI routing | fork/Dependabot read-only runs; trusted relay verification; ambiguous PR association; explicit reruns; base/head races; merge groups; required-gate publication failures |
| Hooks | TFLint after successful plan; production scanner selection; HTTP after apply; failure cleanup; optional reviewer/comment actions; unavailable step kinds and incompatible lifecycle contexts |
| Hook authority | `--skip-hooks`, disabled/shadowed bindings, and lower-trust `on_failure` cannot remove requirements; executable/config dependencies remain trusted |
| Hook outcomes | must-execute versus must-succeed; fail/warn/ignore; timeout/cancellation; retries/order; original apply outcome preserved on callback failure |
| Duplicate execution | review reevaluation never replays lifecycle hooks; comment upserts; event-key deduplication; callback retries documented; recursion bounded |
| Simulation | test/explain/dry-run/input capture never run hooks; mock hook results; no live API during fixture tests; secret-redacted exports marked |
| Rollout / reporting | advisory labels cannot satisfy required gates; no-target matrix still has repository gate; stdout remains valid data; optional reporting does not swallow mandatory failures |

Use dependency-injected collectors/executors and table-driven fixtures; do not require real credentials
or network access for unit tests. Add integration tests for command lifecycle placement and isolated
provider/reporting adapters. Preserve existing validation behavior and hook/CI APIs unless a migration
is explicitly documented.

## Files to Create / Change

**Implementation map, not changes made by this PRD edit:**

- `pkg/policy/`: trusted loader, input registry, decision/effect types, four evaluator adapters,
  library resolver, governing-revision selection, approval requirements and simulation.
- `pkg/ci` and provider packages: optional snapshot/facts collector interface, versioned provider
  schemas, permission diagnostics, required-gate reporting and freshness checks; reuse the incoming
  reporter seam without inheriting best-effort error swallowing.
- `internal/exec/describe_affected*` and command selection/scheduling seams: factor shared analysis,
  add complete enforcement inventories and `atmos.affected` dispatch after analysis/before consumption.
  Preserve `schema.Affected` CLI compatibility; extend policy data with files/components/instances,
  structured changes and analysis metadata. Do not reuse filtered output as proof of completeness.
- `pkg/hooks` and `pkg/runner/step`: accept trusted policy bindings and immutable event/decision
  context, preserve engine reuse, enforce mandatory work and record execution results.
- `pkg/script/starlark/value`: shared restricted compilation and value conversion; no new interpreter or
  automation-host exposure inside policy evaluation.
- Command/MCP/function/source/plan entry points: invoke the appropriate input-kind evaluation;
  early admission does not require fully rendered components. Ordinary commands must not implicitly
  run affected analysis just because affected policies exist.
- `cmd/policy/`: proposed list/check/test/explain/schema/inputs commands and safe input capture.
  Read-only authoring/MCP operations expose requirements without executing effects.
- Component-section extract/merge/describe plumbing and `pkg/manifest`: register policy manifests,
  preserve validation migration, and add trusted repository-wide hook/approval binding schemas.
- Generated schemas, documentation and conformance fixtures: input kinds, provider-native metadata,
  decisions/effects, permission profiles, explicit affected execution, and hook lifecycle examples.
- Source/revision loading utilities: separate governance from diff base, isolate caches/worktrees,
  validate completeness and source access; emit evaluation metadata, not an attestation claim.

## Open Questions

The following are implementation details, not invitations to reopen the product boundaries above:

- Final manifest field validation for inline bodies versus library references, typed approval effects,
  and the proposed hook-binding syntax; keep component policy map compatibility.
- Exact versioning/size limits for native payloads and affected snapshots, and safe capture redaction.
  Exceeding limits must be visible and blocking when the omitted data is required.
- Scope of the initial library seed and precise registered input schemas; GitHub first, with no
  GitLab/Azure or Kubernetes implementation implied by the extension interface.
- Safe deterministic Atmos inventory rendering when configuration depends on dynamic/secret inputs.
  Until completeness can be established, relevant enforced analysis cannot pass.
- Detailed Actions workflow templates and repository-control prerequisites, including merge-queue
  correlation and trusted review refresh. Verify fork cases end-to-end before claiming support.
- Event-action deduplication storage/retention where needed; no exactly-once promise for arbitrary
  hooks, and no hidden open-source webhook-service dependency.
- Selector implementation placement, library namespace/pinning validation, and normalized
  infrastructure-plan schema; none should force CI providers into a universal event vocabulary.
- Remaining function/source security details: symlink handling, datasource alias resolution,
  provenance fidelity and audit volume. Admission must still precede the governed action.

**Settled:** provider-native context; token-only open-source GitHub scope; no live team-membership
claim; existing hook executor reuse; separate required execution/success; command-driven affected
policies; explicit comparison/governing revisions; complete analysis for scope claims; custom
Apps/durable orchestration in Pro.

## References

- [Function Execution Security Analysis](../function-execution-security-analysis.md).
- [Palantir policy-bot](https://github.com/palantir/policy-bot) and its linked result/review/file-predicate
  implementation in the design-precedent section; design reference, not copied code.
- [Merged restricted Starlark evaluation](https://github.com/cloudposse/atmos/pull/3286);
  [CI module PR](https://github.com/cloudposse/atmos/pull/3307).
- [GitHub token scope](https://docs.github.com/en/actions/concepts/security/github_token),
  [workflow permissions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions),
  and [event triggers](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows).
- [GitHub reviews API](https://docs.github.com/en/rest/pulls/reviews),
  [team-membership API](https://docs.github.com/en/rest/teams/members),
  and [changed-file API limits](https://docs.github.com/en/rest/pulls/pulls#list-pull-requests-files).
- [CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)
  and [protected branches](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches).
- Existing code: `pkg/ci/registry_provider.go`, `pkg/ci/internal/provider/types.go`,
  `pkg/ci/providers/github/checks.go`, `pkg/hooks/hook.go`, `pkg/hooks/step_engine.go`,
  `pkg/schema/schema.go` (`Affected`), and `internal/exec/describe_affected*`.
- Existing validation: `internal/exec/validate_utils.go`, `internal/exec/validate_component.go`;
  manifest convention: `pkg/manifest`; CEL: `pkg/condition`.
