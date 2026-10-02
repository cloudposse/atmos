# Migrating to Native CI

This reference is the agent's decision guide for converting a hand-rolled or third-party-action-based
GitHub Actions Terraform pipeline into Atmos **Native CI**. It is named `to-native-ci.md`, not
`from-github-actions.md`, on purpose: the workflow keeps running in GitHub Actions in every case --
only *which actions run inside it* changes (third-party wrapper actions replaced by native `atmos`
commands). Unlike other migration references with one source and the constant destination "Atmos,"
this reference has many heterogeneous sources (tool-setup actions, cloud-auth actions,
plan/apply/comment action suites) converging on one destination: Native CI.

This file does **not** re-explain the Atmos-side `ci:` config -- see
[atmos-ci/references/native-ci.md](../../atmos-ci/references/native-ci.md) for that. Its job is the
mapping: "you have this third-party action doing X, here's the Atmos-native equivalent."

Two working example repos show the destination end-to-end and confirm the `github` profile-naming
convention used throughout this reference:

- [cloudposse-examples/atmos-native-ci](https://github.com/cloudposse-examples/atmos-native-ci) --
  basic plan-on-PR / deploy-on-merge pipeline
- [cloudposse-examples/atmos-native-ci-advanced](https://github.com/cloudposse-examples/atmos-native-ci-advanced) --
  affected/matrix pipeline

## Foundational: Run Atmos From the Container, Not an Install Step

Before mapping anything else, replace any step that installs Atmos -- Cloud Posse's deprecated
`cloudposse/github-action-setup-atmos`, a version-manager action such as `jdx/mise-action` or
`aquaproj/aqua-installer`, Homebrew, or a `curl` download of the release binary -- with the
job-level container:

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
```

Pin `ATMOS_VERSION` with a repository or organization variable; avoid `latest` in CI. None of the
Before/After examples below need a separate "install Atmos" step -- the container is the base every
other section in this file builds on.

## Identify What's in the Existing Workflow

| Existing step/action does...                                                    | Replace with...                                  |
|-----------------------------------------------------------------------------------|---------------------------------------------------|
| Installs the Atmos CLI itself (`cloudposse/github-action-setup-atmos`, mise/aqua actions, Homebrew, `curl`) | [Foundational: the container image](#foundational-run-atmos-from-the-container-not-an-install-step) |
| Installs Terraform/OpenTofu (`hashicorp/setup-terraform`, `opentofu/setup-opentofu`) | [Toolchain](#replacing-terraformopentofu-setup-actions) |
| Assumes a cloud role via OIDC (`aws-actions/configure-aws-credentials`, `azure/login`, `google-github-actions/auth`) | [Auth and profiles](#replacing-cloud-oidc-role-assumption-actions) |
| Runs plan/apply and posts PR comments (`dflook/terraform-plan`/`terraform-apply`) | [Replacing dflook](#replacing-dflookterraform-github-actions) |
| Wraps a raw `terraform plan`/`apply` to post PR comments/labels (`tfcmt`)         | [Replacing tfcmt](#replacing-tfcmt-suzuki-shunsuke) |
| Stores/retrieves a plan file between PR and merge jobs                            | [Planfile storage](#planfile-storage----plan-once-apply-the-reviewed-plan) |
| Runs `tfsec`/`checkov`/`kics`/`infracost`/`tflint`                                | [Linting and static analysis](#linting-and-static-analysis) |
| Sends a Slack/Teams/custom notification, or runs an arbitrary script step         | [Custom bolted-on steps](#custom-bolted-on-steps-notifications-scripts) |
| Posts commit statuses/checks or a custom-formatted PR comment                     | [Status checks and comments](#status-checks-and-comments----beyond-the-basics) |
| Serializes Terraform runs with a `concurrency:` group                             | [Concurrency groups and state locks](#concurrency-groups-and-state-locks) |
| Updates vendored components and opens PRs (`cloudposse/github-action-atmos-component-updater`) | [Component updater migration](from-component-updater.md) |

For every replacement, review the job's token wiring and permissions against
[Native CI permissions](../../atmos-ci/references/native-ci.md#minimal-permissions). Distinguish
commit statuses (`statuses: write`) from Check Runs (`checks: write`), and preserve PR comments,
scanner uploads, artifact access, and required-check behavior only with the permissions they need.

## Replacing Terraform/OpenTofu Setup Actions

`hashicorp/setup-terraform` and `opentofu/setup-opentofu` install a pinned binary onto the runner.
Atmos replaces this with the toolchain, which installs and injects the pinned version into the
command environment automatically -- no separate install step, and the version travels with the
component/stack config instead of living only in workflow YAML.

**Before:**
```yaml
- uses: hashicorp/setup-terraform@v3
  with:
    terraform_version: "1.10.3"
- run: terraform plan
```

**After:**
```yaml
# atmos.yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    opentofu: opentofu/opentofu
    tofu: opentofu/opentofu

terraform:
  dependencies:
    tools:
      terraform: "1.10.3"
      # For OpenTofu projects:
      # opentofu: "1.10.3"
```
```yaml
# workflow -- no setup step needed, runs inside the container from the Foundational section
- run: atmos terraform plan vpc -s prod
```

This runs *inside* the container job from the Foundational section above -- the container provides
Atmos itself, `dependencies.tools` provides Terraform/OpenTofu at the pinned version. See
[atmos-toolchain](../../atmos-toolchain/SKILL.md) for aliasing, per-component pins, and
`atmos toolchain env --format=github` for exposing the resolved path to non-Atmos shell steps.

## Replacing Cloud OIDC Role-Assumption Actions

Lead with `aws-actions/configure-aws-credentials`, the most common case:

**Before:**
```yaml
permissions:
  id-token: write
steps:
  - uses: aws-actions/configure-aws-credentials@v6
    with:
      role-to-assume: arn:aws:iam::123456789012:role/atmos-ci
      aws-region: us-east-2
  - run: terraform plan
```

**After:**
```yaml
# atmos.yaml (or profile-scoped config, see Profiles below)
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-2
      spec:
        audience: sts.amazonaws.com
  identities:
    plat-dev/terraform:
      kind: aws/assume-role
      via:
        provider: github-oidc
      principal:
        assume_role: arn:aws:iam::123456789012:role/atmos-ci
```
```yaml
# workflow
permissions:
  id-token: write
  contents: read
env:
  ATMOS_PROFILE: github
steps:
  - uses: actions/checkout@v6
  - run: atmos terraform plan vpc -s prod
```

The IAM trust policy's `sub`-claim constraint carries over unchanged, but the exact subject depends
on job context and repository age. A job *without* a GitHub Environment (like the PR-plan example
below, triggered by `pull_request`) gets `repo:ORG/REPO:ref:refs/heads/main` or
`repo:ORG/REPO:pull_request`; a job that *references* an Environment gets
`repo:ORG/REPO:environment:prod` instead -- these are alternatives, not both-at-once. Repositories
created after July 15, 2026 default to an immutable subject format that embeds owner/repo IDs
(`repo:ORG@ORG-ID/REPO@REPO-ID:...`). Merge-queue runs (the `merge_group` trigger, also in the
PR-plan example) are ref-based on a temporary branch, so without an Environment they need a pattern
like `repo:ORG/REPO:ref:refs/heads/gh-readonly-queue/main/*` (`StringLike` in AWS). Match the trust
policy to the subject the job actually emits, or the OIDC exchange is denied.

`azure/login` and `google-github-actions/auth` follow the identical shape -- an OIDC action becomes
an `auth.providers`/`auth.identities` pair with the corresponding provider `kind`.
`gcp/workload-identity-federation` auto-detects `token_source` in GitHub Actions. Confirm the exact
provider `kind` for Azure OIDC in the [atmos-auth](../../atmos-auth/SKILL.md) skill before using it --
don't guess the identifier.

### Profiles, Explicitly

This is the piece easy to gloss over: the CI-specific `auth.providers`/`auth.identities` block is
usually not the same config a developer uses locally (SSO/SAML), so it lives in a named **profile**
and is activated with `ATMOS_PROFILE`:

```yaml
env:
  ATMOS_PROFILE: github   # activates the profile holding the github/oidc provider + identity
permissions:
  id-token: write
```

Do not add a routine `atmos auth login` step to non-interactive OIDC jobs -- Atmos exchanges the
OIDC token automatically when the command runs. For profile directory layout, activation precedence,
and merge behavior (as opposed to the auth block *inside* a profile), see
[atmos-profiles](../../atmos-profiles/SKILL.md); `atmos-auth` intentionally keeps its own profile
coverage scoped to just the auth sections.

## Replacing `dflook/terraform-github-actions`

The most popular non-HashiCorp Terraform action suite. It provides a plan-on-PR-comment /
apply-on-merge workflow across several individual actions:

| dflook action                         | Native replacement                                                                 |
|----------------------------------------|--------------------------------------------------------------------------------------|
| `terraform-plan`                       | `atmos terraform plan <component> -s <stack>` + `ci.comments`/`ci.checks` (native PR comment/check, no separate action) |
| `terraform-apply`                      | `atmos terraform deploy <component> -s <stack>`                                     |
| `terraform-fmt` / `terraform-fmt-check`| `atmos terraform fmt <component> -s <stack>` (passthrough to native `terraform fmt`) |
| `terraform-validate`                   | `atmos terraform validate <component> -s <stack>`                                   |
| `terraform-check` (drift)              | Atmos Pro drift detection (`settings.pro.drift_detection`) -- see [Drift](#drift-detection-without-atmos-pro) below for a lighter-weight alternative |
| `terraform-output`                     | `atmos terraform output <component> -s <stack>`, or `!terraform.state`/`!terraform.output` for cross-component reads |
| `terraform-new-workspace` / `workspace:` input | Not needed -- Atmos stacks replace workspace-per-environment. If the user is also migrating workspace state, route to [from-terraform-workspaces.md](from-terraform-workspaces.md) |
| Plan-comment-on-PR behavior            | `ci.comments.enabled: true` -- **remove the dflook comment step**, or PRs get duplicate plan comments |

**Before (raw workflow):**
```yaml
# .github/workflows/plan.yaml
name: plan
on: [pull_request]
permissions:
  contents: read
  id-token: write
  pull-requests: write
jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: "1.10.3"
      - uses: aws-actions/configure-aws-credentials@v6
        with:
          role-to-assume: arn:aws:iam::123456789012:role/atmos-ci
          aws-region: us-east-2
      - uses: dflook/terraform-plan@v2
        with:
          path: my-terraform-config

# .github/workflows/apply.yaml
name: apply
on:
  push:
    branches: [main]
permissions:
  contents: read
  id-token: write
jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: "1.10.3"
      - uses: aws-actions/configure-aws-credentials@v6
        with:
          role-to-assume: arn:aws:iam::123456789012:role/atmos-ci
          aws-region: us-east-2
      - uses: dflook/terraform-apply@v2
        with:
          path: my-terraform-config
```

**After (Native CI, affected matrix):**
```yaml
# .github/workflows/plan.yaml
name: plan
on:
  pull_request:
  merge_group:
jobs:
  affected:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    permissions:
      contents: read
      id-token: write
      statuses: write
      pull-requests: write
    env:
      ATMOS_PROFILE: github
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    outputs:
      matrix: ${{ steps.affected.outputs.matrix }}
    steps:
      - uses: actions/checkout@v6
      - id: affected
        run: atmos describe affected --format=matrix

  plan:
    needs: affected
    if: ${{ needs.affected.outputs.matrix != '' }}
    strategy:
      fail-fast: false
      matrix: ${{ fromJson(needs.affected.outputs.matrix) }}
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    permissions:
      contents: read
      id-token: write
      statuses: write
      pull-requests: write
    env:
      ATMOS_PROFILE: github
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    steps:
      - uses: actions/checkout@v6
      - run: atmos terraform plan "${{ matrix.component }}" -s "${{ matrix.stack }}"
```
```yaml
# .github/workflows/deploy.yaml
name: deploy
on:
  push:
    branches: [main]
jobs:
  affected:
    # same affected job as above
  deploy:
    needs: affected
    if: ${{ needs.affected.outputs.matrix != '' }}
    strategy:
      matrix: ${{ fromJson(needs.affected.outputs.matrix) }}
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    permissions:
      contents: read
      id-token: write
    env:
      ATMOS_PROFILE: github
    steps:
      - uses: actions/checkout@v6
      - run: atmos terraform deploy "${{ matrix.component }}" -s "${{ matrix.stack }}"
```

The `ci:` config that enables the comments/checks shown here (`ci.comments`, `ci.checks`) lives in
`atmos.yaml` -- see [native-ci.md](../../atmos-ci/references/native-ci.md) for the full block. For
a full deploy of every instance instead of just affected ones, use
`atmos list instances --format=matrix`.

## Replacing `tfcmt` (suzuki-shunsuke)

A distinct pattern from dflook: `tfcmt` doesn't run plan/apply itself -- it's a CLI wrapper around a
raw `terraform`/`tofu` invocation that posts the result as a PR comment and sets a result label
(`tfcmt plan -- terraform plan ...`, `tfcmt apply -- terraform apply ...`, with `-patch` to update
rather than duplicate comments). It's commonly paired with `hashicorp/setup-terraform` +
`aws-actions/configure-aws-credentials` + raw shell `terraform plan`/`apply` steps.

**Before:**
```yaml
- run: terraform plan -no-color -out=tfplan
- run: tfcmt plan -patch -- terraform show tfplan
```

**After -- no wrapper needed, plan/comment/status are native:**
```yaml
- run: atmos terraform plan vpc -s prod
```

Map `tfcmt`'s comment-patching to `ci.comments.behavior: upsert` (the default), and its
pass/fail/no-changes label to `ci.checks` -- commit statuses/checks convey the same signal natively,
with no repo label management required.

## Planfile Storage -- "Plan Once, Apply the Reviewed Plan"

`dflook/terraform-apply` applies exactly the binary plan that was posted on the PR by
`dflook/terraform-plan`, and fails if that plan is missing or stale. The native equivalent is
`components.terraform.planfiles`:

```yaml
components:
  terraform:
    planfiles:
      stores:
        github: {type: github/artifacts, options: {retention_days: 7}}
      priority: [github, s3, local]
      verify: fail   # fail (default under CI) | warn | off
```

Under CI mode (`--ci`, auto-detected from the `CI`/`ATMOS_CI` env vars that GitHub Actions already
sets, so it usually needs no explicit flag), `atmos terraform plan` uploads the plan to the
configured store, and `atmos terraform deploy` downloads it, runs a **fresh** plan, and structurally
diffs stored-vs-fresh before applying the fresh plan (override the diff check with
`--verify-plan`/`--verify-plan=false`).

**This is a real behavior difference, not a bug**: this is *diff-and-apply-fresh* by default, not
dflook's byte-identical replay. If a user needs byte-identical replay, point them at
`--planfile`/`--from-plan` as the opt-in for that exact semantic.

Do not confuse this with `--upload-status` -- that's a separate, Atmos-Pro-specific flag for
uploading plan status/drift results to the Pro API, unrelated to planfile storage.

The `github/artifacts` store needs `ACTIONS_RUNTIME_TOKEN`/`ACTIONS_RESULTS_URL`, which GitHub
injects into action steps but not `run:` shell steps. The bundled `github-runtime` action re-exposes
them; `mode: env` needs no further per-step wiring:

```yaml
- uses: cloudposse/atmos/actions/github-runtime@v1   # pin to a release or SHA
  with:
    mode: env
- run: atmos terraform plan vpc -s prod --ci      # uploads to github/artifacts
- run: atmos terraform deploy vpc -s prod --ci    # downloads & verifies the planfile
```

## Linting and Static Analysis

When replacing TFLint, Checkov, Trivy, KICS, Infracost, or tfsec actions, read
[to-native-ci-scanners.md](to-native-ci-scanners.md). Load
[atmos-toolchain](../../atmos-toolchain/SKILL.md) for version pins,
[atmos-lint](../../atmos-lint/SKILL.md) for TFLint, and
[atmos-hooks](../../atmos-hooks/SKILL.md) for native scanner hooks.

Replace scanner setup with `dependencies.tools` and supported scan operations with native hooks
or `atmos terraform lint`. Preserve targets, arguments, exclusions, and failure thresholds;
blocking checks need both a nonzero scanner exit on policy violations and `on_failure: fail`.
Keep unsupported scanning modes in command hooks or workflow steps.

## Custom Bolted-On Steps (Notifications, Scripts)

Generic third-party "run a script before/after plan/apply" steps map to native hooks two ways, and
notifications are the clearest example of the split:

**Arbitrary script/CLI notifiers** -- a custom Python/Slack-CLI script, or a third-party notify
action that isn't just a raw HTTP POST -- use `kind: command`:

```yaml
hooks:
  notify:
    events: [after.terraform.plan]
    kind: command
    command: python3
    args: ["scripts/notify.py"]
    format: markdown
    on_failure: warn
```

**Plain webhook notifiers** -- a Slack/Teams/Discord incoming webhook, or any other third-party
action that's really just an HTTP POST with a JSON/form body -- use `kind: step` with
`type: webhook` (an alias of the registered `http` step handler), which avoids writing and
maintaining a wrapper script entirely:

```yaml
hooks:
  notify-slack:
    events: [after.terraform.apply]
    kind: step
    type: webhook
    on_failure: warn
    with:
      url: "{{ .env.SLACK_WEBHOOK_URL }}"
      method: POST
      headers:
        Content-Type: application/json
      form:
        text: "Deployed {{ .env.ATMOS_COMPONENT }} in {{ .env.ATMOS_STACK }}"
```

`type: webhook` supports templated `url`, `method`, `headers`, `body`/`form`,
`expect.status`/`expect.response`, and built-in `retry` -- prefer it over `kind: command` plus a
hand-rolled `curl`/script whenever the notifier is just an HTTP call.

Use `when:` (CEL) to scope either hook form to specific stacks/CI contexts, e.g.
`when: stack == "prod" && ci`. See [atmos-hooks](../../atmos-hooks/SKILL.md) for the full set of
hook kinds and events.

## Status Checks and Comments -- Beyond the Basics

Beyond the `ci.checks`/`ci.comments` basics covered in
[native-ci.md](../../atmos-ci/references/native-ci.md), two things are worth knowing when migrating
off a third-party comment/status tool:

- **Per-status granularity**: `ci.checks.statuses.component/add/change/destroy` are each
  independently toggleable.
- **Comment behavior**: `ci.comments.behavior: create|update|upsert` (default `upsert`), comments
  keyed per component/stack.

`ci.templates` (custom Go-template overrides of job summaries/PR comments) exists as a last-resort
escape hatch -- it is **not** a default recommendation. Most users migrating from a
custom-formatted third-party action (`tfcmt`, dflook, a hand-rolled comment script) should adopt the
native default summary/comment format as-is rather than reimplementing their old format. Only reach
for `ci.templates` when there's a specific compliance or branding requirement the default can't
meet:

```yaml
ci:
  templates:
    base_path: ".atmos/ci/templates"
    terraform: {plan: "plan.md", apply: "apply.md"}
```

### Drift Detection Without Atmos Pro

Atmos Pro drift detection (`settings.pro.drift_detection`) is the recommended path for drift --
history, a remediation workflow, and a dashboard. For teams not ready to adopt Pro,
`atmos terraform plan --all` ("Plan all components in all stacks") on a scheduled workflow, combined
with `ci.output`'s `has_changes` variable, is a lighter-weight non-Pro alternative for a simple
scheduled diff check.

## Concurrency Groups and State Locks

Hand-rolled Terraform workflows often add a GitHub Actions `concurrency:` group to keep two runs
from touching the same state. Don't carry it over:

- A group holds one running and one pending run; a third trigger evicts the pending one, so queued
  deploys are silently dropped. It is not a FIFO deploy queue.
- `cancel-in-progress: true` kills a running `terraform apply` mid-write and can leave the state
  lock held. Recovery means confirming the run stopped, then `atmos terraform force-unlock`.

Terraform's state lock already prevents two writers. What's missing is waiting: Terraform's
`-lock-timeout` defaults to `0s`, so a run that finds the lock held fails immediately. Set a lock
timeout once and every `plan`/`apply`/`destroy`/`refresh`/`import` retries the held lock instead:

```yaml
# atmos.yaml
components:
  terraform:
    flags:
      lock_timeout: "5m"
```

Override it per stack (root-level `terraform.flags`) or per component (the component's `flags:`),
or set `ATMOS_COMPONENTS_TERRAFORM_FLAGS_LOCK_TIMEOUT`. For ordering across deploys, use the merge
queue, GitHub Environments, or Atmos Pro's dependency-ordered applies -- see the Concurrency Warning
in [atmos-ci](../../atmos-ci/SKILL.md).

## Other Actions Seen in the Wild

A short, honest list -- Atmos does not invent replacements it doesn't have:

| Action                                      | Fate                                                                 |
|-----------------------------------------------|-----------------------------------------------------------------------|
| `actions/checkout`                             | Stays, or becomes `atmos git clone` -- note the fork-PR trust gate on `pull_request_target`/`workflow_run` contexts (see [atmos-git](../../atmos-git/SKILL.md)) |
| `hashicorp/terraform-github-actions` (legacy/archived) | Same fate as `hashicorp/setup-terraform` + dflook combined -- toolchain + Native CI |

## Common Mistakes

- **Leaving `configure-aws-credentials` alongside `auth.providers`** -- double/conflicting
  credential resolution. Remove the action once `auth` is configured.
- **Leaving dflook's or tfcmt's PR-comment behavior AND `ci.comments` both enabled** -- duplicate
  comments on every PR.
- **Treating a `dflook workspace:` input as this reference's job** -- that's state-mapping, not
  CI-mapping. Route to [from-terraform-workspaces.md](from-terraform-workspaces.md).
- **Assuming `atmos terraform deploy` replays the exact reviewed plan like dflook does by default**
  -- it diffs stored-vs-fresh and applies fresh unless `--verify-plan=false`/`--from-plan` is used.
- **Reaching for `ci.templates` by default** instead of the native comment/summary format -- it's an
  escape hatch, not a starting point.
- **Porting a `concurrency:` group to serialize Terraform** -- it drops queued runs and, with
  `cancel-in-progress`, can strand a state lock. Set `components.terraform.flags.lock_timeout`
  instead.

## Related Skills

- **Native CI mechanics** (matrices, outputs, summaries, checks, comments) → [atmos-ci](../../atmos-ci/SKILL.md)
- **OIDC providers, identities, trust policies** → [atmos-auth](../../atmos-auth/SKILL.md)
- **Profile activation, directory layout, merge behavior** → [atmos-profiles](../../atmos-profiles/SKILL.md)
- **Tool versions, `dependencies.tools`, PATH behavior** → [atmos-toolchain](../../atmos-toolchain/SKILL.md)
- **Hook kinds, events, `when:` scoping** → [atmos-hooks](../../atmos-hooks/SKILL.md)
- **TFLint execution, config discovery, and rules** → [atmos-lint](../../atmos-lint/SKILL.md)
- **Back to the migration decision guide** → [atmos-migration SKILL.md](../SKILL.md)
