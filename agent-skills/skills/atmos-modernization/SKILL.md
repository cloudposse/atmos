---
name: atmos-modernization
description: "Atmos Modernization: migrate deprecated or legacy Atmos patterns to current names, Native CI, Atmos Pro drift detection, dependencies.components, name_template, and declared secrets"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: state-versioning
---

# Atmos Modernization

Use this skill when updating older Atmos projects to current conventions. "Atmos Modernization" is
the umbrella term for replacing legacy patterns with supported, current patterns.

## Modernization Checklist

| Legacy pattern | Replace with |
|---|---|
| `name_pattern` | `name_template` or explicit stack `name` |
| `settings.depends_on` | `dependencies.components` |
| Custom command `type: component`/`type: stack` on arguments, or `semantic_type: component`/`semantic_type: stack` on flags | `provides: component`/`provides: stack` on both arguments and flags |
| `cloudposse/github-action-atmos*` wrapper actions | Native CI with direct `atmos` commands |
| `cloudposse/github-action-atmos-component-updater` | `atmos vendor update --pull-request` with native GitHub PR publishing |
| `cloudposse/github-action-setup-atmos` as default | GitHub Actions container `ghcr.io/cloudposse/atmos:<version>` |
| `apt-get install docker.io` in an Atmos container job | Remove it: the official Atmos image includes the Docker CLI (`docker-cli`) and daemon package (`docker.io`) |
| GitHub Actions `concurrency` around jobs or workflows that invoke `atmos` | An explicit promotion workflow or deployment controller — environments and merge queues are approval/merge-order controls, not deployment-order guarantees; a concurrency group evicts its pending run regardless of `cancel-in-progress` |
| `hashicorp/setup-terraform` / `opentofu/setup-opentofu` in Atmos jobs | Atmos `dependencies.tools` and toolchain |
| Manual `atmos toolchain install <tool>` preinstall steps for Atmos-owned tools | Declarative `dependencies.tools` at the owning component, workflow, hook, or custom command |
| Large inline workflow/custom-command shell scripts, repeated `echo`, shell loops, ad hoc sleeps | Native step types such as `atmos`, `toast`, `table`, `parallel`, `matrix`, `wait`, `container`, `emulator`, and `http` |
| Hand-rolled scheduled drift GitHub Actions | Atmos Pro drift detection |
| `cloudposse/github-action-atmos-terraform-drift-*` | `settings.pro.drift_detection` plus `atmos terraform plan --upload-status` |
| Secret values through raw store calls | Declared `secrets.vars` plus `!secret` |
| Legacy hook event spelling | Modern dotted lifecycle events such as `after.terraform.plan` |
| Static GitHub tokens in URLs | Atmos Auth `github/sts` through Atmos Pro |
| `cloudposse/terraform-aws-components` sources | Repos in the `cloudposse-terraform-components` organization |
| Parser-specific doubled double quotes (`""`) in `!terraform.state` or `!terraform.output` expressions | Direct YQ expressions; quote only as required by YAML |

## Process

1. Start with `atmos doctor` (or `--offline` to avoid network metadata checks).
  Use `atmos doctor --fix --dry-run` to inspect eligibility, then `--fix` for verified repairs.
  These commands do not evaluate executable YAML functions or resolve remote state/secrets.
2. Inspect current project behavior with `atmos describe stacks`, `atmos list components`, and
  `atmos validate stacks`.
3. Replace one class of legacy pattern at a time.
4. Preserve resolved stack output unless the modernization intentionally changes behavior.
5. Validate with `atmos describe component <component> -s <stack>` before changing CI.
6. Update CI last, after stack config and auth patterns are current.

Doctor's schema replacement descriptions are guidance, not executable transforms.
Automatic repairs are limited to recognized custom-command semantic fields, hook
event spellings, explicit Terraform reconfigure settings in verifiable inputs, and moving
archived `cloudposse/terraform-aws-components` sources to their successor repositories
at the same version once the files are verified identical.
`init_run_reconfigure: false` maps to `init.reconfigure: never`; `true` maps to `auto`.
Conflicts remain manual. Other configuration layers (imports, `atmos.d`, active profiles,
home or system configuration) and dynamic values at or under the edited key leave a config
repair verification-incomplete; the finding names the blocking file and line. Templates elsewhere
in the file do not block a repair. A repair can reformat blank lines and folded scalars, so review
the diff. `--offline` notices never count toward `--fail-on=warning`.
Doctor preserves exact originals in its displayed XDG backup directory even in Git;
Git status is a recovery warning, not evidence of semantic equivalence.

Do not assume `settings.depends_on` is equivalent to `dependencies.components`:
legacy missing targets warn and succeed, while modern required targets fail.
Adding `required: false` also changes edge metadata. `atmos doctor --fix` migrates
simple lists (entries with a literal `component` and optional `stack`) automatically
and intentionally adopts required semantics, reporting any resulting graph errors.
Context selectors, file/folder entries, and naming repairs remain manual.

Use `atmos doctor --only=sources` to check vendor and source-provisioned upstream
repositories. Archived sources need manual review. A GitHub 404 can mean a private
repository or insufficient access; it does not establish that the repository was
deleted. Doctor never guesses replacement URLs.

## YAML Function Quoting

Older stack manifests may wrap an entire YQ expression in double quotes and double its inner
double quotes. That parser-specific escaping is obsolete: `!terraform.state` and
`!terraform.output` parse the component, optional stack, and remaining expression directly.

Replace legacy string-default quoting:

```yaml
# Before
username: !terraform.output config ".username // ""default-user"""

# After
username: !terraform.output config .username // "default-user"
```

Replace legacy string-concatenation quoting:

```yaml
# Before
postgres_url: !terraform.state aurora-postgres ".master_hostname | ""jdbc:postgresql://"" + . + "":5432/events"""

# After
postgres_url: !terraform.state 'aurora-postgres .master_hostname | "jdbc:postgresql://" + . + ":5432/events"'
```

The outer single quotes in the concatenation example are YAML quoting, not function-parser
escaping. Use YAML quoting only when the scalar requires it; for complete YAML and YQ quoting
guidance, see the [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) skill.

## Native CI Direction

New CI should run Atmos directly, preferably in the Atmos container image:

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@v6
      - run: atmos terraform plan vpc -s prod
```

The official Atmos image includes `docker.io`, so do not add an `apt-get install docker.io` step
to containerized Atmos jobs. Docker-backed commands use the runner-provided Docker daemon/socket.

Use `atmos describe affected --format=matrix` for PR matrices and `atmos list instances
--format=matrix` for full estate operations.

## Component Updater Migration

Replace the legacy Component Updater action with checkout plus `atmos vendor update --pull-request`.
Keep update selection under `vendor.update` (including named groups) and PR/summary policy under
`vendor.ci`. Grant only `contents: write`, `pull-requests: write`, and `issues: write` where
needed. Atmos writes a native GitHub step summary for every vendor update; it links to any created
or reused PR. Do not use third-party actions for update, commit, push, or PR creation. For exact
configuration and staged migration, use the vendoring
[component-updater reference](../atmos-vendoring/references/component-updater.md) and migration
[from-component-updater reference](../atmos-migration/references/from-component-updater.md).

## Drift Direction

Atmos Pro is the product path for drift detection. Enable drift per stack/component:

```yaml
settings:
  pro:
    enabled: true
    drift_detection:
      enabled: true
```

Then upload plan status:

```bash
atmos terraform plan vpc -s prod --upload-status
```

## Dependencies Direction

Use `dependencies.components` for component, file, and folder dependencies:

```yaml
dependencies:
  components:
    - component: vpc
    - component: dns-zone
      stack: plat-ue2-prod
    - kind: file
      path: configs/service.yaml
    - kind: folder
      path: src/lambda
```

Treat `settings.depends_on` as migration-only syntax.

## Component Source Repository Direction

Check `source`, `vendor.yaml`, every vendor manifest that `vendor.yaml` imports, `component.yaml`,
Terraform module sources, and documentation examples for `cloudposse/terraform-aws-components`.
That monorepo is archived; components moved to individual repositories in the
`cloudposse-terraform-components` organization.

The move keeps the version. The new repositories carry the monorepo's historical tags unchanged,
and the component code lives in their `src` directory, so only the repository and the path change.
For example, migrate VPC references from the old monorepo form:

```yaml
source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.450.0"
```

to the component repository form, with the same `ref` and a `//src` subdirectory:

```yaml
source: "github.com/cloudposse-terraform-components/aws-vpc.git//src?ref=1.450.0"
```

A `ref={{ .Version }}` placeholder stays as it is and keeps using the `version` field. Do not bump
the version while changing the repository.

Look every component up in the map Cloud Posse published instead of deriving the repository name,
because several names differ: `aws-sso` is `aws-identity-center`, `argocd-repo` is
`argocd-github-repo`, `datadog-configuration` is `datadog-credentials`, and
`tgw/cross-region-hub-connector` is `tgw-hub-connector`. Components that were split, such as `tgw`
and `spacelift`, follow a special procedure, and components removed before the migration, such as
`eks/efs`, have no successor.

Run `atmos doctor --fix --only=component-monorepo` to make this change. Doctor compares the Git tree
of `modules/<component>` with `src` in the new repository at the pinned tag and rewrites a source
only when the files are identical. Preview it first with `atmos doctor --fix --dry-run
--only=component-monorepo`. Doctor reports sources it cannot verify, with the reason, and leaves
them unchanged.
