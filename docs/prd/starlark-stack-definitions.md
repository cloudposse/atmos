# Native Starlark Stack Definitions

**Date:** 2026-10-05

**Last Updated:** 2026-10-06

**Status:** Exploratory proposal — not approved for implementation

Explore whether Atmos should support defining stacks through native Starlark
objects, reusable functions, and composable capabilities. The intended audience
is stack authors and platform teams maintaining shared catalogs and automation.
The potential benefit is a coherent language for expressing infrastructure
composition while retaining Atmos's existing execution and inspection tools.

This document records an exploration, not an implementation commitment. All new
APIs and command forms below are illustrative; their names and signatures are
unsettled. Creating this PRD does not authorize building the feature.

The proposal is separate from [Starlark automation and command testing](starlark-automation-and-command-testing.md).
That work embeds scripting in automation and supports standalone executable
scripts. It does not establish native Starlark stack declarations, discovery, or
the object model proposed here.

The automation stack now includes a [shared Go service boundary](automation-sdk.md),
extension-based interpreter registration, decoded list/describe results, and direct
step-library calls. These can inform a future stack-authoring design, but querying
existing stacks from automation does not implement native Starlark stack definitions.
Git-hook scripts are covered separately in [Git-hook steps](git-hook-steps.md).


## Problem Statement

Stack authors and platform teams use YAML imports, catalogs, inheritance, and
templates to express reusable infrastructure, but parameterized composition can
be difficult to follow when it spans several files and related components.
The exploration motivating this PRD asks whether native objects and functions
could make that work easier without losing reliable listing, validation,
provenance, and affected detection. Without an improvement, teams continue paying
the authoring and debugging cost of complex composition; adopting another language
could introduce a greater maintenance cost instead.

The evidence is the product discussion captured here and existing repository
examples, not measured customer demand. Frequency, task-completion baselines, and
support impact have not been established. The proposed spike must test the
usability hypothesis before a feature commitment.

## Goals

These are proposed evaluation outcomes, not approved release targets. Measurement
methods and thresholds are defined in [Success Metrics](#success-metrics).

1. **Stack authors:** Complete representative composition and diagnostic tasks
    faster than with the equivalent YAML, without increasing incorrect outcomes.
2. **Infrastructure operators:** Preserve resolved component behavior and existing
    identity, parent-scope, and command contracts in every compatibility scenario.
3. **CI maintainers:** Identify all expected affected components and deletions in
    the comparison fixtures, with conservative callback-related results explained.
4. **Platform maintainers:** Inspect reproducible definitions without executing
    deployment actions, and explain effective values through useful provenance.
5. **Product and engineering:** Reach an evidence-backed go/no-go decision, including
    authoring benefit, evaluation cost, tooling gaps, and ongoing maintenance burden.

## Non-Goals

- Replace YAML or migrate existing projects automatically; gradual interoperability
  is needed to evaluate the authoring model without forcing adoption.
- Build a new provisioner or resource-state engine; Terraform and existing component
  implementations remain responsible for provisioning and resource planning.
- Reimplement Ruby or TypeScript's class systems; full method injection and lookup
  semantics are premature until simpler composition proves insufficient.
- Deliver all workflow, command, hook, and global-configuration authoring in the
  initial exploration; their broader lifecycle and bootstrap contracts need separate
  scope decisions, though compatibility must be considered here.
- Guarantee arbitrary program equivalence or automatic edits to computed source
  expressions; both exceed what declaration comparison and provenance can establish.

## User Stories

Ordered by proposed evaluation priority:

1. As a stack author, I want to instantiate a shared service with explicit parameters
    so that I can reuse its configuration without tracing templated imports.
2. As a CI maintainer, I want shared-definition changes to identify affected
    instances so that I can select the appropriate components for planning.
3. As an infrastructure operator, I want to inspect stacks without running their
    deployment actions so that discovery is usable before credentials or live outputs
    are available.
4. As a platform maintainer, I want to identify which default or override supplied
    a value so that I can diagnose an unexpected configuration.
5. As a catalog author, I want reusable presets and capabilities to remain isolated
    between instances so that configuring one service does not change another.
6. As an existing Atmos user, I want YAML and native definitions to coexist so that
    I can evaluate the feature one stack at a time.
7. As a CI maintainer, I want missing imports, conflicting definitions, and evaluation
    failures to produce errors so that a broken inventory is not reported as no changes.
8. As an infrastructure operator, I want an intentionally empty inventory to remain
    distinguishable from evaluation failure, and removed components to be reported
    without automatic destruction, so that I can review deletions deliberately.

## Requirements

Priorities below are proposed for evaluation, not an approved implementation
backlog. P0 defines the minimum evidence for a viable stack-authoring feature;
P1 identifies candidate improvements to validate next; P2 records possibilities
outside the initial scope. Detailed acceptance checks appear in
[Future acceptance scenarios](#future-acceptance-scenarios).

### Must-Have (P0)

| ID | Expected behavior and acceptance criteria | Constraints and dependencies |
|----|------------------------------------------|------------------------------|
| R1 | Authors define named stacks and components through native objects and reusable functions. Equivalent YAML and Starlark fixtures produce equivalent resolved behavior (AC-01, AC-05). | Reuse the stack processor and identity rules; dictionary-shaped configuration is not the primary authoring interface. |
| R2 | Inspection evaluates a complete, reproducible declaration graph without running actions. Unknown outputs remain references; errors cannot masquerade as an empty inventory (AC-02, AC-06, AC-10, AC-12). | Requires a restricted declaration host, tracked inputs, cancellation, and evaluation limits. |
| R3 | Affected computation combines effective declaration changes, implementation dependencies, and deletions. Attached behavior is tracked if callbacks are supported (AC-03 through AC-06). | Depends on existing affected and dependency processing; callback dependency representation remains a blocking design question. |
| R4 | Reusable configuration is isolated per instance and preserves explainable precedence. Invalid composition produces diagnostics (AC-07, AC-08). | Depends on merge semantics, source attribution, and a decision on the minimum preset/factory interface. |
| R5 | Mixed YAML/Starlark projects preserve parent ownership, duplicate handling, and existing command-wrapper contracts (AC-09, AC-11). | Requires loader integration and an audit of direct YAML consumers; native queries must not silently replace subprocess results. |

### Nice-to-Have (P1)

| ID | Candidate capability and acceptance criteria | Constraints and dependencies |
|----|---------------------------------------------|------------------------------|
| R6 | Executable stack files dispatch selected operations; direct invocation and ordinary CLI execution select the same component and context, and help executes no deployment callbacks. | Depends on discovery and standard-versus-file-defined verb decisions; see AC-02 and AC-11. |
| R7 | Behavioral concerns compose configuration, hooks, and commands; registration does not execute callbacks, conflicts are actionable, and implementation changes appear in affected results. | Depends on callback binding, inclusion/conflict semantics, and dependency tracking; see AC-02, AC-04, and AC-07. |
| R8 | Native object queries expose the resolved inventory without parsing CLI output, and diagnostics attribute values to declaration and call sites. | Requires defined query timing and useful provenance; see AC-08 and AC-11. |

### Future Considerations (P2)

| ID | Deferred capability and future acceptance criteria | Constraints and dependencies |
|----|----------------------------------------------------|------------------------------|
| R9 | Broader workflow and global-configuration authoring must preserve existing execution contracts and initialize without configuration/service cycles. | Separate scope decision and bootstrap design; not needed to validate stack authoring. |
| R10 | If native templates or method-extending mixins are justified, they must preserve identity and define binding, overrides, repeated inclusion, and compatibility errors explicitly. | Requires evidence that functions and presets are insufficient; Starlark supplies no native class system. |
| R11 | If selective reevaluation or persisted assemblies are added, cached and uncached evaluation must agree after transitive input and evaluator changes. | Optimize only after measuring full evaluation; cache identity must include all declaration inputs. |

## Success Metrics

No baseline has been measured. These thresholds are evaluation hypotheses, not
claims about current performance or commitments to ship. Record results at the
end of the proposed spike, using the same fixtures, machine, evaluator version,
and declared inputs for comparisons.

| Outcome | Measurement method | Proposed success threshold |
|---------|--------------------|----------------------------|
| Authoring benefit | Stack authors perform matched YAML/native tasks: instantiate a service, change a shared default, and trace an override. Record completion time and correctness, alternating task order. | Lower median completion time with no increase in incorrect results. Report participant count and small-sample limitations. |
| Compatibility | Compare normalized resolved components and run the identity, parent-scope, and wrapper acceptance fixtures. | All applicable P0 checks pass; no unexplained semantic difference. |
| Affected correctness | Compare expected and observed selections for default changes, overrides, refactors, source changes, callbacks, and deletions. | No missed expected components; helper-only equivalent refactors produce no declaration-only affected entries; conservative extras have a documented reason. |
| Inspection isolation and reproducibility | Observe attempted action/tool execution and repeat evaluation with fixed inputs. | Zero deployment-action or tool-install attempts; equivalent normalized graphs on repeated evaluation; invalid inputs fail visibly. |
| Provenance usefulness | Ask authors to identify the source and override responsible for each selected effective value. | Correct attribution for every tested value, including an overridden catalog default. |

Also report cold/warm evaluation duration and memory relative to YAML. Engineering
and product should set a performance budget from those measurements before feature
approval. There is no invented adoption, revenue, or support-reduction target for
an unapproved feature; a later pilot should establish eligible users, uptake, and
repeat-use measurements if the spike supports proceeding.

## Open Questions

Owners are roles, not assigned individuals. “Blocking” means before feature
implementation approval; exploratory prototypes may compare alternatives.

| Question | Owner | Timing |
|----------|-------|--------|
| Is the authoring benefit sufficient to justify a second stack language and its maintenance cost? | Product owner and stack authors | Blocking: go/no-go after spike. |
| How do discovery, file naming, multi-stack files, and direct execution work? Does Atmos supply verbs or do files register them? | CLI and stack-loader engineering | Blocking for R1/R5 discovery; dispatch decision before R6. |
| What is the smallest native object, preset, factory, and query interface, and how does it map to existing scopes? | API design and stack-processing engineering | Blocking for the applicable interface. |
| Are concerns functions, registered capabilities, or method-extending mixins? What are their conflict, ordering, dependency, repeated-inclusion, and receiver rules? | API design and runtime engineering | Blocking before including R7; full method extension deferred. |
| Which declaration inputs are allowed, and how are remote libraries, external context, secrets, and evaluator versions tracked? | Runtime and security engineering | Blocking. |
| How are symbolic references and callback dependencies represented across resolution, serialization, and subprocess boundaries? | Runtime and affected-processing engineering | Blocking for references and any supported callbacks. |
| How far does YAML interoperability extend, and which editing, formatting, provenance, and editor features are required initially? | Stack-processing and developer-tooling engineering | Blocking for minimum interoperability and diagnostics; richer tooling can follow. |
| What evaluation-performance budget is acceptable, and when is caching justified? | Runtime engineering and product owner | Set budget after spike, before feature approval; cache strategy non-blocking. |
| When, if ever, should workflows and global configuration gain native authoring? | Product owner and configuration engineering | Non-blocking; deferred proposal. |

## Timeline Considerations

No hard deadline, staffing assignment, or release commitment has been supplied.
Suggested phases are gated rather than dated:

1. Review this exploratory PRD and choose the minimum questions the spike will test.
2. If authorized, run the isolated evaluation spike and report the success metrics,
    compatibility gaps, author feedback, and maintenance implications.
3. Make a go/no-go decision. If proceeding, resolve blocking questions, select an
    initial scope, and produce an implementation-ready specification and estimate.
4. Consider a limited mixed YAML/Starlark pilot before broader rollout; use its
    results to decide whether P1 improvements or P2 proposals are warranted.

The work depends on the existing Starlark automation runtime, stack loader,
affected/dependency processing, and CLI contracts. It requires coordination among
their maintainers; no external-team delivery dates are assumed. Adding scope after
approval should trigger an explicit scope and schedule review.

## Proposed authoring experience

An executable `prod.stack` could look like this:

```python
#!/usr/bin/env atmos

prod = stack("prod")
prod.vars(stage = "prod", region = "us-east-1")

network = prod.terraform("network", component = "vpc")
network.vars(cidr = "10.0.0.0/16")

foo = prod.terraform("foo", component = "services/app")
foo.vars(
    replicas = 3,
    vpc_id = network.output("vpc_id"),
)
```

The desired executable-file experience is:

```shell
./prod.stack deploy foo
./prod.stack list
./prod.stack describe foo
```

The existing standalone shebang mechanism provides a starting point, but these
stack declarations and stack-scoped commands are proposed functionality. Whether
`deploy` is supplied by Atmos or declared by the file remains open. File location,
project discovery, stack selection, and configuration context must work equally
through direct invocation and the ordinary Atmos CLI.

`network.output("vpc_id")` would construct a symbolic reference, not read live
Terraform state during declaration. It would preserve the producing component,
output name, and dependency relationship until execution can resolve the value.
The graph must be discoverable without deployment-time outputs deciding which
components exist.

## Evaluation and shared model

The proposed boundary is:

```text
YAML processing ----------+
                          +--> shared stack model --> inspection and validation
Starlark declarations ----+            |
                                      +--> affected selection --> execution
```

The shared model would retain named components, effective configuration, symbolic
references, dependencies, parent ownership, and source provenance. Its exact
representation and the integration point in the current loader remain open.
Starlark should reuse Atmos processing semantics rather than create an independent
definition of inheritance or component execution.

Declaration evaluation would allow computation and tracked module/file inputs.
It would not expose unrestricted process execution, deployment operations, tool
installation, or live network lookups. Merely reusing the automation runtime's
full capability set would violate this boundary. Cancellation and evaluation
limits need to apply to entry files and their imports.

Callbacks may be registered during declaration, but deployment hooks and action
bodies run only during the selected operation. After declaration and resolution,
the graph should be stable for inspection and selection. Secrets and remote
outputs should remain references where possible; introspection must preserve
existing masking behavior.

Reproducibility requires recorded inputs and compatible evaluator semantics.
Versioned modules and tracked files must be resolved from each revision being
evaluated. Any external context must be explicit and held constant or supplied
as versioned snapshots; it must not silently depend on the current shell or cloud
environment. A persisted assembly or cache is optional, not required for an
initial experiment. Any cache would need to account for transitive inputs and
runtime versions.

### Identity and parent ownership

Preserve [Stack Name Identity](stack-name-identity.md): explicit name, name
template, name pattern, then filename determine one canonical identifier. The
illustrative `stack("prod")` supplies an explicit name; moving a file must not
silently change that identity. Component comparison uses canonical stack name,
component type, and component instance name, independently of helper names or
source locations. Renaming infrastructure identities is a separate concern from
refactoring source code.

Preserve [parent-scoped stack composition](top-level-stack-composition.md):

- Peer parent manifests resolve their imports and defaults independently. Sharing
  a logical stack identity does not deep-merge their global configuration.
- Inherited bases must be available in the owning parent's import graph, not
  discovered implicitly through another parent.
- Equivalent duplicate components remain valid under existing comparison rules,
  with the lexically first parent selected as canonical source. Conflicting
  duplicate definitions remain errors identifying the conflicting parents.

Mixed YAML/Starlark discovery must maintain those rules. The meaning of duplicate
definitions within one Starlark parent is a separate open design question.

## Imports, presets, catalogs, and concerns

These operations have different jobs:

| Concept | Proposed meaning |
|---------|------------------|
| Module loading | Bring reusable symbols into scope without implicitly registering components in a caller's stack. |
| Configuration preset | Apply reusable defaults or configuration layers. |
| Catalog factory | Instantiate named components, potentially including related components and references. |
| Component template | Optional reusable base definition corresponding to abstract component patterns. |
| Behavioral concern | Compose capabilities such as hooks, validation, commands, and their configuration. |

### Loading definitions and instantiating components

```python
# catalog.star
def web_service(stack, name, replicas = 2):
    app = stack.terraform(name, component = "services/app")
    app.vars(replicas = replicas)
    return app
```

```python
# prod.stack
load("./catalog.star", "web_service")

prod = stack("prod")
foo = web_service(prod, "foo", replicas = 3)
bar = web_service(prod, "bar")
```

Loading a library and instantiating its components are explicit separate actions.
Function arguments replace many templated-import use cases. Local module paths
should follow the existing file-relative Starlark loading convention. Cycles and
missing imports need actionable diagnostics. Remote libraries would need pinned,
tracked resolution; importing configuration still does not supply the referenced
Terraform or other component source code.

### Configuration presets

An illustrative preset factory could declare reusable defaults:

```python
def production():
    defaults = preset("production")
    defaults.vars(stage = "prod", deletion_protection = True)
    return defaults

prod = stack("prod", presets = [production()])
```

Configuration composition should preserve existing deep-merge and inheritance
semantics where applicable. Ordered layers could retain later-layer precedence,
with explicit component overrides, but the precise mapping of native operations
onto existing scopes needs validation. Applying a preset must not mutate shared
library state or another component's configuration.

A dedicated component-template object, with an API such as `extends=web`, is an
alternative to factories. Its value should be demonstrated before adding another
composition mechanism. Existing YAML abstract components remain relevant for
interoperability regardless of whether native templates are introduced.

### Behavioral mixins from a language perspective

Ruby modules compose methods into a class's lookup chain. Rails concerns also
support inclusion-time declarations and dependencies on other concerns.
TypeScript's documented mixin pattern extends a base class with state and methods.
These mechanisms compose behavior, not simply configuration values. See the
official sources in [Prior art](#prior-art).

Use “preset” or “configuration layer” for defaults such as production settings.
Reserve “mixin” or “concern” for capabilities such as observability or backup.
For example, a concern might configure metrics, attach a validation hook, and
register a logs command:

```python
# check_observability and stream_logs are action callbacks supplied by the library.
def observable(component):
    component.vars(metrics_enabled = True)
    component.before("deploy", run = check_observability)
    component.command("logs", run = stream_logs)

foo.include(observable)
```

In this sketch, inclusion runs a declaration function; it does not execute the
registered callbacks. This resembles concern inclusion behavior without claiming
to reproduce Ruby's object model.

Starlark has no native classes or inheritance. Composable functions operating on
host-defined objects fit the language. Full method injection, such as a concern
adding `foo.healthcheck()`, would require Atmos-owned binding and lookup semantics.
Method precedence, explicit behavioral overrides, repeated inclusion, concern
dependencies, and receiver compatibility remain open. Configuration's “last value
wins” rule must not silently become a rule for replacing executable behavior.

### YAML interoperability

An explicit bridge such as `prod.include_yaml("catalog/network")` could preserve
existing YAML imports, contexts, abstract components, and provenance. The spelling
and direction of interoperability remain open, including whether YAML may import
a Starlark-produced declaration. A bridge must not treat Starlark source as YAML
or silently reinterpret import paths. Migration should be possible one stack or
catalog at a time.

## Affected computation

Evaluate declarations at BASE and HEAD, then compare stable component identities,
effective configuration, and dependency relationships. Combine those differences
with component implementation changes and declared file/folder dependencies,
using the existing [component dependency model](component-dependencies.md).
Both graphs are relevant for removals and changed relationships.

| Change | Proposed interpretation |
|--------|-------------------------|
| Helper default changes from two replicas to three | Components using that default are affected; an instance explicitly requesting five remains unchanged. |
| Helper is refactored but produces equivalent declarations | Reevaluation is required, but source movement alone does not make those declarations affected. |
| Component implementation or declared input file changes | Existing source/dependency detection still applies even if declarations are identical. |
| Component or entire stack disappears | Report deletion, retaining the existing [deleted affected behavior](describe-affected-deleted-detection.md). Do not automatically destroy it. |
| Upstream component is affected | Dependent expansion can select consumers as potentially affected; it does not prove their resolved output values change. |
| Attached action or hook implementation changes | Track executable behavior separately from configuration and conservatively select attached components. |

Library-load dependencies and infrastructure dependencies are different graphs.
A library change invalidates evaluation of its users. A component output reference
describes a relationship between deployed components. They should not be treated
as interchangeable reasons for deployment.

An initial experiment can evaluate all definitions at both revisions. Selective
reevaluation is an optimization that requires transitive module, file, and other
input tracking. Comparisons should exclude incidental source paths and provenance
while preserving semantically meaningful identity and configuration.

Callbacks cannot be compared only by name. A conservative approach is to track
their defining modules and transitive imports, together with bound declaration
inputs; the precise representation remains open. This can produce false positives.
Arbitrary action bodies do not reliably reveal which components they may operate
on, so global commands need explicit associations or conservative treatment.

Syntax errors, import failures, incompatible evaluation, or unavailable required
inputs must produce diagnostics rather than an empty “nothing affected” result.
The feature does not promise to determine arbitrary program equivalence.

Keep three questions distinct:

| Operation | Question |
|-----------|----------|
| Affected selection | Which components need consideration because these revisions differ? |
| Provisioner plan or preview | What resource changes would the selected provisioner perform? |
| Drift detection | What has changed in the deployed environment? |

An affected component may legitimately produce an empty Terraform plan.

## Compatibility and broader surface

| Surface | Direction and unresolved work |
|---------|-------------------------------|
| Discovery, list, and describe | Discover native definitions through the shared loader and inspect the resolved model without running actions. File extensions, include/exclude patterns, and multi-stack files need design. |
| Component execution | Reuse provisioner lifecycles, authentication, hooks, environment handling, cancellation, and stack selection. Direct file invocation must carry the same context. |
| Validation | Validate both declaration evaluation and resolved configuration. Preserve meaningful template/function processing controls and source diagnostics. |
| Provenance | Retain declaration sites, factory call sites where useful, and contributing layers so users can explain effective values. |
| Source editing and formatting | Computed values have no general inverse edit. Define clear unsupported cases or explicit override workflows for stack set/delete operations; Starlark formatting needs separate support. |
| Imports and inheritance | Support gradual YAML interoperation without changing parent scope, identity, or duplicate rules. Keep module loading distinct from configuration merging. |
| Dependencies and affected | Preserve existing dependency selection and deletion handling; add tracked definition and behavior inputs where necessary. |
| Integrations and tooling | Audit CI, external stack consumers, auth-default discovery, completion, LSP, and direct YAML readers; supporting the main loader alone does not establish parity. |
| Workflows, hooks, and custom commands | Explore native construction of existing definitions and explicit registration of callbacks. Execution and serialization contracts remain open. |
| Global configuration | Starlark-authored configuration analogous to atmos.yaml is a further possibility. Bootstrap must avoid depending on services or command registries that configuration itself creates. |

### Existing command wrappers versus native queries

Today, Starlark `atmos.list(...)` wraps an Atmos CLI invocation and returns a
process result. It could discover Starlark-defined stacks once the invoked CLI's
loader supports them. In-memory declarations in the caller would not automatically
be visible to that subprocess; discovery or explicit context transfer is required.

A native query interface returning component objects is a separate proposal. An
illustrative `prod.components()` could query a completed declaration graph, while
a registry query could address multiple stacks. Neither requires silently changing
the return type or behavior of existing `atmos.list` calls. Query timing must avoid
recursively loading the inventory while that inventory is still being declared.

## Prior art

These are design lessons and recommendations for Atmos, not claims that the
systems provide an interchangeable Git-based affected implementation.

- **AWS CDK:** Synthesis produces a reusable assembly that supports inspection and
  deployment. This suggests an inspectable boundary behind native authoring.
  Cached lookup context illustrates how external inputs can be made reproducible;
  construct-derived logical IDs illustrate refactoring hazards. Its ordinary diff
  baseline is deployed stacks, distinct from comparing two Git revisions.
  Sources: [cloud assemblies](https://docs.aws.amazon.com/cdk/v2/guide/toolkit-library-configure-ca.html),
  [context and best practices](https://docs.aws.amazon.com/cdk/v2/guide/best-practices.html),
  [identity](https://docs.aws.amazon.com/cdk/v2/guide/identifiers.html),
  [diff](https://docs.aws.amazon.com/cdk/v2/guide/ref-cli-cmd-diff.html).
- **Pulumi:** Native inputs and outputs carry dependency information. Resources
  created inside apply callbacks can be absent from preview when outputs are
  unknown, motivating a complete declaration graph before execution. Atmos can
  adopt symbolic references while retaining its existing provisioners.
  Sources: [inputs and outputs](https://www.pulumi.com/docs/iac/concepts/inputs-outputs/),
  [apply limitations](https://www.pulumi.com/docs/iac/concepts/inputs-outputs/apply/),
  [engine model](https://www.pulumi.com/docs/iac/guides/basics/how-pulumi-works/).
- **Bazel:** Loading, analysis, and execution are separate phases. Functions can
  expand into declarations while actions remain deferred. This is a useful
  Starlark model for building inspectable graphs.
  Source: [evaluation model](https://bazel.build/extending/concepts).
- **Ruby/Rails and TypeScript:** Behavioral composition is broader than merging
  defaults. Borrow explicit capability composition while making conflicts and
  lifecycle effects visible.
  Sources: [Ruby modules](https://docs.ruby-lang.org/en/master/Module.html),
  [Rails concerns](https://api.rubyonrails.org/classes/ActiveSupport/Concern.html),
  [TypeScript mixins](https://www.typescriptlang.org/docs/handbook/mixins.html).
- **Starlark:** Functions and host-defined data types support a native domain API;
  the language does not supply a class system. Shared module freezing also makes
  mutable global configuration builders a poor composition contract.
  Sources: [language and embedding overview](https://github.com/google/starlark-go),
  [implementation and freezing](https://github.com/google/starlark-go/blob/master/doc/impl.md).

## Alternatives and Risks

Alternatives include keeping YAML definitions with Starlark only for automation,
generating YAML as a separate build step, or adding native declarations with a
shared resolved model. Generating YAML is simpler to integrate but adds artifact
management and provenance problems. A fully programmable application framework
offers more flexibility but expands lifecycle, dependency, and bootstrap concerns.

The central risks are a larger public API, hidden composition effects, incomplete
source tooling, expensive evaluation, nondeterministic inputs, and behavioral
changes missed by configuration-only comparison. A useful prototype must measure
these costs alongside authoring improvements.

## Proposed evaluation spike

Use the existing [demo-context stack family](../../examples/demo-context/stacks/)
as the baseline: it contains shared defaults, a catalog, regional mixins, and
environment-specific declarations. Preserve actual resolved values even where
filenames are only conventions. Add isolated experimental cases for a symbolic
component dependency and a behavioral concern, which the baseline does not cover.

Compare native authoring with YAML for readability, effective configuration,
diagnostics, provenance, refactoring, and evaluation cost. Exercise both revisions
after a preset change, a helper-only refactor, a callback change, and a deletion.
Keep baseline fixtures intact and do not deploy infrastructure for the experiment.

The spike should produce a go/no-go recommendation and a smaller proposed API,
not commit to replacing all Atmos configuration. It is future work, not part of
the documentation change introducing this PRD.

## Future acceptance scenarios

These scenarios evaluate a future prototype; they are not claims of implemented
support or tests required for this documentation-only change.

- [ ] **AC-01:** Equivalent YAML and Starlark inputs resolve to equivalent component
  behavior, allowing format-specific provenance differences.
- [ ] **AC-02:** List, describe, validation, help, and affected evaluation never
  execute deployment callbacks or install tools merely to discover definitions.
- [ ] **AC-03:** Changing a shared default affects its consumers while explicit
  overrides remain stable; an equivalent helper refactor causes no
  declaration-only affected entry.
- [ ] **AC-04:** Component source, attached callback code, transitive callback
  modules, and bound callback inputs are considered even when ordinary
  configuration is unchanged. Callback cases apply when that capability is included.
- [ ] **AC-05:** Moving declarations between helper files preserves explicit
  stack/component identity; removals are reported without automatically
  scheduling destruction.
- [ ] **AC-06:** Symbolic output references remain inspectable without live state
  and contribute dependency edges; dependent selection reports potential impact
  accurately.
- [ ] **AC-07:** Reusing a preset or supported concern never mutates another
  instance. Conflicts, cycles, missing imports, and unsupported applications have
  actionable diagnostics.
- [ ] **AC-08:** Provenance explains a default and its override, including useful
  declaration and call-site locations; computed source edits do not silently
  corrupt code.
- [ ] **AC-09:** Mixed YAML/Starlark parents preserve independent scope, equivalent
  duplicate handling, canonical ownership, and conflicting duplicate errors.
- [ ] **AC-10:** Repeated evaluation of the same revisions and declared inputs is
  reproducible. Missing inputs, evaluation failures, and resource limits fail
  visibly rather than appearing as an empty inventory or no affected components.
- [ ] **AC-11:** Existing subprocess-based command wrappers retain their contracts.
  Native object queries and direct file execution, if included, use a clearly
  defined stack context consistent with ordinary CLI selection.
- [ ] **AC-12:** A successfully evaluated empty stack or inventory is distinguishable
  from a load failure and follows existing empty-result behavior. Deleting the last
  component remains visible in the base/head comparison.
