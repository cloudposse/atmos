# Migrating from Fogg

Use this recipe for `fogg.yml`, Fogg-generated Terraform, and Fogg-generated Make/CI tasks.
The goal is equivalent behavior managed entirely by Atmos and Terraform. Explicit configuration
counts as parity. Retaining generated Terraform is the adoption step; retaining Fogg as a
runtime dependency is not the completed migration. Automatic conversion/import tooling is not
required unless the user separately requests it.

Read the [Fogg migration guide](https://atmos.tools/migration/fogg) for complete configuration
examples and the capability matrix. In the Atmos repository its source is
`website/docs/migration/fogg.mdx`. The assessment uses Fogg v2 configuration at source revision
[`6b8979176e6fa20c3745cbb1d71393becf935120`](https://github.com/chanzuckerberg/fogg/tree/6b8979176e6fa20c3745cbb1d71393becf935120).
Compare the user's actual revision and generated files: the published configuration reference
omits capabilities such as extra templates and provider mappings.

## Discover the Existing Contract

Inspect `fogg.yml`, generated `.tf`/`.tf.json`, Makefiles, CI, plugin scripts, and templates.
Inventory each global, account, and environment root, including any TFE-management root:

- Component directory, actual Terraform CLI workspace, complete backend location and locking.
- Terraform/provider versions, lockfiles, module labels and sources, provider aliases and mappings.
- Inputs/defaults, locals, tags, account restrictions, assume roles, output names and sensitivity.
- Remote-state consumers, task ordering, extra files, custom binaries/providers, and authentication.

Fogg's directory term "workspace" is not proof of the Terraform CLI workspace. Check
`terraform workspace show` and `TF_WORKSPACE` using the existing setup. `fogg apply` generates
files; `atmos terraform apply` deploys infrastructure. Never substitute those commands by name.

## Adopt One Existing Root In Place

Keep generated files and relative module paths. Point Atmos at the existing layout, disable
backend/file generation, and leave `providers`/`generate` unset until deliberately transferring
ownership of those declarations. For roots under `terraform/`:

```yaml
# atmos.yaml
base_path: .
components:
  terraform:
    base_path: terraform
    auto_generate_backend_file: false
    auto_generate_files: false
    apply_auto_approve: false
stacks:
  base_path: stacks
  included_paths: [deploy/**/*]
```

```yaml
# stacks/deploy/development.yaml
name: development
components:
  terraform:
    webserver:
      metadata:
        component: envs/development/webserver
        terraform_workspace: default
```

Use `default` only if it is the recorded workspace. Keep the existing executable and credentials
available; pin that version with `dependencies.tools` when adding toolchain selection.

```shell
atmos validate stacks
atmos describe component webserver -s development
atmos terraform plan webserver -s development
```

Compare with the established plan against the same state, inputs, credentials, versions, and
refresh conditions. Investigate empty-state selection, replacements, or account changes before
cutover. Do not introduce state imports/moves, Terraform upgrades, or new backend conventions as
part of adoption. Preserve remote workspace names and execution mode; use `--skip-planfile`
where the existing remote execution setup does not support saved plans.

## Replace Fogg's Ongoing Responsibilities

For every used capability, supply maintained configuration or identify a concrete remaining
implementation. Do not claim parity merely because a shell command can be executed.

| Existing behavior | Migration decision |
|---|---|
| Global/account/environment roots and defaults | Components mapped to existing paths; stacks, imports, and inheritance for shared configuration |
| Backend generation | Keep HCL permanently or transfer the exact backend to `backend_type`/`backend`; remove only the old backend block before enabling generation |
| Provider generation | Keep provider blocks or use `providers`; retain `required_providers`, aliases, roles, account restrictions, and tags |
| Module calls and output forwarding | Maintain the wrapper's module labels, provider maps, and sensitive outputs; use scaffolds for future wrappers |
| Remote-state access | Keep Terraform data sources or declare inputs populated by `!terraform.output`; ordering is a separate concern |
| Extra/component templates | Imports/inheritance for configuration, scaffolds for initial files, `generate` for continuously regenerated files |
| Make/CI checks | A custom command such as `atmos check-webserver` with ordered auth, fmt, lint, validate, and plan steps; hooks for checks on each invocation |
| EKS | Atmos Auth `aws/eks` integration and `atmos aws eks token` exec credentials where suitable; preserve Terraform provider wiring and cluster permissions |
| Custom downloads/providers | Toolchain registries for CLI binaries; Terraform registries or provider mirrors for provider plugins |
| TFE management | Manage the existing TFE root and its catalog/locals or inputs explicitly; defining a stack does not create a TFE workspace |

Stop generation for migrated roots before editing their formerly generated files; Fogg can
regenerate files across the repository. Demonstrate a future change, such as a new wrapper
argument/output or environment, using maintained Terraform/stacks with no Fogg invocation.
Remove obsolete generated-file notices and Fogg CI dependencies once ownership transfers.

Preserve the locking mechanism during adoption. Afterwards recommend S3 lockfiles where the
selected Terraform/OpenTofu version supports them. For Terraform, `use_lockfile` requires 1.10+;
a staged migration can retain DynamoDB locking alongside S3 locking until all writers use S3.
Include `.tflock` permissions and keep the backend location unchanged. Treat upgrades and lock
modernization as separate changes from the Fogg cutover.

For detailed recipes, load the relevant sibling skill only when needed: `atmos-custom-commands`,
`atmos-hooks`, `atmos-toolchain`, or `atmos-aws-eks`.

## Ordering and Operational Differences

`dependencies.components` declares execution order; it does not retrieve values.
`!terraform.output` retrieves values; it does not deploy producers. Use explicit relationships
and `atmos terraform plan --components webserver -s development --include-dependencies`.
A new producer must be applied before its outputs exist; planning alone does not create them.
Do not create edges for every unused remote-state source that Fogg generated.

Fogg's `check-plan` accepts Terraform exit codes 0 and 2. Preserve that in the custom command,
while propagating failures. Preserve required checks and approvals when replacing CI entrypoints.
Do not copy `-lock=false` merely to match Make flags.

Fogg's ten-minute cached refresh policy is a behavioral gap in ready-made configuration.
Hooks provide the extension point (`before.terraform.plan`, `on_failure: fail`), but a complete
cache policy still needs successful-refresh timestamps, backend/workspace isolation, expiry,
concurrency coordination, and remote-execution handling. Only suppress the outer plan's refresh
after that policy succeeds. `when: local` does not identify the Terraform backend's execution
mode. Prefer normal plan refresh unless the user needs equivalent caching. Label custom cache
implementation work explicitly; do not call hook wiring alone a cache implementation.

## Completion Evidence

Report which roots were adopted and which remain, resolved directories/workspaces and tool
versions, provider aliases/module addresses/output sensitivity, dependency order, and baseline
versus Atmos plan results. Exercise both successful checks and a failed check, including plan
exit code 2. Label cloud-dependent authentication, EKS, remote-state, or TFE checks that were
not run. Require equivalent plans before operational cutover and maintain a single apply writer
per state. Rollback restores the prior configuration/pipeline against current state; it does
not restore an old state snapshot over newer applies.
