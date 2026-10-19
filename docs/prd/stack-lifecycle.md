# Stack Management and Lifecycle Operations

**Status:** Implementation in progress. Current-definition deploy/destroy, scaffold-backed manifest creation, and hook-aware dry runs are implemented. Stack creation reuses the scaffold catalog and renderer, validates complete file plans, and records each stack independently. Deleted-definition recovery remains blocked pending the prerequisite teardown implementation; selecting deletions with `--allow-destroy` fails before any operations start.

**Prerequisite:** Merge [PR #3415, Teardown of Deleted Components](https://github.com/cloudposse/atmos/pull/3415)
before this work. Its `docs/prd/deleted-component-teardown.md` governs configuration
recovery, source provenance, deployed identity, execution, and retries for
removed components. This proposal selects a CLI entry point for that workflow;
it does not assume that merging the PRD delivers its implementation. Approval and confirmation are delegated to each component handler, superseding the proposed shared review-artifact boundary for this initial implementation.

## Purpose

Atmos should operate on a stack's components across component types without
requiring users to choose a tool execution order. Declared dependencies determine
readiness, and the existing concurrent DAG scheduler runs independent components.

Stack operations belong under `atmos stack`. Creating and deleting definitions,
editing configuration, and deploying and destroying infrastructure have distinct
commands. No top-level `atmos deploy` or `atmos destroy` is introduced.

## Command structure

| Command | Responsibility |
| --- | --- |
| `atmos stack create` | Create a stack manifest. |
| `atmos stack delete` | Remove a stack manifest. |
| `atmos stack config get\|set\|delete\|format\|list` | Read and edit configuration within manifests. |
| `atmos stack deploy` | Deploy selected components; explicitly opt into teardown of removed components. |
| `atmos stack destroy` | Destroy selected components that remain defined. |
| `atmos stack schema` | Describe the manifest schema. |
| `atmos stack validate` | Validate stack manifests. |

Broaden the stack command group's help description beyond reading and editing
component configuration. Keep the existing type-specific lifecycle commands.

## Selection and execution controls

```shell
atmos stack deploy --affected
atmos stack deploy --affected --stack dev
atmos stack deploy --all
atmos stack deploy --all --stack dev

atmos stack deploy --affected --allow-destroy
atmos stack deploy --affected --allow-destroy --stack dev

atmos stack destroy --affected --stack dev
atmos stack destroy --all --stack dev
```

Require exactly one of `--affected` and `--all`. Initially, these commands accept
no positional component argument. `--stack` narrows the initial selection; omitting
it selects across stacks. Reuse existing affected detection and baseline options,
including `--base`, rather than introducing another Git comparison algorithm.

Support `--include-dependencies[=N]` and `--include-dependents[=N]`, disabled by
default. Preserve the existing optional-depth syntax. Explicit expansion can cross
stack boundaries. These flags expand the current-definition selection; they do
not expand the deleted-component destruction set.

Reuse `--max-concurrency`, `--failure-mode=fail-fast|keep-going`, and
`--log-order=stream|grouped`, with defaults `1`, `fail-fast`, and `stream`.
`--dry-run` shows selections, exclusions, dependency relationships, and resolved
operations without invoking lifecycle commands. It also shows the registered before/after hook events, declared actions, and unevaluated conditions, including image builds attached to deployment. Skipped hooks are identified. It does not execute hooks, resolve their runtime expressions, or promise conditional hooks will run. A dry run is not a saved engine plan or approval artifact.

Empty selections succeed with an explicit message. Summaries distinguish
successful operations, failures, skipped operations, excluded components, and
deletions awaiting teardown; a partial reconciliation must not be presented as
all infrastructure having converged.

## Allowing destruction during affected deployment

`--allow-destroy` is an explicit deletion-selection opt-in for
`atmos stack deploy --affected`. Reject it with `--all`, without `--affected`, or
on `stack destroy`. An inventory-only selection has no revision comparison from
which to recover removed definitions.

### Without the flag

Deploy affected components that remain defined. Report removed components and
stacks as requiring explicit teardown, and leave their infrastructure untouched.
Report the exclusion even when no deployable components remain.

### With the flag

Include removed components as teardown candidates alongside current-definition
deployment targets. A removed stack contributes its removed concrete components;
the manifest itself is not an infrastructure resource. Apply `--stack` to the
canonical identity in the appropriate snapshot, including the base snapshot for
deleted stacks.

The flag authorizes inclusion of teardown operations in reconciliation. Each component's
existing destroy handler owns confirmation and approval behavior. For example,
interactive Terraform/OpenTofu destruction can prompt. One worker preserves
exclusive stdin and live output; concurrent workers have no stdin and rely on the
component's noninteractive approval mechanism. There is no additional shared
prepare/review/execute approval transport in this initial design.

Recovery must still resolve immutable base/head SHAs, recover base configuration
and source provenance in isolation, and check deployed identities, protection,
and dependencies before any operations begin. Native engine previews and plan
verification remain the responsibility of the component handler.

An affected component that remains defined is a deployment target, not a deletion
candidate. Identity-equivalent renames must be detected before execution; a
removed Atmos name alone is insufficient evidence that its resources should be
destroyed. Resource deletion or replacement inside an ordinary Terraform apply
continues to be governed by engine plans and policy controls, not this flag.

### Eligibility and expansion

Use the deleted-component PRD's eligibility rules for locked targets, disabled
definitions, source uncertainty, surviving consumers, identity collisions, and
unsupported adapters. Display blocked candidates with their reasons. Do not
silently omit them or fall back to ordinary command execution.

Dependency expansion must never implicitly add a surviving consumer or shared
prerequisite to the deletion set. A surviving dependency on a removed target
blocks teardown under the governing PRD, even if that consumer is also selected
for deployment. Do not assume its deployment makes the old target safe to remove;
the required migration and renewed preparation must establish that separately.

## Lifecycle registry

Use `pkg/stack/lifecycle.Registry`, keyed by component kind and a fixed `Event`:
`Build`, `Test`, `Deploy`, `Destroy`, or `Validate`. The set of lifecycle events is
not dynamically extensible. A component type supports an event only when it
registers a handler. Duplicate registrations are rejected. Each handler declares
its structured component command bindings, hook preview events, optional approval
arguments, and execution resource/environment policy. The current CLI work wires
`stack deploy` and `stack destroy`; it does not repurpose existing `stack validate`.

Built-in component command packages register their own handlers during startup.
Configured custom handlers are added to invocation-local snapshots so loading one
project cannot pollute the registry of another. Stack orchestration does not have
a hard-coded list of component kinds or infer a deploy handler from a build command.

## Lifecycle capability discovery

Use registered built-in handlers and configured custom component commands explicitly defining
the requested verb. Include nested command paths and custom semantic inputs
(`provides: component` and `provides: stack`). Introduce an internal operation
descriptor containing kind, command path, and component/stack argument bindings.
Reject ambiguous bindings and unsatisfied required custom inputs before starting
execution.

Do not infer mappings such as Packer `build` or Ansible `playbook`. Report and
exclude unsupported types from ordinary current-definition selection. If explicit
dependency expansion requires an unsupported operation, fail before execution;
report and omit optional unsupported prerequisites.

Add explicit `destroy` aliases for the existing native Helm, Kubernetes, and
CloudFormation deletion operations, preserving their underlying behavior. Keep
Terraform and Helmfile's existing lifecycle semantics.

A command defining `destroy` qualifies for ordinary current-definition execution.
Recovered deleted-component execution additionally requires an adapter satisfying
the deleted-component PRD: source recovery, deployed identity, verification and retry handling. A custom `destroy` command alone is not that
contract. Engine-specific previews must accurately describe their guarantees;
a release list is not equivalent to a saved Terraform destroy plan.

## Graph and execution architecture

Reuse `pkg/dependency`, `pkg/scheduler`, and `pkg/process`. Keep new orchestration in `pkg/stack/lifecycle` and CLI wiring in `cmd/stack`; reuse existing affected APIs without adding files to `internal/exec`. Build graph identities
from `(kind, stack, component)` so names shared across types cannot collide. Reuse
`dependencies.components` parsing, kind and stack resolution, optional dependency
rules, and legacy compatibility. Preserve ordering through unselected intermediate
nodes and validate relevant cycles and required targets before scheduling.

Current-definition deployment uses the current graph with prerequisites first.
Current-definition destruction reverses that graph, with consumers first. The
expansion flags retain their original graph meaning when execution is reversed.
Unchanged prerequisites remain unexecuted unless selected or explicitly included.

Deleted-component teardown uses the recovered base graph, with selected consumers
before prerequisites, and checks incoming dependencies in both snapshots. Never
construct deleted-target ordering solely from the current configuration.

For mixed reconciliation, prepare and validate the complete selection before
execution. The initial execution design uses two phases: complete current-definition
deployments, then execute eligible deletions in reverse base-graph order.
Use concurrency within each phase. Any failed or skipped deployment prevents the
teardown phase; `keep-going` can finish independent deployment branches but does
not bypass that barrier. The phase barrier does not relax surviving-consumer checks.

Operation targets retain their action, source snapshot, working directory,
component identity, and applicable native-plan references. The dispatcher
must keep current and recovered configurations separate, including their backend,
workspace, tool versions, source paths, and execution-time authentication.

Invoke the current Atmos executable using structured arguments, preserving command
hooks, validation, authentication, and engine approval behavior while isolating
concurrent Cobra/Viper state. Forward effective configuration and identity settings;
consume selection and orchestration flags in the parent so each child executes
exactly one authorized operation. Resolve execution-time values when each node
starts so downstream deployments see newly produced outputs.

Reuse Terraform's shared-directory and plan-verification locks and concurrent
plugin-cache safeguards across workers. Process isolation alone does not prevent
filesystem races. Preserve scheduler readiness, failure propagation, cancellation,
and stable per-component outcomes. For recovered teardown, persist outcomes and
remote operation identifiers according to the governing PRD, and do not blindly
repeat successful destructive steps or hooks on retry.

## Manifest management and alias migration

Initial manifest operations require an explicit file:

```shell
atmos stack create --file stacks/deploy/dev.yaml
atmos stack delete --file stacks/deploy/dev.yaml
```

A logical stack can span multiple parent manifests. Do not infer that its name
identifies one file, or recursively delete all contributing files. See the
[stack composition design](top-level-stack-composition.md) and
[stack identity design](stack-name-identity.md).

### Scaffold-backed stack creation

`stack create` is a stack-specific entry point into the existing scaffold engine.
A scaffold is the recipe; creation applies that recipe and validates its stack
output. Reuse `scaffold.templates.<name>.source` and optional `description` in
`atmos.yaml`, the embedded catalog, direct sources, and `AtmosScaffoldConfig`.
Do not introduce a second template catalog or invoke Atmos subprocesses for generation.
`init` bootstraps projects; `scaffold generate` remains the general-purpose entry
point; `stack create` adds one stack to an existing project.

```shell
atmos stack create --file stacks/deploy/dev.yaml \
  --import catalog/defaults --component terraform:network --dry-run
atmos stack create --file stacks/deploy/dev.yaml \
  --template application-stack --set environment=dev
```

Without `--template`, use an embedded minimal stack scaffold. Repeated `--import`
values add imports; repeated `--component kind:name` values add disabled, incomplete
component declarations with empty vars. These flags do not generate component
source directories. Complete catalog references and custom template definitions
retain their intended enabled state. Do not infer dependency order from flag order
or component kinds. `--name` is an explicit identity override; otherwise preserve
the project's naming convention rather than imposing the filename as a canonical name.

Stack-compatible scaffolds declare `spec.stack.manifest`, identifying the rendered
manifest output relative to their target root. The stack command supplies reserved
`stack_file`, `stack_name`, `stack_imports`, and `stack_components` values. Templates
can use `{{ .Config.stack_file }}` for their manifest output. Other generated files
are rooted at the project base path, allowing optional component sources without
rewriting existing components. Unknown kinds, invalid paths, missing required
answers, ambiguous outputs, and existing destination files fail before hooks or
writes. The manifest must be discoverable under the configured stack patterns and
validate against the stack schema. Existing project files and generation records
remain untouched. Use the existing scaffold prompts, conditions, matrices, file
rendering, and hooks; expose `--interactive`, `--set`, `--ref`, and `--skip-hooks`.

Render a complete creation plan first, then validate all output paths and content.
`--dry-run` prints the generated contents and hook declarations without running
hooks or creating output files/directories. Real creation writes the same rendered
bytes using exclusive file creation. Scaffold hooks run around generation; they
are distinct from component deployment hooks. Generation does not deploy resources.

Record each stack generation independently under `.atmos/stack-generations/`,
keyed by its project-relative manifest path, with the template source/revision,
answers, and owned outputs. Creating dev and prod in one directory must not replace
each other's records or the project's init/scaffold history. Reuse scaffold update
machinery for future updates; this creation command never updates existing files.
General recursive composition of scaffold recipes and a stack-update CLI are later
increments; this change establishes a shared in-process render/validate/write plan
that can support them.

### Research and design rationale

- [Pulumi project creation](https://www.pulumi.com/docs/iac/cli/commands/pulumi_new/)
  creates a project and stack from a template and offers `--generate-only`.
  Keep project initialization and stack creation distinct while sharing templates.
- [Terramate stack creation](https://terramate.io/docs/cli/reference/cmdline/create)
  accepts imports, metadata, and ordering relationships and triggers generation.
  Make creation useful through imports and component declarations; preserve Atmos's
  explicit component dependency schema instead of deriving ordering from types.
- [Nx generator composition](https://nx.dev/docs/kb/composing-generators) invokes
  generators against a shared virtual filesystem, retaining dry-run behavior.
  Render and validate the entire file plan before applying it, through a shared Go API.
- [Copier updates](https://copier.readthedocs.io/en/stable/updating/) retain answers
  and reconcile template evolution with user edits. Its
  [composition configuration](https://copier.readthedocs.io/en/stable/configuring/#external_data)
  uses separate answer records for composed templates. Preserve independent template
  provenance and output ownership for every stack; never reuse a single directory's
  project-generation record for unrelated stacks.

### Manifest deletion

Delete only the named manifest; reject deletion if another manifest imports it and
identify the callers. Both manifest commands support `--dry-run`. Manifest deletion
does not operate on infrastructure, state, component sources, or imported files.

Deleting a manifest creates a configuration change discoverable by a subsequent
affected comparison. It neither executes nor approves its infrastructure teardown.
Users can destroy resources while the definition exists, or use isolated recovery
through `stack deploy --affected --allow-destroy` after removing it.

The existing `stack` group is not experimental. Retire the flat configuration
aliases in two release phases:

1. Deprecate and hide `stack get`, `set`, `delete`, and `format`, including alternate
   spellings. Preserve behavior and direct callers to `stack config ...`. Introduce
   `stack create`, `deploy`, and `destroy` without changing the old deletion behavior.
2. In a breaking release, remove the flat aliases and introduce manifest deletion
   under `stack delete`. Reject the old positional-path and `--component` syntax
   with migration guidance; never reinterpret those invocations as file deletion.

## Delivery and acceptance

Merge the deleted-component PRD first. Implement the common lifecycle orchestration
and current-definition commands, then integrate the isolated recovery workflow before enabling deleted-definition execution through `--allow-destroy`. Do not ship a flag that accepts the opt-in but
silently skips deleted targets. Its initial adapter coverage must be documented;
other deletion candidates remain visible and blocked.

The recovery artifact format, approval/resume transport, adapter rollout, and
disabled-definition policy remain governed by implementation decisions required
by the deleted-component PRD. This document settles the selection entry point,
not those unresolved contracts.

Acceptance tests must cover:

- Mixed-type and cross-stack chains, duplicate names across kinds, forward and
  reverse ordering, independent-node concurrency, and resource serialization.
- `--affected`/`--all` exclusivity, stack filters, bounded expansion, empty
  selections, unsupported operations, optional targets, and relevant cycles.
- Custom nested lifecycle commands, argument bindings, ambiguous commands,
  missing required inputs, authentication, hooks, and fresh dependency outputs.
- Failure modes, cancellation, grouped/streamed output, skipped dependents, and
  the deployment-to-teardown phase barrier.
- Deleted components and entire stacks reported without `--allow-destroy`, with
  no destruction calls; invalid flag combinations rejected before execution.
- With the flag, current components remain deploy targets and deleted components
  become recovery candidates; native confirmation remains delegated to the component handler.
- Immutable post-merge recovery, base-graph destruction order, surviving consumers,
  locked/disabled targets, unsupported adapters, and identity-equivalent renames.
- Native plan verification, source changes, stale plans, credential rotation,
  partial outcomes, retries, and remote operations still running after cancellation.
- No expansion flag adding surviving resources to the deletion set.
- Scaffold-backed creation using named/local/remote templates, required answers,
  reserved stack inputs, conditional files and matrices, stack discovery/schema
  validation, and identical preview/applied bytes. Hooks are previewed without
  execution, run around real writes, and respect skip controls. Separate stack
  records preserve existing init/scaffold records and other stacks' history.
- Manifest creation, overwrite refusal, import references, deletion of exactly one
  parent manifest, dry runs, and rejection of the former deletion argument shape.
- Existing type-specific commands and old configuration aliases retaining behavior
  until their explicitly breaking migration.

Update command help, documentation, examples, and screengrabs as behavior ships.
Document the distinction between removing a definition, destroying a current
component, and reconciling a removed component through isolated recovery.
