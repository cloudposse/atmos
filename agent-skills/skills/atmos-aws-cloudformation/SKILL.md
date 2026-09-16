---
name: atmos-aws-cloudformation
description: "Native AWS CloudFormation components (experimental): render/plan/diff/apply/deploy/delete/validate/output/fmt/list via the AWS SDK for Go v2, changesets, drift detection, S3 backend, source provisioning, StackSets, tree/logs/watch"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: orchestrators
---

# Atmos Native AWS CloudFormation Components

Use this skill for the **native `aws/cloudformation`** component type
(`components."aws/cloudformation"`). It deploys CloudFormation stacks directly through the
**AWS SDK for Go v2** — no `aws` CLI, and no `cfn`/`sam`/`Rain` binary. `aws/cloudformation` is the
first member of an `aws/*` namespace for AWS-native primitives that bypass Terraform.

This feature is **experimental** — the nested command group carries the same experimental
annotation pattern `atmos terraform backend` uses; the top-level `aws` command group itself stays
stable.

## Related Skills

| Need | Load |
|---|---|
| Terraform/OpenTofu orchestration (contrast: HCL + state file vs. CFN's own stack/changeset state) | [atmos-terraform](../atmos-terraform/SKILL.md) |
| Component architecture, inheritance, catalogs, `dependencies.components` DAG ordering | [atmos-components](../atmos-components/SKILL.md) |
| AWS credentials / identities for the SDK client and per-target auth overrides | [atmos-auth](../atmos-auth/SKILL.md) |
| Lifecycle hooks around `diff`/`apply`/`delete` | [atmos-hooks](../atmos-hooks/SKILL.md) |
| Native CI job summaries | [atmos-ci](../atmos-ci/SKILL.md) |
| `!secret` values flowing into `parameters:` | [atmos-secrets](../atmos-secrets/SKILL.md) |
| `kind: git` GitOps delivery target mechanics | [atmos-git](../atmos-git/SKILL.md) |
| Local AWS sandbox (Floci) for testing without real credentials | [atmos-emulator](../atmos-emulator/SKILL.md) |
| Migrating off Rain / raw CloudFormation | [atmos-migration](../atmos-migration/SKILL.md) → `references/from-rain.md` |

## Component Shape

Define CloudFormation stacks under `components."aws/cloudformation"` in stack manifests. Quoting the
key is optional in YAML (a `/` doesn't require it) but is the convention used throughout Atmos docs
and examples:

```yaml
components:
  "aws/cloudformation":
    vpc:
      path: template.yaml                    # file reference, relative to the component's base path
      stack_name: "{{ .vars.stage }}-vpc"     # explicit; no legacy name-pattern interpolation
      parameters:
        CidrBlock: "10.0.0.0/16"
        AvailabilityZones:
          - us-east-1a
          - us-east-1b
        DbPassword: !secret DB_PASSWORD       # secrets flow into parameters, not env
      capabilities:
        - CAPABILITY_IAM
      tags:
        team: platform
      stack_policy:
        file: stack-policy.json
      role_arn: "arn:aws:iam::123456789012:role/cfn-deploy"
      termination_protection: true
      timeout_in_minutes: 30
      settings:
        aws_cloudformation:
          region: us-east-2
      provision:
        backend:
          enabled: true
        targets:
          artifacts:
            kind: aws/s3
            bucket: acme-plat-cfn-artifacts
            region: us-east-2
          gitops:
            kind: git
            repository: acme/infra-gitops
            path: stacks/dev/vpc
```

CloudFormation components use the same stack sections as other component types — `vars`, `env`,
`auth`, `metadata`, `settings`, `dependencies`, `hooks`, `source`/`provision`, inheritance, and
overrides — plus CloudFormation-specific fields. They do **not** support `generate:` (no
codegen-artifact output, unlike Terraform's backend/provider generation) or `plugins:` (no
chart-style plugin system, unlike native Helm).

| Field | Purpose |
|---|---|
| `template` / `path` *(exactly one required)* | `template` is an **inline** body (string or YAML map) that flows through Atmos's `{{ }}` templating before reaching CloudFormation. `path` is a **file reference**, read as raw bytes, no templating. Setting both is an error. |
| `stack_name` | Explicit stack name. Supports Go templates; no legacy name-pattern interpolation. |
| `parameters` | `map[string]any`, normalized at the API boundary: scalars stringified, lists comma-joined for `List<Type>`. `UsePreviousValue` isn't expressible — Atmos config is always the source of truth. |
| `capabilities` | Acknowledged IAM capabilities, e.g. `CAPABILITY_IAM`, `CAPABILITY_AUTO_EXPAND` (macros/SAM). |
| `tags` | `map[string]string` tags on the stack — distinct from Atmos's own component `tags`/`--tags`. |
| `stack_policy.file` | Stack policy JSON path. Applied via `SetStackPolicy` *after* apply — protects the *next* update. |
| `role_arn` | The CloudFormation **service role**, not caller credentials — `CreateChangeSet`'s `RoleARN`. |
| `notification_arns` | SNS topic ARNs CloudFormation publishes stack events to. |
| `disable_rollback` | Prevents automatic rollback on stack creation/update failure. |
| `termination_protection` | See [Delete Safety](#delete-safety--termination-protection) below. |
| `timeout_in_minutes` | Bounds how long stack creation may run before CloudFormation rolls back. |
| `source` | JIT template provisioning — see [Source & Template Management](#source--template-management). |
| `provision` | Delivery targets — see [Delivery Targets](#delivery-targets-backend-management). |

## Commands

| Command | Purpose |
|---|---|
| `atmos aws cloudformation render <component> -s <stack>` | Render the local template client-side. No API calls. |
| `atmos aws cloudformation validate <component> -s <stack>` | Server-side `ValidateTemplate` — syntax and capability discovery, not a local linter. |
| `atmos aws cloudformation diff <component> -s <stack>` | Creates (or reuses) a changeset and previews the changes an apply would make, then deletes the preview changeset (best-effort cleanup) so it never leaks against the account's changeset quota. `plan` is an alias. |
| `atmos aws cloudformation apply <component> -s <stack>` | Executes the changeset (`ExecuteChangeSet`), creating or updating the stack — never a direct `CreateStack`/`UpdateStack` call. Streams per-resource stack events live and ends with a rendered Outputs summary. |
| `atmos aws cloudformation deploy <component> -s <stack>` | Alias for `apply` with `--auto-approve` defaulted to `true`. |
| `atmos aws cloudformation delete <component> -s <stack>` | `DeleteStack`, respecting termination protection — see [Delete Safety](#delete-safety--termination-protection). |
| `atmos aws cloudformation output <component> -s <stack> [key]` | Renders the deployed stack's Outputs (all, or one by key) via `DescribeStacks` — the same view `apply`/`deploy` render at completion. Alias: `outputs`. |
| `atmos aws cloudformation fmt <component> -s <stack> [--check]` | Canonically formats the local template in place (comment-preserving YAML round-trip, no shell-out). `--check` reports without writing, for CI. |
| `atmos aws cloudformation list` | Lists CloudFormation stacks in the active identity's account/region via `ListStacks`, including stacks Atmos doesn't manage. |

All operation commands accept `--all`, `--affected` (with `--base`/`--ref`/`--sha`/`--repo-path`/
`--clone-target-ref`/`--ssh-key`/`--ssh-key-password`), `--include-dependents` (requires
`--affected`), and `--tags`/`--labels` bulk selection, matching `atmos describe affected` semantics.
`--all`/`--affected` are mutually exclusive with a positional component argument, and so is
`--tags`/`--labels`-based selection. `atmos aws cfn` is a Cobra alias for `atmos aws cloudformation`
that works with every verb.

**Confirmation**: `apply` and `delete` prompt for interactive confirmation on a TTY; pass
`--auto-approve` to skip it. `deploy` defaults `--auto-approve` to `true`.

### Output formats

`output` supports the full standard format set shared with `atmos terraform output`: `json`, `yaml`,
`hcl`, `env`, `dotenv`, `bash`, `csv`, `tsv`, `table` (default on a TTY), and `github` (GitHub
Actions `$GITHUB_OUTPUT` syntax via `atmos aws cloudformation output vpc -s dev --format=github`),
plus `--flatten` and `--uppercase` key options. Outputs sourced from `NoEcho`-declared template
parameters are masked by value wherever they reappear (changeset rendering, `describe` output,
`output` results, logs) — not just at the original parameter field.

## Changesets

`diff`/`plan` and `apply`/`deploy` manage changesets implicitly. For manual, two-phase control (e.g.
create + review in one CI job, execute in another), use the explicit verb group:

```shell
atmos aws cloudformation changeset create vpc -s dev
atmos aws cloudformation changeset list vpc -s dev
atmos aws cloudformation changeset execute vpc -s dev --changeset-name=<name>
atmos aws cloudformation changeset delete vpc -s dev --changeset-name=<name>
```

`changeset execute` and `changeset delete` prompt for confirmation (skip with `--auto-approve`), same
as top-level `apply`/`delete`. Templates using macros/transforms (`Fn::Transform`, SAM) work through
this same flow — declare the required capability (typically `CAPABILITY_AUTO_EXPAND`) in
`capabilities:`.

## Drift Detection

```shell
atmos aws cloudformation drift detect vpc -s dev [--fail-on-drift]
atmos aws cloudformation drift describe vpc -s dev
```

`drift detect` runs `DetectStackDrift`/polls `DescribeStackDriftDetectionStatus`; `drift describe`
renders the results of the most recent detection (`DescribeStackResourceDrifts`). `--fail-on-drift`
exits non-zero when drift is found, for CI gating — drift is not a hard failure by default. Drift
results are local/CLI-only in the current implementation; routing them into the Atmos Pro dashboard
(the same `UploadInstanceStatus` pipeline Terraform's drift detection uses) is recorded as future
work, not yet implemented.

## Delivery Targets (Backend Management)

By default `apply`/`deploy` deploy directly to the account/region resolved for the component (see
[Region Resolution](#region-resolution)). A component can declare additional named
`provision.targets`, selected with `--target`: `aws/s3` (publish-only upload, also used
**automatically** to package any template over CloudFormation's 51,200-byte inline limit),
`git` (GitOps commit instead of deploying), and `aws/stackset` (multi-account/region, see
[Stack Sets](#stack-sets)). "Backend" means the S3 artifact bucket a `kind: aws/s3` target
declares — CloudFormation's own stack state is service-managed. Manage it like
`atmos terraform backend`:

```shell
atmos aws cloudformation backend create vpc -s dev
```

Set `provision.backend.enabled: true` to auto-provision the bucket on first use instead of running
`backend create` explicitly. See
[references/delivery-targets.md](references/delivery-targets.md) for the full target-kind table,
the `backend` verb group, auto-provisioning semantics, and today's packaging-scope limitation
(local assets like Lambda zips aren't rewritten, only the template body is uploaded).

## Source & Template Management

Point a component at a remote template through the top-level `source:` section instead of committing
it to your infrastructure repository — the same JIT-vendoring mechanism other component types use:

```yaml
components:
  "aws/cloudformation":
    vpc:
      source:
        uri: github.com/acme/cfn-templates.git//vpc?ref={{ .Version }}
        version: 1.2.0
      path: template.yaml           # relative to the vendored directory
```

A single-file source URI (e.g. a raw `https://.../dns.yaml` link) is fetched directly and used
as-is — `path:` isn't needed in that case.

Inspect and manage vendored sources with the `source` verb group:

```shell
atmos aws cloudformation source pull vpc -s dev
atmos aws cloudformation source list [vpc] [-s dev]
atmos aws cloudformation source describe vpc -s dev
atmos aws cloudformation source delete vpc -s dev
```

`aws/cloudformation` components **cannot** be vendored through a `vendor.yaml` manifest
(`atmos vendor pull`) — only `source:`-based JIT provisioning, the same restriction native Helm and
Kubernetes components have.

## Stack Sets

Multi-account/multi-region orchestration, resolved from a `kind: aws/stackset` provision target
(`accounts`, `regions`, `permission_model`, `administration_role_arn`, `execution_role_name`):

```shell
atmos aws cloudformation stackset create vpc -s dev --target=multi-account
atmos aws cloudformation stackset update vpc -s dev --target=multi-account
atmos aws cloudformation stackset delete vpc -s dev
atmos aws cloudformation stackset instances vpc -s dev
```

`create`/`update`/`delete` prompt for confirmation (skip with `--auto-approve`). `delete` and
`instances` act directly on `stack_name`, not the target. See
[references/delivery-targets.md](references/delivery-targets.md) for the full target YAML shape
and `create`-vs-`update` semantics.

## Observability: tree, logs, watch

```shell
atmos aws cloudformation tree vpc -s dev              # nested-stack/resource dependency graph
atmos aws cloudformation logs vpc -s dev [--chart] [--follow]   # combined event log across nested stacks
atmos aws cloudformation watch vpc -s dev             # attach to an in-progress (or terminal) operation
```

`logs --chart` renders a per-resource timeline instead of a flat chronological list; `--follow`
tails new events continuously and is mutually exclusive with `--chart`. `watch` is for *attaching* to
an operation already in progress — including one started outside Atmos — distinct from the live
streaming `apply`/`deploy`/`delete` already show while they run.

## Deployed-Stack Retrieval

```shell
atmos aws cloudformation get template vpc -s dev [--original]
atmos aws cloudformation get policy vpc -s dev
```

`get template` answers "what's actually deployed right now" via `GetTemplate` (`--original` fetches
the user-submitted body instead of the post-transform one); `get policy` fetches the live stack
policy via `GetStackPolicy`. This is the inverse of `render` (local-only) — useful for drift
investigation and for inspecting a stack before adopting it into Atmos management (stack
import/adoption itself is not supported).

## Delete Safety & Termination Protection

`delete` maps to `DeleteStack`, with two safety behaviors:

- **`--retain-resources=<logical-id1>,<logical-id2>`** passes retained logical IDs through to the
  API — only valid for a `DELETE_FAILED` stack.
- **Termination protection is checked against the stack's *live* AWS state, not just local config**,
  and is never silently disabled. If live protection is enabled, `delete` fails with a hint pointing
  at `--disable-termination-protection` (calls `UpdateTerminationProtection` before deleting).
  Editing `termination_protection: false` and re-applying does **not** unprotect an already-protected
  stack — `apply` only ever turns protection on. Only the explicit delete flag turns it off.

## Region Resolution

CloudFormation API calls need an AWS region, resolved most-specific-wins:

1. `settings.aws_cloudformation.region` on the component
2. The active identity's region
3. The AWS SDK's default credential/region chain (`AWS_REGION`, shared config)

There is no per-component account override outside StackSets — the account is always the active
identity's account.

## Auth

Component-level `auth:` selects an identity exactly like a Terraform component does — see
[atmos-auth](../atmos-auth/SKILL.md) for identity chaining, FIPS endpoints, and `aws/emulator`
(Floci) local/no-credentials testing. Per-target `provision.targets.<name>.auth` overrides let a
deploy target assume a workload account's identity while an artifact-bucket target uses a
shared-services account's identity. `role_arn` on the component is a **separate concept** — it's
the CloudFormation service role, not caller credentials; see the [Component Shape](#component-shape)
table.

## atmos.yaml Configuration

```yaml
components:
  "aws/cloudformation":
    base_path: components/cloudformation   # default
```

`base_path` is the only project-wide setting. Every other CloudFormation field (`template`/`path`,
`stack_name`, `parameters`, `capabilities`, `tags`, `stack_policy`, `role_arn`, `notification_arns`,
`disable_rollback`, `termination_protection`, `timeout_in_minutes`, `source`, `provision`, `auth`,
`dependencies`) is configured per stack, not in `atmos.yaml`.

## Native CI Summaries

When `ci.enabled: true` and Atmos runs in a supported CI provider (e.g. GitHub Actions), a native CI
plugin (`pkg/ci/plugins/cloudformation`) writes a compact Markdown job summary for `diff`, `apply`,
`delete`, `drift detect`, and `drift describe` — the same summaries-only tier Kubernetes and Helmfile
occupy (no `$GITHUB_OUTPUT`, commit statuses, PR comments, or artifacts; that richer tier remains
Terraform-only). See [atmos-ci](../atmos-ci/SKILL.md) for the native-CI plumbing this rides on.

## Hooks

Five lifecycle pairs fire hook events: `before`/`after` × `diff` (`plan` normalizes to `diff`),
`apply` (`deploy` normalizes to `apply`), `delete`, `drift detect`, and `drift describe` — e.g.
`after.aws/cloudformation.apply`. Every other verb — `render`, `validate`, `output`, `fmt`, `tree`,
`logs`, `watch`, `changeset *`, `get *`, `stackset *`, `list`, `backend *`, and `source *` — does
not fire hook events. See [atmos-hooks](../atmos-hooks/SKILL.md) for the `hooks:` block shape.

## Secrets

Secrets flow into **`parameters:` values** via `!secret`, resolved at stack-processing time and
passed directly in the `CreateChangeSet` parameter list — there is no subprocess to receive `env:`,
so the shell-out pattern of exporting secrets as environment variables doesn't apply to the
CloudFormation API call itself (`env:` still works normally for hooks/`!exec`/template functions).
CloudFormation's own `NoEcho` only hides a parameter from the AWS Console — it does not redact the
value from API responses — so Atmos additionally registers `NoEcho`-declared parameter values with
its own masker by value, covering changeset rendering, `describe` output, `output` results, and logs.
See [atmos-secrets](../atmos-secrets/SKILL.md) for `!secret` mechanics.

## Migrating from Rain or Raw CloudFormation

There is no Rain CLI or config-file compatibility layer, and no `!Rain::` directive preprocessing —
`aws/cloudformation` reads a component's `path:` template as raw bytes and submits it unmodified.
Existing templates are pointed at (or `!include`d for parameter files), not rewritten. See
[atmos-migration](../atmos-migration/SKILL.md)'s `references/from-rain.md` for the full `!Rain::`
directive mapping table (`Constant`, `Env`, `Include`, `S3`, `Embed`, `Module`) and the
Rain-verb-to-`atmos aws cloudformation`-verb cross-reference (`fmt`→`fmt`, `cat`→`get template`,
`ls`→`list`, `bootstrap`→`backend create`, `log`→`logs`, `rm`→`delete`).

## Guidance

- Prefer `path:` for templates that live in the component directory; use inline `template:` only
  when the body needs Atmos's own `{{ }}` templating before reaching CloudFormation.
- Use `dependencies.components` so `--all`/`--affected` deploys stacks in the right order — a
  Terraform component can `depends_on` a CFN stack and vice versa.
- Use `provision.backend.enabled: true` for a zero-friction dev sandbox; use explicit
  `backend create`/`update` in shared/production environments to re-apply secure defaults
  deliberately rather than only on first use.
- Referenced local assets (Lambda zips, nested-stack templates) need manual out-of-band S3
  upload today — automatic packaging only covers the template body itself.
- Use the Floci `aws/emulator` identity (see `examples/cloudformation/`) to develop and test with
  zero AWS credentials before pointing at a real account.
