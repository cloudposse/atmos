---
name: component-development
description: "Atmos core component development: adding or changing native component types, component registry providers, commands, stack schema, docs, examples, DAG/affected behavior, auth, hooks, source/provisioning, and tests"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Component Development

Use this skill when adding or changing an Atmos native component type or shared component behavior.

## Start With The Existing Contract

Inspect comparable component types before designing:

```bash
rg -n "ComponentType|SectionName|components\\..*base_path" pkg/config pkg/schema
rg -n "component.Register|ComponentProvider|Execute\\(" pkg/component cmd internal
rg -n "process.*ComponentsIndexed|ExecuteGraph|dependencies.components" internal pkg
```

For stack-surface decisions, compare Terraform, Helmfile, Packer, and Ansible. Reuse shared sections when they already exist:

- `metadata`
- `vars`
- `env`
- `settings`
- `hooks`
- `auth`
- `dependencies`
- `generate`
- `source`
- `provision`

Do not invent nested settings for component-owned inputs. If an instance is `components.<type>.<name>`, component-specific inputs belong directly under that instance, e.g. `components.kubernetes.argocd.paths`, not `settings.kubernetes.paths`.

## Required Wiring

For a new component type, update all applicable surfaces in one PR:

- Constants: `pkg/config/const.go`
- CLI/default config structs: `pkg/schema/schema.go`
- Defaults: `pkg/config/default.go`
- Config path resolution: `pkg/config/config.go`, `pkg/utils/component_path_utils.go`
- Component registry provider: `pkg/component/<type>/`
- Command package: `cmd/<type>/`
- Root side-effect imports: `cmd/root.go`
- Source provisioning base path support: `pkg/provisioner/source/`
- Hooks/events if lifecycle hooks are supported: `pkg/hooks/`
- Affected detection: `internal/exec/describe_affected_*`
- DAG/bulk execution if `--all` or `--affected` is supported
- Embedded CLI usage examples: `cmd/markdown/atmos_<command>*_usage.md`
- Runtime stack schemas: `pkg/datafetcher/schema/atmos/manifest/1.0.json` and `pkg/datafetcher/schema/stacks/stack-config/1.0.json`
- LSP hints if stack authoring changes: `pkg/lsp/server/`
- Docs: command docs, stack component docs, `atmos.yaml` config docs
- Examples under `examples/` for user-visible component types

If a public website schema file exists in the checkout, update it too. Do not assume it exists; verify with `find website/static -path '*schema*'`.

## Command Behavior

Commands should be Atmos-native, not thin binary wrappers, when the implementation owns behavior.

- Use the component registry provider rather than hard-coded command logic.
- Use `flags.NewStandardParser()` for command-specific flags.
- Preserve `--identity` and profile handling.
- For bulk operations, allow zero positional component args with `--all` or `--affected`.
- Reject ambiguous forms such as component arg plus `--all`.
- Use dotted hook events such as `before.<type>.<operation>` and `after.<type>.<operation>`.
- Add embedded usage examples for every public subcommand.

If a command name like `kubectl` or `kustomize` is used as a provider, be explicit whether it means CLI execution or compatible behavior. For SDK-backed providers, do not add a `command` override unless a binary is actually executed.

## Auth

Thread Auth through stack processing and execution:

- Parse global `--identity` into `ConfigAndStacksInfo.Identity`.
- Use the same auth setup path as existing component commands when stack YAML functions or integrations need credentials.
- Keep component-level `auth:` in the stack schema.
- For Kubernetes-like SDK clients, create the client after Auth has prepared environment or kubeconfig integration output.
- Prefer plain `ambient` for local examples that should respect the surrounding environment without prompting for provider login. Use concrete provider identities for examples that actually require a cloud or service-specific authentication flow.

## DAG And Affected

If the component supports `--all` or `--affected`:

- Build nodes from `components.<type>`.
- Skip abstract, disabled, and locked components where applicable.
- Use `dependencies.components` as the primary dependency source.
- Keep legacy `settings.depends_on` only as compatibility.
- Process in topological order.
- For `--affected`, run affected detection first, filter to the component type, then graph-filter selected nodes.
- Include dependencies by default so prerequisites run before selected nodes.
- Make `--include-dependents` explicit for downstream components.

Affected detection must include both implementation path changes and stack config changes. For new component-specific top-level sections, add affected reasons for those sections.

## Schemas And Docs

Schema and docs are part of the feature, not follow-up polish.

Update stack schemas with the exact component shape. Validate Atmos-owned fields strictly, but keep domain resources permissive when a complete third-party schema would be brittle. Example: validate Kubernetes manifest entries have `apiVersion` and `kind`, but do not model the entire Kubernetes API.

Docs must cover:

- `atmos.yaml` configuration
- Stack component configuration
- Every command and flag
- Reusable sections supported by the component
- Auth behavior
- Hooks and lifecycle events
- Dependencies, `--all`, and `--affected`
- Examples for file paths, inline manifests/config, render/generate behavior, source/provisioning when supported

Use the local docs skill for website docs conventions.

Command and component docs must explain the user outcome before implementation
details. Do not open intros with mechanical phrasing such as "Use this command
to..." or with internal architecture such as providers, SDKs, DAGs, or stack
processing unless that detail changes how the user should operate the feature.
Lead with what the user can accomplish, when they should reach for the command
or configuration page, and the next action they can take.

## Examples

Examples should be runnable and reflect the intended user model:

- Use local infrastructure like k3s when possible.
- Keep command examples working without cloud credentials unless the example is explicitly cloud-auth focused.
- If Auth is part of the feature, include a local identity path and a commented real-cloud integration example when useful.
- Add separate examples for materially different providers or modes.

## Validation

Run focused tests first:

```bash
go test ./pkg/component/<type> ./cmd/<type>
go test ./pkg/config ./pkg/schema ./pkg/hooks ./pkg/provisioner/source
go test ./internal/exec ./pkg/lsp/server
jq empty pkg/datafetcher/schema/atmos/manifest/1.0.json
jq empty pkg/datafetcher/schema/stacks/stack-config/1.0.json
```

For docs:

```bash
pnpm --dir website exec prettier --check "docs/**/*.{mdx,json}"
```

For examples:

```bash
(cd examples/<name> && atmos validate stacks)
(cd examples/<name> && atmos <type> render <component> -s <stack>)
```

If a validation failure is pre-existing or requires external services, say that explicitly and include the narrow command that failed.
