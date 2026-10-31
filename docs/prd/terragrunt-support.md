# Terragrunt component plugin

**Status:** Implementation in progress. CLI discovery and native execution are wired to the SDK plugin on this branch. Validation covers local modules and local state. The plugin is not published. The requirements below define the complete target, not a compatibility claim.

**Decision:** Ship Terragrunt as a separately versioned native plugin. Use Terragrunt Go modules to interpret HCL and Atmos to execute native Terraform components. Provide executable mode as a separate compatibility path.

This carries forward the [checkpointed pilot](https://github.com/cloudposse/atmos/tree/5fea956fd6061efae7c8c97ce76d3a2491446b9b/tools/terragrunt-poc). The [component plugin PRD](plugins/extensions/component-engines.md) defines shared contracts. Keep pilot evidence in engineering reports, not in the migration guide.

## User outcome

Users retain Terragrunt units, explicit stacks, HCL, module sources, and state. With the plugin installed and discovery enabled, they can list instances and run Atmos without generating YAML for every unit. Conversion to native Atmos YAML remains optional.

Native mode can use Terragrunt Go dependencies but must not require its executable. Do not translate Terragrunt functions into Atmos functions or maintain another evaluator.

Do not infer a compatibility percentage from block or fixture counts. Measure unchanged adoption against a documented repository corpus and pinned Terragrunt version. Report language evaluation, execution semantics, and Atmos integration separately.

## Modes

| Mode | Configuration | Executor | Scheduling |
|---|---|---|---|
| Native, primary | Terragrunt Go evaluator in the plugin. | Atmos Terraform/OpenTofu engine. | Atmos selection and dependency graph. |
| Terragrunt executable, optional | Terragrunt. | Explicitly configured Terragrunt binary. | One declared owner; never nest all-unit Terragrunt scheduling inside per-unit Atmos scheduling. |

Reject unsupported native behavior before execution. Never fall back silently. Test executable mode separately; passing it does not prove native compatibility.

## Reuse existing capabilities

| Need | Reuse | Preserve or add |
|---|---|---|
| Execution | `pkg/component/terraform`, registry, lifecycle. | Per-operation plugin preparation; no global override. |
| Generated files | `pkg/terraform/generate`. | Literal bytes, ownership signatures, `if_exists`, disable behavior. |
| Sources | `pkg/provisioner/source`, vendoring, toolchain. | Upstream path/auth semantics and cache state safety. |
| Dependencies | Dependency graph, bulk executor, deferred values. | Ordering/output edges and fresh outputs per operation. |
| State/output | Native state reader and Terraform output handling. | Plugin backend/workspace identity; test both functions. |
| Authentication | Scoped identities and environments. | Equivalent upstream cloud and credential semantics. |
| Hooks, mocks, retries | Existing lifecycle and services. | Terragrunt ordering, allowlists, merge rules, errors. |
| Discovery/affected | Resolved stack map, naming, watches, dependency expansion. | HCL/stack provenance for both revisions. |
| UI/reporting | Masked I/O, `AtmosLogger`, `ui.*`, Terraform CI. | HCL diagnostics and consistent instance/engine identity. |
| Delivery | Merged SDK, OCI, locks, policy, local builds, catalog. | Component capabilities and adapters. |

Generation, locals, hooks, mocks, retries, and caching already exist. Matching feature names do not establish matching semantics.

## Explicit configuration

Use a structured section, not a boolean compatibility list. Target configuration:

```yaml
components:
  terraform:
    terragrunt:
      enabled: true
      mode: native
      discovery:
        enabled: true
        included_paths: [live/**]
        excluded_paths: [vendor/**]
```

Enablement and automatic discovery default to false. Installation alone starts no process and scans no monorepo. Scope traversal to literal prefixes of included paths; prune exclusions and caches. Allow explicit `terragrunt://` imports without automatic discovery.

Provide settings for the executable/version in executable mode, project/discovery paths, cache location, and supported provider options. Keep package source/version/path in plugin package configuration. Native Terraform/OpenTofu selection stays with Atmos toolchain configuration. Resolve paths against the Atmos base directory. Document schema and precedence; preserve explicit false and empty values.

## Discovery and inventory

- Discover standalone units and explicit `terragrunt.stack.hcl` definitions, including nested stacks and values.
- Expand stacks without treating generated cache directories as separate projects.
- Dispatch `terragrunt://` through a generic import interface. Share the evaluator with discovery.
- Add definitions before commands require YAML stacks. Reuse naming precedence and path fallback.
- Name automatically discovered units from paths: the component is the unit directory name; the stack is the directory above it (`default` and `root` at the base path); an explicit stack file names the stack after its own directory and the component after the unit's path inside the stack. Name templates and patterns do not apply to discovered stacks.
- An import merges its units into the importing stack as components. Imports never create a stack. A mixin's import applies to every stack that imports the mixin; the same unit may be imported by several stacks. Imports ignore `included_paths` and `excluded_paths`, work without automatic discovery, and fail when they select no unit. An imported unit is removed from automatic discovery.
- Keep logical coordinates stable across discovery, generated directories, and machines. An imported unit's coordinates are the importing stack and the unit's name.
- Use one pattern language for `included_paths` and `excluded_paths`: doublestar globs relative to the base path that select the matched path and everything below it. Report an include pattern that selects nothing, a `terragrunt.hcl` without `terraform.source`, and skipped symlinks as warnings.
- Follow Terragrunt v1.2.1 for symlinks: do not follow directory links, treat a symlinked `terragrunt.hcl` or `terragrunt.stack.hcl` as a unit at the link's location, and report what is skipped.
- Fail on two units with one component name in one stack, and on a unit that shares its name with a YAML component, naming both definitions.
- Do not attribute a plugin-provided component to an unrelated YAML manifest in provenance output.
- Leave source configuration and state unchanged during listing. Report denied effects.

Imported units have type `terraform`. Native mode reports engine `terraform` or `opentofu`; executable mode reports `terragrunt`. Do not infer the engine from HCL.

The default instance table shows `Component`, `Stack`, `Type`, `Engine`. Expose type/engine in structured output and custom columns. Preserve user column overrides.

Lists, describe, selection, affected, execution, and Pro must use the same resolved instances. No separate Terragrunt inventory is acceptable.

## Native evaluation and preparation

- Reuse Terragrunt functions, includes, locals, and merge behavior through its Go evaluator.
- Preserve exact cty values, unknowns, sensitivity, and generated bytes across the wire.
- Discover graphs without dependency state or mocks. Reevaluate selected units before execution with the real command and fresh outputs.
- Map `dependencies.paths` to ordering edges. Map enabled `dependency` blocks to ordering and output edges.
- Preserve mock command allowlists and merge rules. Report dependencies outside adoption scope until explicit mapping exists.
- Use Atmos forward order for apply and reverse order for destroy.
- Resolve sources and materialize declared backends even when Atmos backend generation is disabled.
- Preserve relative paths, upstream working-directory functions, workspace identity, and inputs.
- Check Terragrunt and Terraform/OpenTofu version constraints before execution.
- Apply the exact reviewed saved plan. Bind provider/options/source/state identity to the artifact.
- Ordinary plan exit 0 is success without an asserted change count. Detailed plan exit 2 means changes, not failure.

Discovery defaults to denying commands, network, secret decryption, writes, and state reads. Fail with the source when an effect is required. Do not omit units or substitute empty values. Stack-generation writes are limited to their declared workspace and do not authorize hooks or infrastructure operations.

Native plugins are trusted code. Broker grants and evaluator controls are not an OS sandbox. Test actual effect interception through nested includes and stack expansion.

## Atmos execution contracts

Treat environment, identity, toolchain, and workdir behavior as release gates.

- Resolve the selected Atmos identity before native preparation can read outputs, decrypt data, or run commands. Honor explicit, default, and disabled identity selection. Stop on authentication failure.
- Pass the sanitized subprocess environment to the evaluator. Do not merge ambient credentials back into it. Distinguish a missing environment from an explicitly empty environment.
- Use the same component environment, toolchain PATH, managed Terraform RC, registry trust, and cache policy for preparation and Terraform execution. Preserve empty overrides and last-value precedence.
- Pass an absolute executable path after toolchain resolution. HCL commands must use the invocation PATH. Never mutate the plugin process environment to switch identities or tools.
- Resolve explicit relative HCL commands from their Terragrunt unit directory. Keep `get_terragrunt_dir()` independent of the Atmos execution workdir.
- Keep credentials and temporary RC files scoped to the operation. Clean temporary files after preparation, provisioning, or execution fails.
- Test two identities and two tool versions in consecutive and concurrent invocations. Test cancellation and auth/tool failures before evaluator effects.
- Preserve configured workdir and source settings when replacing the evaluated module URI. Place generated files, variable files, and plans in the resolved Atmos working directory.
- Isolate discovered instances by stack and component. Do not let two stacks overwrite the same module's generated backend.
- Keep state identity independent of execution workdir. Relative Terragrunt backend paths must retain their upstream base. Workdir refresh must not move, delete, or recreate state.
- Test ordinary placement, automatic workdirs, absolute and project-relative working directory overrides, repeated runs, and conflicting directory ownership. Check both generated bytes and the backend used by Terraform. A rejected second owner must leave the first owner's artifacts and state unchanged.

Read-only discovery precedes component authentication. If discovery requires an identity-dependent effect, report the unsupported requirement. Do not use ambient credentials as an implicit substitute for the selected identity.

## State and cache ownership

Resolve state independently of preparation. `!terraform.state` must work before a Terraform command runs for the target. Do not run hooks, write files, or read unrelated outputs to locate state. Reject backend expressions requiring those effects.

Reuse Atmos state reading, sensitivity, and invalidation after successful upstream operations. Test `!terraform.output` separately. Preserve default/named workspace paths and freshness across mixed YAML/HCL updates.

`.terragrunt-cache` can contain local state. Distinguish state, downloaded modules, output-read scratch directories, and generated `.terragrunt-stack` units. Adoption and refresh must not move/delete state. Preserve `get_working_dir()` or reject the affected configuration.

The pilot found a separate named-workspace dependency gap: Terragrunt output reads can require its initialized backend directory while Atmos initializes another. Resolve this before accepting those graphs. Standalone workspace handoff and state-function tests do not prove HCL-to-HCL dependency behavior.

## Affected detection

`describe affected` and `terraform <operation> --affected` must share identities and source relationships.

The current native implementation creates separate discovery sessions for HEAD and BASE. It retains the invocation's trusted plugin, scope, environment, and cancellation. It restores HEAD before dependent expansion and execution. Acceptance covers local HCL and module watches, nested stack provenance, changed dependency targets, additions, removals, renames, mixed YAML/HCL graphs, stack filters, and dependent depth. Reads in unevaluated input expressions and arbitrary dynamic glob membership remain open requirements below.

- Track primary HCL, includes, cross-directory reads, `file`, `mark_as_read`, modules, and optional reads.
- Watch directories and glob membership to detect new/deleted files.
- Track root/nested stacks, values, catalog sources, generated origins, and changed remote references.
- Distinguish file watches, output edges, and ordering edges. Define dependent expansion.
- Evaluate each revision in its own root. Do not read current HCL or live state for an older revision.
- Test additions, removals, renames, globs, filters, mixed graphs, and selection reasons.
- Preserve `--include-dependents` and existing stack/component filters.

Hierarchy helps provenance but does not prove dependencies stay within an ancestor subtree.

## Flags, I/O, and UI

Use shared static metadata and the unified Atmos flag parser. Declare modes, commands, types, defaults, aliases, sensitivity, and deprecations. Separate provider options from Terraform argv. Validate unsupported options before effects. Bind discovery-relevant options to graph/cache/plan identity.

Atmos owns selection, `--affected`, `--ui`, and native scheduling. Explicit executable passthrough must reject ownership conflicts. Never silently forward flags or fall back.

Preserve stdout, stderr, logs, diagnostics, lifecycle, and outcomes as distinct channels. Stream while running. Adapt Terragrunt logs through `AtmosLogger`, with explicit severity conversion, timestamp, source, stack, and component. Mask before presentation.

Optionally map selected known messages to `ui.*`; unmatched messages stay in normal logging. Logs never establish operation success. Define stdin, TTY, backpressure, cancellation, deadlines, and crashes. Preserve native Terraform `--ui`. Never automatically replay apply after a lost connection.

## Compatibility gaps requiring acceptance

| Area | Requirement |
|---|---|
| Public Go API | Upstream public environment/stack APIs or an explicitly maintained bridge. The pilot facade wraps internal APIs and is not directly importable from an independent module. |
| GCS backend creation | Equivalent provisioning or an existing bucket. S3/Azure support does not prove GCS parity. |
| Generate ownership | Signatures, overwrite/skip/error policies, and disable behavior. |
| `errors.ignore` and signals | Define/test native semantics; generic retries are insufficient. |
| `prevent_destroy`, exclusions, feature flags | Enforce before effects or reject. |
| Hooks and extra arguments | Preserve order, command, environment, failure behavior, and paths. |
| Remote sources/cloud state | Real source/auth/backend tests before parity claims. |
| SOPS/credential-dependent evaluation | Phase-specific effects and scoped credentials. |
| Named-workspace dependencies | Resolve the separate directory/backend gap. |
| Explicit stacks | Remote catalogs, values, generation policies, nested provenance. |
| UI/platforms | Masking, cancellation, terminal and path tests on Linux/macOS/Windows. |

## Pro and CI

Reuse Terraform CI for native units. Preserve inherited Pro settings and enablement. Test inventory/affected uploads, locking, drift, and runner plan/apply with the same artifact. Restore pinned plugins, HCL, sources, and scoped credentials on the runner.

Local contracts do not establish live Pro compatibility. Test the configured end-to-end flow before claiming it. External engine reporting needs separate outcomes and adapters.

## Optional setup and migration guide

Provide `atmos migrate terragrunt <path>` for custom grouping/scope. Discovery must not require it. Accept directories, units, and explicit stacks. Preview writes/installs nothing. Preserve existing Atmos configuration, allow a stack name, detect collisions, and make repeated setup a no-op. Import stacks once, not also their generated units.

Setup must not evaluate HCL functions, run hooks, read outputs, generate stacks, or run plan/apply/destroy. Mark evaluation-dependent checks pending and print describe/plan commands. Do not silently widen scope for dependencies.

Write the migration guide for the completed experience: adopt HCL unchanged through the plugin, or optionally convert to Atmos YAML. Present executable mode separately. Keep pilot results/status in engineering docs. Verify state identity and a plan before apply.

## Delivery gates

1. Complete independent contracts, typed values, static metadata, flags, and host adapters. Preserve existing plugins and built-ins.
2. Run the advanced native fixture through an installed SDK plugin and stock Atmos, without temporary host patches.
3. Extend acceptance to nested stacks, discovery, mixed YAML/HCL, state functions, affected, saved plans, reverse destroy, cancellation, and crashes.
4. Port field regressions into the default runner. Add differential and explicit unsupported-behavior tests.
5. Verify Pro/CI, supported platforms, and release installation. Require at least 90% patch coverage per layer.

Do not report completion while host wiring, compatibility tests, or required product flows remain unfinished.
