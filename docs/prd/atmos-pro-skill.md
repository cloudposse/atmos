# PRD: Atmos Pro Onboarding Skill

## Status: Draft

## Overview

Customers onboarding to Atmos Pro must assemble a non-trivial set of artifacts before their first
plan runs: GitHub OIDC trust, IAM roles in every target account, auth profiles, a `settings.pro`
mixin, four GitHub Actions workflows, and reciprocal state-backend trust. A recent guided
onboarding session made no progress because the starting conditions (GitHub Actions disabled
org-wide, no Atmos Auth, federated IAM via Okta, Geodesic-based dev environment) were not
anticipated and the team could not identify which stacks to modify.

This PRD proposes a new Atmos agent skill, `atmos-pro`, that encodes the full onboarding
contract so an AI agent can analyze any Atmos repo, generate the necessary files, and open a PR.
The skill is invocable by any conforming AI tool (Claude Code, Copilot, Codex, Gemini, Grok)
through the Agent Skills open standard, and directly through `atmos ai ask "setup atmos pro"
--skill atmos-pro`.

The skill is packaged identically to the 22 existing skills under `agent-skills/skills/`, follows
the same structure (SKILL.md + `references/`), and ships with the Atmos repo so it stays in sync
with Atmos and Atmos Pro versions.

## Problem Statement

1. **Onboarding is a multi-day project, not a command.** A known-good reference implementation
   spans roughly 1,900 additions across 28 files: a vendored IAM role component, catalog
   entries, two auth profile files, four workflows, a mixin, and two unrelated-looking
   tfstate-backend edits. None of it is discoverable without prior knowledge.
2. **Starting conditions vary more than expected.** From one real onboarding:

- GitHub Actions was disabled organization-wide for security reasons.
- The customer was not using Atmos Auth at all.
- The customer used federated IAM roles with Okta (not AWS Identity Center + Okta), so
  Atmos Pro works but Atmos Auth local-dev benefits do not automatically apply.
- Engineers used Geodesic shells with Atmos; Atmos Pro onboarding via Geodesic had never
  been exercised.

3. **Knowing *which* stacks and files to touch is the hardest part.** Even experienced
   operators cannot, by inspection, identify which `_defaults.yaml` to edit, which catalog to
   add, or which accounts need the IAM role deployed.
4. **The `settings.pro` dispatch contract is undocumented in user-facing material.** The
   mapping from PR event → workflow → inputs lives only inside example mixins and the Atmos Pro
   dispatcher.
5. **Safety rails are invisible.** Root-account always-plan, default-restrictive branch scoping,
   reciprocal tfstate trust — all are policy decisions that must happen correctly on the first
   try or the first apply breaks or (worse) succeeds with too much privilege.
6. **There is no portable capture of the working setup.** Each onboarding rediscovers the same
   structure from scratch. The knowledge needs to live with Atmos.

## Goals

1. Ship an `atmos-pro` skill under `agent-skills/skills/atmos-pro/` that encodes the full
   onboarding contract.
2. Make the end-to-end flow a single AI invocation:
   `atmos ai ask "setup atmos pro" --skill atmos-pro` produces a worktree with all changes
   committed and optionally opens a PR.
3. Detect and name the starting-condition variants that actually occur (no Atmos Auth, federated
   Okta, Geodesic, Actions disabled) and branch the generated output accordingly.
4. Generate artifacts that match the shape of the known-good reference implementation, not a
   cleaner hypothetical version — we are encoding what we know works.
5. Keep the skill portable across AI tools via the Agent Skills open standard; do not hard-code
   Claude-specific behavior in the skill itself.
6. Keep the skill in sync with Atmos and Atmos Pro versions by co-locating it in this repo.

## Non-Goals

1. **Not a deterministic code generator.** The skill guides an AI agent; it does not replace one.
   A pure-Go `atmos pro init` command may follow, but it is out of scope for this PRD.
2. **Not an IAM-role provisioner.** The skill writes Terraform and YAML; the user still runs
   `atmos terraform apply` (or an apply workflow) to create roles in AWS.
3. **Not an Atmos Auth adopter.** If a repo has no `auth:` config, the skill generates
   `profiles/github-{plan,apply}/atmos.yaml` that work standalone; it does not retrofit Atmos
   Auth onto local developer workflows.
4. **Not a cloud-agnostic abstraction.** The first version targets AWS + GitHub Actions, matching
   what Atmos Pro actually supports today. Azure, GCP, and GitLab variants are future work.
5. **Not a Spacelift migrator.** The skill can disable Spacelift (`workspace_enabled: false`) but
   does not attempt to move state, mirror stack-level Spacelift settings, or reproduce Spacelift
   policy graphs.

## Audience

| Audience                                             | How they use the skill                                                           |
|------------------------------------------------------|----------------------------------------------------------------------------------|
| **New Atmos Pro customers**                          | `atmos ai ask "setup atmos pro" --skill atmos-pro` from their infra repo root    |
| **Onboarding engineers**                             | Same command; the skill captures the implicit playbook so sessions don't regress |
| **External AI tools** (Copilot, Codex, Gemini, Grok) | Load the skill via the Agent Skills standard and follow the same instructions    |
| **Existing Atmos Pro users**                         | Use the skill to add a new repo, org, or account to an existing workspace        |

## Reference Implementation

The canonical working output the skill should reproduce is a known-good, manually-assembled
customer repo that currently runs Atmos Pro in production. The artifact set:

- `components/terraform/aws/iam-role/` — vendored from `cloudposse-terraform-components/aws-iam-role v1.537.0`
- `stacks/catalog/aws/iam-role/{defaults,gha-tf}.yaml`
- `stacks/mixins/atmos-pro.yaml`
- `profiles/github-plan/atmos.yaml` and `profiles/github-apply/atmos.yaml`
- `.github/workflows/atmos-pro.yaml`, `atmos-pro-list-instances.yaml`, `atmos-terraform-plan.yaml`,
  `atmos-terraform-apply.yaml`
- Edit to `stacks/catalog/tfstate-backend/defaults.yaml` adding wildcard `allowed_principal_arns`
- `docs/atmos-pro.md`

This set is the ground truth. The skill's generators and reference docs should be validated by
replaying the skill against a fresh checkout and diffing against a sanitized copy kept under
`tests/fixtures/scenarios/atmos-pro-setup/golden/`.

## Skill Placement

```text
agent-skills/skills/atmos-pro/
  SKILL.md                          # frontmatter + top-level decision tree
  references/
    onboarding-playbook.md          # step-by-step playbook (what the skill does)
    starting-conditions.md          # repo-shape detection and variant selection
    artifact-catalog.md             # every file the skill generates, with why/when
    settings-pro-contract.md        # the settings.pro dispatch contract in full
    iam-trust-model.md              # OIDC sub-claim scoping, reciprocal tfstate, root safety
    auth-profiles.md                # identity naming, deep-merge mechanics, no-Atmos-Auth path
    geodesic-integration.md         # running the skill inside Geodesic
    troubleshooting.md              # failure-mode catalog with symptoms and fixes
  templates/
    mixins/atmos-pro.yaml.tmpl
    profiles/github-plan.yaml.tmpl
    profiles/github-apply.yaml.tmpl
    workflows/atmos-pro.yaml.tmpl
    workflows/atmos-pro-list-instances.yaml.tmpl
    workflows/atmos-terraform-plan.yaml.tmpl
    workflows/atmos-terraform-apply.yaml.tmpl
    docs/atmos-pro.md.tmpl
```

The skill is loaded by the AI agent before acting. Templates are literal text the agent fills in
from detected values; they are intentionally not rendered by Atmos itself, so the skill stays
tool-agnostic and any conforming AI can use it.

## User Workflow

### Primary entry point

```shell
atmos ai ask "setup atmos pro" --skill atmos-pro
```

This resolves to the Atmos AI CLI loading `atmos-pro/SKILL.md` and following its instructions
using whatever provider is configured (Claude Code, Codex, Gemini, etc.). The skill then drives
the agent through the flow below.

### The flow the skill prescribes

1. **Create an isolated worktree.** The agent runs `git worktree add .conductor/atmos-pro-setup
   feat/atmos-pro` (or equivalent) so the user's main checkout is never touched.
2. **Detect the repo shape.** The agent walks the repo and answers:

- What does the stack hierarchy look like? (tenants × stages × regions)
- Is Atmos Auth already configured?
- Is Spacelift in use? Which orgs?
- Is `github-oidc-provider` already deployed?
- Is there an existing `tfstate-backend` component? What's its `allowed_principal_arns`?
- Are GitHub Actions enabled on the repo? (Detectable via `gh api repos/:owner/:repo/actions/permissions`.)
- Is the repo run inside Geodesic? (Detectable via `Dockerfile`, `.envrc`, `geodesic.mk`.)

3. **Confirm the plan with the user.** The agent prints a summary: which org is being enabled,
   which accounts will get IAM roles, what the branch scope will be, which workflows will be
   added. The user approves before any files are written.
4. **Generate artifacts.** The agent writes templates under the worktree, filling in account
   IDs, tenant/stage names, and role ARNs from the detected values.
5. **Self-validate.** The agent runs `atmos validate stacks` and `atmos describe component
   aws/iam-role/gha-tf-plan -s <one-stack>` inside the worktree. It reports failures and stops.
6. **Open a PR (optional).** If `gh` is authenticated, the agent runs `gh pr create` with a
   template body that includes the rollout checklist (deploy IAM roles per account, deploy
   tfstate-backend trust update, run `oidc-test.yaml`, open a test PR).

### Starting-condition branches

The skill must explicitly handle the variants we've seen:

| Variant                                                            | Skill behavior                                                                                                                                                                                                       |
|--------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **GitHub Actions disabled org-wide**                               | Detect via API; refuse to proceed and print the exact org-settings URL and required permissions                                                                                                                      |
| **No Atmos Auth in repo**                                          | Generate standalone `profiles/github-{plan,apply}/atmos.yaml` without expecting an auth provider in `atmos.yaml`; emit a "next steps" doc for adopting Atmos Auth locally later                                      |
| **Federated IAM via Okta (not Identity Center)**                   | Skip the local-dev Atmos Auth recommendation; generate only the CI-side profiles; note that local dev continues using the customer's existing Okta federation                                                        |
| **Geodesic-based dev environment**                                 | Emit an additional `docs/atmos-pro.md` section explaining how to run the setup inside Geodesic (bind-mount the worktree, `gh` token passthrough, `atmos ai` invocation); validate against a Geodesic container in CI |
| **Spacelift currently enabled**                                    | Set `settings.spacelift.workspace_enabled: false` in the mixin and flag the migration in the PR description; do not attempt to move state or mirror stack policies                                                   |
| **`github-oidc-provider` not deployed**                            | Detect via `atmos describe component github-oidc-provider -s <stack>`; add a step-0 to the rollout checklist to deploy it first                                                                                      |
| **tfstate-backend `allowed_principal_arns` already has wildcards** | Merge additively; never overwrite a user-edited list                                                                                                                                                                 |

## What the Skill Knows

### The generated artifact set

The skill ships with templates for every file in the reference implementation. Generation rules:

- **Mixin** (`stacks/mixins/atmos-pro.yaml`): single entry point. Imports the IAM role catalog,
  sets `settings.spacelift.workspace_enabled: false`, declares the full `settings.pro` block
  including `pull_request.{opened,synchronize,reopened,merged}` and `drift_detection.{detect,remediate}`.
- **Auth profiles**: one identity entry per `{tenant}-{stage}` combination discovered in the
  stack hierarchy. The plan profile points all identities at `*-gha-tf-plan`. The apply profile
  points all identities at `*-gha-tf-apply` *except* the root account, which is forced to
  `*-gha-tf-plan` (safety rail).
- **GitHub workflows**: the four-workflow set, parameterized by `ATMOS_VERSION` and
  `ATMOS_PRO_WORKSPACE_ID` GitHub variables. All run in `ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}`.
- **IAM role catalog**: `defaults.yaml` (abstract, `main`-branch-scoped, OIDC enabled) and
  `gha-tf.yaml` (two concrete roles plus an abstract with reciprocal tfstate access). Vendored
  from `cloudposse-terraform-components/aws-iam-role`.
- **tfstate-backend edit**: additive merge of `allowed_principal_arns` with wildcard patterns
  matching the new role names.
- **Documentation**: `docs/atmos-pro.md` in the target repo, describing what was generated and
  the rollout procedure.

### The `settings.pro` dispatch contract

The skill's `references/settings-pro-contract.md` captures the full contract so the AI does not
have to infer it:

```yaml
settings:
  pro:
    enabled: true
    drift_detection:
      enabled: true
      detect:
        workflows:
          atmos-terraform-plan.yaml:
            inputs: { component, stack, upload }
      remediate:
        workflows:
          atmos-terraform-apply.yaml:
            inputs: { component, stack }
    pull_request:
      opened: &plan
        workflows:
          atmos-terraform-plan.yaml:
            inputs: { component, stack }
      synchronize: *plan
      reopened: *plan
      merged:
        workflows:
          atmos-terraform-apply.yaml:
            inputs: { component, stack, github_environment }
```

This is the actual contract Atmos Pro dispatches against, captured verbatim so the skill does
not hallucinate keys.

### Safety rails encoded in the skill

1. **Default-restrictive branch scoping.** IAM role defaults scope OIDC trust to the `main`
   branch. The planner role explicitly opts out (needs PR branches). The apply role never
   opts out.
2. **Root-account always-plan.** In `profiles/github-apply/atmos.yaml`, the root-account
   identity is forced to the plan role's ARN. Comment explains why.
3. **Reciprocal trust must deploy together.** The skill refuses to output the IAM role edit
   without the tfstate-backend edit. Both appear in the same PR.
4. **`trusted_github_repos` subject-claim translation** is captured in
   `references/iam-trust-model.md` so the AI does not misinterpret the syntax:

- `"org/repo:main"` → `ref:refs/heads/main`
- `"org/repo"` → `:*` (any ref, any PR)
- `"org/repo:environment:<name>"` → GitHub Environment-gated

5. **GitHub Environment scoping** is offered as an opt-in for apply; the skill asks before
   choosing.

### Identity naming

The skill's `references/auth-profiles.md` names the resolution rule explicitly:

> For a stack `{org}-{tenant}-{region}-{stage}`, Atmos looks up identities under the name
> `{tenant}-{stage}/<role>`. The skill generates one identity block per detected
> `{tenant}-{stage}` pair. The role suffix distinguishes plan from apply: `gha-tf-plan` or
> `gha-tf-apply`.

Identity names are not configurable in v1 — matching the naming used by the reference
implementation is more important than supporting variants.

## Invocation Options

The skill supports three invocation modes, in order of preference:

1. **Interactive AI** (primary): `atmos ai ask "setup atmos pro" --skill atmos-pro`.
   The agent asks clarifying questions; user approves the plan; agent generates and validates.
2. **Non-interactive AI** (CI or scripts): `atmos ai ask "setup atmos pro" --skill atmos-pro
   --non-interactive --approve-all`. All detected defaults are accepted; only fatal detection
   failures stop the run.
3. **Direct skill load** (other AI tools): external tools load `agent-skills/skills/atmos-pro/`
   via the Agent Skills standard and drive the flow themselves. No Atmos-specific CLI needed.

A future `atmos pro init` command (deterministic, no AI) may wrap the same generators for users
who want reproducible non-AI runs. That is out of scope for this PRD but the skill's template
layout is deliberately compatible with it.

## Validation & Testing

1. **Unit**: template rendering tests for each generated file, covering variant paths (no
   Atmos Auth, Spacelift present/absent, root account present/absent).
2. **Integration**: a fixture repo under `tests/fixtures/scenarios/atmos-pro-setup/` representing
   a minimal multi-tenant stack hierarchy. Running the skill against the fixture must produce
   output matching a golden-snapshot of the reference PR (minus environment-specific values like
   account IDs).
3. **Contract**: a periodic CI job that replays the skill against a sanitized reference repo
   checked into the Atmos test fixtures and diffs the generated output against the golden
   snapshot. Drift fails CI.
4. **End-to-end**: a manual playbook (`references/onboarding-playbook.md`) that onboarding
   engineers follow and score against real customer sessions. Any session that cannot complete
   is a bug report against the skill.

## Delivery Plan

### Phase 1 — Skill scaffold and references (week 1)

- Create `agent-skills/skills/atmos-pro/SKILL.md` with frontmatter and the decision tree.
- Write the seven reference docs listed under "Skill Placement".
- Commit templates as plain files derived directly from the reference PR. No template
  parameterization yet.

### Phase 2 — Template parameterization and repo detection (week 2)

- Parameterize templates with Go-template placeholders the agent fills in.
- Document the detection probes (what files/commands the agent runs to infer repo state).
- Add the starting-condition branch table to `starting-conditions.md`.

### Phase 3 — Integration with `atmos ai ask` and the Skill index (week 3)

- Add `atmos-pro` to `agent-skills/AGENTS.md` skill index.
- Verify `atmos ai ask ... --skill atmos-pro` loads and runs the skill end-to-end with Claude
  Code and Codex.
- Write the validation fixture and golden snapshot.

### Phase 4 — Safety & UX (week 4)

- Implement the pre-flight detection (GitHub Actions enabled, `github-oidc-provider` deployed,
  tfstate backend discoverable).
- Implement the PR-opening step with the rollout checklist body.
- Ship `references/troubleshooting.md` populated from real onboarding failures.

### Phase 5 — Deterministic fallback (future; not in this PRD)

- Optional: an `atmos pro init` Go command that calls the same generators without an AI.
- Targets users who need reproducible, unattended runs (CI pipelines that bootstrap new orgs).

## Alternatives Considered

### A Go command (`atmos pro init`) as the primary entry point

A deterministic command is simpler to reason about and does not require an AI. We still intend
to ship one eventually (Phase 5). But the onboarding *pain* is not "I want a wizard"; it is
"I don't know what my repo already looks like and I don't know what to change." An AI agent
is the correct tool for the detection and branching problem. The Go command is the right tool
for the generation problem — and both can share the same templates.

### A template repo

GitHub template repos give users a starting point but cannot adapt to an *existing* repo. Our
target customer already has a large Atmos repo; they need surgery, not a fresh checkout.

### A Terraform module

A module cannot generate YAML manifests, GitHub workflows, or auth profiles in the repo
filesystem. It would only cover the IAM-role portion and leave every other artifact to
the user.

### Docs-only (improve `docs/atmos-pro.md`)

The failed onboarding session did not fail for lack of docs; it failed because the docs exist
only *after* someone has done the work. Even with perfect docs, translating them into the
specific file edits a given repo needs still takes hours. The skill closes that gap.

## Open Questions

1. **Does the skill write the `settings.pro` block into a mixin, or into per-org `_defaults.yaml`?**
   The reference PR uses a mixin imported by each org. That is cleaner but requires a second
   decision point (which orgs opt in). For customers with a single org, a direct
   `_defaults.yaml` edit may be simpler. The skill could detect and pick.
2. **Should `trusted_github_repos` default to `{org}/{repo}:main` or to a GitHub Environment?**
   The reference uses branch scoping. Environments give better approval gates but require the
   user to create them first. Proposed default: branch; offer Environment as an opt-in question.
3. **How do we test end-to-end without a real AWS account?** Phase 2 golden snapshots cover the
   generated files. Phase 4 reciprocal-trust validation requires either a fixture AWS account
   or a CI-only mocked backend. Needs owner.
4. **Geodesic integration**: should the skill require Geodesic, work equally well inside and
   outside, or emit a Geodesic-specific variant? We suspect "work equally well" but have never
   tested the outside case.
5. **Multi-repo OIDC**: the reference PR includes a "how to grant additional repos OIDC access
   to the same accounts" procedure. Is that a separate invocation of the skill
   (`atmos ai ask "add repo X to atmos pro"`), or a flag on the original?
6. **What happens when Atmos Auth *is* configured?** Does the skill retrofit the GitHub OIDC
   provider into the existing `auth:` block, or generate standalone profiles that shadow the
   existing config? The deep-merge semantics of `ATMOS_PROFILE` mean both can coexist, but
   we should pick a recommended path.

## References

- Existing skills directory: `agent-skills/skills/`
- Companion PRD: [`atmos-agent-skills.md`](./atmos-agent-skills.md)
- Companion PRD: [`atmos-mcp-integrations.md`](./atmos-mcp-integrations.md)
- Agent Skills open standard: <https://agentskills.io/specification>
- Atmos Pro docs entry point: <https://atmos-pro.com/start>
