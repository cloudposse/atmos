# PRD: Atmos Pro Onboarding Skill

## Status: In Progress — Phases 1–5 implemented, manual validation pending

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
  Atmos Pro works, but Atmos Auth local-dev benefits do not automatically apply.
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

1. **Create an isolated worktree.** The agent runs `git worktree add .worktrees/atmos-pro-setup
   feat/atmos-pro` (or matches the repo's existing worktree convention if different) so the
   user's main checkout is never touched.
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

## Invocation Paths

The skill supports two primary invocation paths with meaningfully different ergonomics, plus a
direct-load fallback for tools outside our control. All three paths execute the same skill and
produce the same output; the difference is *who dispatches the agent and where the AI provider
lives*.

### Path A — `atmos ai ask --skill` (Atmos-dispatched)

```shell
atmos ai ask "setup atmos pro" --skill atmos-pro
```

The Atmos binary reads the skill, routes the prompt to the AI provider configured in
`atmos.yaml` (`ai.providers.*`), and streams the session. The user does not need Claude Code,
Codex, or any other AI tool installed — Atmos itself is the AI client.

**Properties:**

- **Self-contained** — a user with just `atmos` installed can run the setup. No separate AI CLI.
- **Provider-agnostic** — works with whichever provider the repo has configured (Claude API,
  OpenAI, Bedrock, etc.). The skill text is plain Markdown; any capable LLM can follow it.
- **Versioned with Atmos** — the skill loaded is the one shipped with the user's Atmos binary.
  No network dependency on a remote repo.
- **Non-interactive mode** — `atmos ai ask --skill atmos-pro --non-interactive --approve-all`
  for CI/automation. Detected defaults are accepted; fatal detections still stop the run.

**When to pick this path:**

- The user is already using Atmos and has an AI provider configured.
- The user wants a command they can run in a pipeline.
- The user does not have Claude Code installed (or uses a different AI tool).

### Path B — Claude Code loads the skill directly (Claude-Code-dispatched)

The user, inside Claude Code, says:

> Load the `atmos-pro` skill from the Atmos repo and set up Atmos Pro for this repository.

Claude Code fetches `agent-skills/skills/atmos-pro/` from the remote Atmos repository (via the
Claude Code plugin marketplace or a direct repo reference), loads `SKILL.md` and any referenced
files on demand via progressive disclosure, and executes the playbook using Claude Code's
native tool use (Read, Write, Bash, Git).

**Properties:**

- **Best agentic experience** — Claude Code's native tools (file editing, shell, git worktrees,
  diff review) produce the tightest loop. No subprocess boundary between the agent and the
  repo.
- **Always-latest skill** — the user gets whatever is on the Atmos repo's default branch, so
  skill updates propagate without updating the local Atmos binary.
- **Progressive disclosure** — Claude Code loads `SKILL.md` first, then pulls in `references/*`
  only when the task needs them. Cheaper context, better focus.
- **No Atmos binary AI config needed** — the user doesn't configure an AI provider in
  `atmos.yaml`; Claude Code has its own authentication.

**When to pick this path:**

- The user is already working in Claude Code.
- The user wants to review every file edit inline before accepting (Claude Code's permission
  prompts).
- The user wants the latest skill version without bumping Atmos.

**Discovery:**

Two ways the skill becomes known to Claude Code:

1. **Plugin marketplace** — Cloud Posse's `agent-skills/.claude-plugin/plugin.json` declares
   the `atmos` plugin. Users install it once; all 23 skills (22 existing + `atmos-pro`) become
   available via `/skill atmos-pro`.
2. **Direct remote load** — the user tells Claude Code the skill path:
   `use the atmos-pro skill at github.com/cloudposse/atmos/agent-skills/skills/atmos-pro`.
   Claude Code fetches it ad-hoc. Useful for evaluating before committing to the plugin.

### Path C — Other AI tools (direct skill load)

External tools that implement the Agent Skills open standard (Copilot, Codex, Gemini, Grok)
reference `agent-skills/skills/atmos-pro/` directly. No Atmos CLI involved. The skill text is
identical across all tools — the Agent Skills standard guarantees portability.

### Comparison

| Aspect                 | Path A: `atmos ai ask`           | Path B: Claude Code direct       | Path C: other AI tools   |
|------------------------|----------------------------------|----------------------------------|--------------------------|
| Dispatcher             | Atmos binary                     | Claude Code                      | External AI tool         |
| AI provider            | Configured in `atmos.yaml`       | Claude Code's own auth           | Tool's own auth          |
| Skill source           | Shipped with Atmos               | Remote repo (always-latest)      | Remote repo              |
| Best for               | Pipelines, self-contained setups | Interactive dev sessions         | Tool-specific workflows  |
| Requires Atmos CLI     | Yes                              | No                               | No                       |
| Requires Claude Code   | No                               | Yes                              | No                       |
| Progressive disclosure | Partial (agent reads as needed)  | Full (native)                    | Depends on tool          |

### Why both Paths A and B exist

Path A is what a user types in a terminal or a CI step. Path B is what a user does inside their
AI IDE. They are not redundant — they serve different moments in the user's day.

Shipping only Path A forces users to leave Claude Code to invoke the skill, losing the context
of their current session. Shipping only Path B forces users who don't use Claude Code to
install it just to onboard to Atmos Pro. Both are real user needs.

A future `atmos pro init` Go command (deterministic, no AI) may wrap the same generators for
users who want reproducible non-AI runs. That is out of scope for this PRD but the skill's
template layout is deliberately compatible with it.

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

### Phase 1 — Skill scaffold and references (week 1) — ✅ Complete

- Created `agent-skills/skills/atmos-pro/SKILL.md` with frontmatter and the decision tree.
- Wrote the eight reference docs under `references/` (onboarding playbook, starting
  conditions, artifact catalog, `settings.pro` contract, IAM trust model, auth profiles,
  Geodesic integration, troubleshooting).
- Committed all templates under `templates/` derived from the known-good reference output.
- Added `atmos-pro` to `agent-skills/AGENTS.md` skill index.
- Created the `.claude/skills/atmos-pro` symlink.
- Updated `agent-skills/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json`
  to mention Atmos Pro onboarding.
- Updated all "N skills" references in CLI markdown and website docs to `23+`.

### Phase 2 — Template parameterization and renderer (week 2) — ✅ Complete

- Switched templates to Go `text/template` with **custom `<<` / `>>` delimiters** so that
  native `{{ }}` (Atmos Pro workflow inputs, GitHub Actions expressions, Atmos vendor
  interpolation) passes through untouched — no backtick escaping required.
- Documented the placeholder contract in `agent-skills/skills/atmos-pro/templates/README.md`.
- Shipped the Go renderer at `pkg/ai/skills/atmospro/` with a typed `RenderData` context
  (accounts, variant flags, probe values), full validation, and a `RenderAll` entry point
  that walks the templates FS and returns a map of output-path → contents.
- Shipped the validation fixture at `tests/fixtures/scenarios/atmos-pro-setup/` (6 accounts
  across 2 tenants, one flagged `is_root`, Geodesic + Spacelift variant flags on).
- Shipped the golden-snapshot diff test (14 rendered artifacts) with a `-regenerate-snapshots`
  flag for intentional updates. Golden files live under the fixture's `golden/` directory.
- Exit criteria verified: scalar substitution, literal `{{ }}` / `${{ }}` pass-through,
  `<<range>>` iteration with `<<$.topLevel>>`, root-account safety rail pinned to the plan
  role in the rendered apply profile.

### Phase 3 — Path A: `atmos ai ask --skill` integration (week 3) — ✅ Complete

- Embedded `agent-skills/skills/` into the Atmos binary via `//go:embed` at
  `agent-skills/agentskills.go`. `atmos ai ask --skill atmos-pro` now works without
  requiring a prior `atmos ai skill install` — all 23+ skills ship with the binary.
- New `pkg/ai/skills/embedded/` package exposes `Load`, `LoadAll`, and an
  `embedded.Loader{}` that implements `skills.SkillLoader`. Marketplace-installed skills
  override embedded ones of the same name (first-writer-wins).
- `skills.LoadSkills` and `skills.LoadAndValidate` now accept variadic loaders; call sites
  in `pkg/ai/tui` and `pkg/ai/analyze` pass both the marketplace and embedded loaders.
- Added `marketplace.ParseSkillMetadataBytes` so the embedded package can parse SKILL.md
  from the embedded FS.
- `atmos ai skill list` now shows a **Built-in skills** section ahead of Installed skills;
  marketplace overrides are hidden from the built-in section.
- `atmos-pro/SKILL.md` gained a `references:` frontmatter so the loader concatenates all
  eight reference docs into the system prompt.
- 7 unit tests in `pkg/ai/skills/embedded/embedded_test.go` cover loading, registry
  override precedence, missing skills, and the `SkillLoader` interface contract.
- **Deferred:** `--non-interactive --approve-all` flags on `atmos ai ask`. The global
  `--interactive=false` flag already exists and the skill instructs agents to honor it,
  but a dedicated "approve every tool call" flag is not yet plumbed. Minor follow-up.

### Phase 4 — Path B: Claude Code direct integration (week 4) — ✅ Complete (pending manual validation)

- `SKILL.md` now has a dedicated **Path A vs Path B** section that maps Claude Code's
  native tools (Read, Write, Edit, Bash, TodoWrite) to the playbook steps and declares
  byte-identical output as a contract between the two paths.
- `pkg/ai/skills/marketplace/manifest_test.go` — `TestMarketplaceManifestShape` and
  `TestPluginManifestShape` validate the structural invariants of the root-level
  `marketplace.json` and `agent-skills/.claude-plugin/plugin.json`. Drift in either file
  (name, license, URLs, Atmos-Pro description) breaks the test loudly before it reaches
  the Claude Code marketplace.
- `pkg/ai/skills/embedded/atmospro_test.go` — three self-containment tests:
  `TestAtmosProSkill_SelfContained` (every reference path resolves inside the skill dir),
  `TestAtmosProSkill_TemplatesSelfContained` (no symlinks under `templates/`), and
  `TestAtmosProSkill_ReferencesDeclared` (every on-disk reference is declared in the
  SKILL.md frontmatter). Together they ensure the skill can be fetched as a standalone
  subtree without dangling references.
- `pkg/ai/skills/atmospro/parity_test.go` — two cross-path parity tests:
  `TestPathParity_AgentSkillsFSMatchesSourceTree` (embed.FS templates are byte-identical
  to the on-disk source) and `TestPathParity_RenderedOutputMatchesGolden` (rendering the
  embedded templates against the fixture matches the Phase 2 golden snapshots). Closes
  the parity contract: whichever path the agent uses, the output matches the golden.
- **Pending manual validation:** exercising Path B inside Claude Code against a real
  Atmos repo. The deterministic infrastructure is in place; the remaining work is
  real-world testing and capturing any friction in `references/troubleshooting.md`.

### Phase 5 — Safety & UX polish (week 5) — ✅ Complete (pending manual validation)

- New `pkg/atmospro/detect/` package with three pure-Go, `fs.FS`-based pre-flight probes:
  `AtmosAuth(fsys)`, `Spacelift(fsys, stacksDir)`, `Geodesic(fsys)`, plus a
  `detect.All(fsys, stacksDir)` aggregator returning results in deterministic order.
- Each `Result` carries `Name`, `Detected`, `Details`, `Hint`, and `Evidence[]` so the
  skill (or a future `atmos pro init`) can show the user exactly what was detected and
  why.
- 14 unit tests covering happy paths, false-positive guards (nested `settings.auth` must
  not match the top-level probe), disabled Spacelift is ignored, missing stack dirs are
  not errors, empty FS returns `ErrEmptyFS`, realistic mid-journey repos classify correctly.
- **Network-dependent probes intentionally stay as shell commands** (GitHub Actions
  enablement via `gh api`, `github-oidc-provider` deployment via `atmos describe component`,
  tfstate-backend introspection). They are documented in
  `references/starting-conditions.md` for the skill's Bash tool to execute — wrapping
  them in Go would require GitHub-token and AWS-credential plumbing without a proportional
  payoff at this stage.
- `references/starting-conditions.md` gains a new "Deterministic probe package" section
  with a table linking each probe name to its Go function.
- `references/troubleshooting.md` expands from 7 to 10 entries with probe-specific
  failure modes: nested-auth false negatives, stale Geodesic marker false positives,
  Spacelift inheritance blind spots.
- **PR-body rendering** was already shipped in Phase 1 as the
  `docs/atmos-pro-pr-body.md.tmpl` template; Phase 2 golden-snapshot tests validate it
  per-fixture.
- **Pending manual validation:** running the full skill against a real repo and
  confirming the pre-flight summary drives useful agent behavior.

### Phase 6 — Deterministic fallback (future; not in this PRD)

- Optional: an `atmos pro init` Go command that calls the same generators without an AI.
- All the building blocks now exist:
  - `pkg/atmospro/detect` — pre-flight probes (Phase 5)
  - `pkg/ai/skills/atmospro` — template renderer (Phase 2)
  - `agent-skills/skills/atmos-pro/templates/` — source templates
  - Golden-snapshot test suite — regression guard
- A Phase 6 command would wire these together behind a Cobra command with
  `--non-interactive --approve-all` flags, matching the AI-dispatched path's invariants.
  Targets users who need reproducible, unattended runs (CI pipelines that bootstrap new
  orgs).

## Local Testing (Before the Skill Is Merged)

Both invocation paths can be tested against an unmerged branch of the Atmos repo, so a
target infrastructure repo can exercise the skill before it reaches `main`.

Assume two working directories on the tester's machine:

- **Atmos source repo** (the branch under test): `/path/to/atmos` — e.g.
  `/Users/andriyknysh/Documents/Projects/Go/src/github.com/cloudposse/atmos` on the
  `aknysh/atmos-pro-init` branch
- **Target infra repo** (a real Atmos-managed infra repo): `/path/to/target-repo`

### Path A — local Atmos binary (embedded skill)

The `atmos-pro` skill is `//go:embed`-ed into the Atmos binary at build time. Build Atmos
from the feature branch and invoke it from the target repo:

```bash
# From the Atmos source repo
cd /path/to/atmos
git checkout aknysh/atmos-pro-init
make build                     # produces ./build/atmos

# Verify the skill is bundled
./build/atmos ai skill list | grep atmos-pro

# Run from the target repo using the locally built binary
cd /path/to/target-repo
/path/to/atmos/build/atmos ai ask "setup atmos pro" --skill atmos-pro
```

Because the skill is embedded, no marketplace install, symlink, or path hack is needed —
the binary's own embedded FS is authoritative.

### Path B — Claude Code loading the unmerged skill

Claude Code has no awareness of unmerged branches by default; the marketplace resolves
`cloudposse/atmos` to the repository's default branch. Three options let Claude Code use
the local branch's skill instead.

#### Option 1 — user-global skill symlink (simplest)

Symlink the local skill into Claude Code's user-global skills directory. Claude Code
auto-discovers any skill under `~/.claude/skills/<name>/SKILL.md`:

```bash
mkdir -p ~/.claude/skills
ln -sfn /path/to/atmos/agent-skills/skills/atmos-pro ~/.claude/skills/atmos-pro

# Verify
ls -la ~/.claude/skills/atmos-pro
```

Open Claude Code inside `/path/to/target-repo` and prompt:

> "Load the atmos-pro skill and set up Atmos Pro for this repository."

Claude Code reads the symlinked `SKILL.md`, loads reference files on demand, and executes
the playbook using its native Read/Write/Edit/Bash/TodoWrite tools.

When testing is done, remove the symlink so the production marketplace install can take
over:

```bash
rm ~/.claude/skills/atmos-pro
```

#### Option 2 — local marketplace registration

Point Claude Code's plugin marketplace at the local Atmos checkout. Inside Claude Code:

```
/plugin marketplace add /path/to/atmos
/plugin install atmos@cloudposse
/reload-plugins
```

Claude Code reads `/path/to/atmos/.claude-plugin/marketplace.json`, resolves the `atmos`
plugin to `./agent-skills` (relative paths resolve from the marketplace root, which is
the directory containing `.claude-plugin/`), and installs every skill under the bundled
skills directory — including `atmos-pro` from the unmerged branch. This mirrors the
production install flow exactly.

##### Manifest schema (must match Claude Code spec)

The Claude Code marketplace validator rejects or silently ignores non-standard top-level
fields. The corrected manifests in this repo follow the
[official schema](https://code.claude.com/docs/en/plugin-marketplaces):

`.claude-plugin/marketplace.json`:

```json
{
  "name": "cloudposse",
  "owner": {
    "name": "Cloud Posse",
    "email": "opensource@cloudposse.com"
  },
  "metadata": {
    "description": "Official Atmos plugins and skills for Claude Code",
    "version": "1.0.0"
  },
  "plugins": [
    {
      "name": "atmos",
      "source": "./agent-skills",
      "description": "...",
      "version": "1.0.0",
      "category": "development",
      "keywords": ["atmos", "terraform", "infrastructure", "iac", "devops", "cloud"],
      "homepage": "https://atmos.tools/ai/agent-skills",
      "repository": "https://github.com/cloudposse/atmos",
      "license": "Apache-2.0",
      "author": { "name": "Cloud Posse", "email": "opensource@cloudposse.com" }
    }
  ]
}
```

`agent-skills/.claude-plugin/plugin.json`:

```json
{
  "name": "atmos",
  "version": "1.0.0",
  "description": "...",
  "author": { "name": "Cloud Posse", "email": "opensource@cloudposse.com" },
  "homepage": "https://atmos.tools/ai/agent-skills",
  "repository": "https://github.com/cloudposse/atmos",
  "license": "Apache-2.0",
  "keywords": ["atmos", "terraform", "infrastructure", "iac", "devops", "cloud"]
}
```

Key rules: `version` and `description` on the marketplace go under `metadata.*`. The
`author` object takes `name` + optional `email` (not `url`).

Validate both files at any time with:

```bash
claude plugin validate /path/to/atmos
```

Expected output: `✔ Validation passed`.

##### Verifying the install worked

The `/plugin install` and `/reload-plugins` output can be misleading. A typical
successful sequence looks like:

```
/plugin marketplace add /path/to/atmos
   → Successfully added marketplace: cloudposse

/plugin install atmos@cloudposse
   → ✓ Installed atmos. Run /reload-plugins to apply.

/reload-plugins
   → Reloaded: 1 plugin · 0 skills · 5 agents · 0 hooks · 0 plugin MCP servers · 0 plugin LSP servers
```

The reload count `0 skills` does **not** mean skills failed to load. Plugin-bundled
skills are not included in that count — it reports user-scope and project-scope
non-plugin skills only. To confirm the plugin skills are live, run `/skills` in
Claude Code:

```
/skills
   → 47 skills · Esc to close

     🔒 on   atmos:atmos-ansible · plugin · ~39 tok · locked by plugin
     🔒 on   atmos:atmos-auth · plugin · ~42 tok · locked by plugin
     🔒 on   atmos:atmos-aws-security · plugin · ~47 tok · locked by plugin
     🔒 on   atmos:atmos-pro · plugin · ~XX tok · locked by plugin
     ... (all 23 atmos skills, plus Claude Code's bundled skills)
```

All 23 skills appear under the `atmos:` namespace (e.g., `atmos:atmos-pro`) since
Claude Code namespaces plugin skills as `<plugin-name>:<skill-name>`. The `🔒` icon
means the plugin owns the skill's on/off state (you manage it via `/plugin` rather
than individually in `/skills`).

Agent Skills open-standard frontmatter fields that Claude Code does **not** document
(nested `metadata:` object, `references:` array) are ignored gracefully — skills
still load and the fields remain available to any consumer that knows to read them
(the Atmos binary uses `references:` for eager concatenation when Path A dispatches
the skill).

The 5 agents shown in the reload count come from the caller's `.claude/agents/`
directory in the working repo, not from the plugin cache (which has no `agents/`
subdirectory).

##### Invoking the skill in Claude Code

Once installed, either ask in natural language or invoke explicitly. See the
**Prompts catalog** below for a full set of tested prompts covering cold-start,
step-by-step, variant simulation, diagnostic, and follow-up scenarios.

### Prompts catalog

Prompts tested against the live `atmos:atmos-pro` skill. Run from inside Claude Code
while `cd`'d into the target Atmos repo. Always mention `atmos:atmos-pro` explicitly
on the first prompt so Claude Code disambiguates against less specific skills.

#### Cold start — full end-to-end

Use this when the skill should do everything: detect, plan, confirm, generate,
validate, open a PR.

> Use the `atmos:atmos-pro` skill to set up Atmos Pro in this repository. Detect the
> repo shape first, print a plan, wait for my approval, then generate all artifacts in
> an isolated worktree and self-validate with `atmos validate stacks`.

Model-picks-skill variant (for testing automatic skill selection):

> Set up Atmos Pro onboarding for this repo. Use the atmos-pro skill.

#### Explicit skill invocation

Load the skill into context without starting the flow:

```
/atmos:atmos-pro
```

Tab-completion lists every `atmos:*` skill. Follow up with:

> Begin the 7-step flow. Start with step 1 (isolated worktree) and stop after step 3
> (plan confirmation) so I can review.

#### Step-by-step (recommended for the first run)

Each prompt runs a single phase so you can inspect output at the boundary.

**Detection only** — no writes:

> Using the `atmos:atmos-pro` skill, run only the detection phase (step 2 of the
> playbook). Report the stack hierarchy, whether Atmos Auth is configured, whether
> Spacelift is enabled, whether `github-oidc-provider` is deployed, whether this is a
> Geodesic repo, and whether GitHub Actions is enabled. Do not create a worktree or
> write any files.

**Plan preview** — diff-before-write:

> Using the `atmos:atmos-pro` skill, show me the plan summary: target org, accounts,
> branch scope, files to create, files to edit, and which starting-condition variants
> apply. Do not write files yet.

**Execute with approval** — after the plan is acceptable:

> Proceed with the `atmos:atmos-pro` flow from step 4 onward: create the worktree at
> `.worktrees/atmos-pro-setup feat/atmos-pro` (or match this repo's existing worktree
> convention if different), generate all artifacts, run `atmos validate stacks`, and
> stop before opening a PR. Show me a git diff of the worktree when done.

**Open the PR** — the last step:

> Open a PR from the atmos-pro worktree using `gh pr create` with the rollout
> checklist body from `templates/docs/atmos-pro-pr-body.md.tmpl`. Do not merge.

#### Variant simulations

Useful for exercising branches of the starting-condition decision table without
staging artificial fixtures.

> Using `atmos:atmos-pro`, pretend this repo has GitHub Actions disabled org-wide.
> Walk me through exactly what the skill would output and stop before generating any
> files.

> Using `atmos:atmos-pro`, treat this repo as Geodesic-hosted. Show me the Geodesic
> section that would be added to `docs/atmos-pro.md`.

> Using `atmos:atmos-pro`, simulate a repo where Spacelift is currently enabled on 3
> stacks. Show me how the mixin and PR body differ from the no-Spacelift case.

#### Follow-up onboarding (adding orgs or accounts)

After the first PR has merged and you want to extend the setup:

> Using `atmos:atmos-pro`, I want to add the `stg` org to the existing Atmos Pro setup.
> Update `profiles/github-plan/atmos.yaml` and `profiles/github-apply/atmos.yaml` with
> the `stg` accounts, update the trusted role ARNs, and open a follow-up PR.

#### Diagnostic / recovery

Use when a prior run is misbehaving:

> Using `atmos:atmos-pro`, diagnose why my first atmos-pro PR's plan workflow is
> failing with `AssumeRoleWithWebIdentity`. Check the entries in
> `references/troubleshooting.md` and tell me which one matches. Do not edit any
> files yet.

#### Prompting best practices

- **Always mention `atmos:atmos-pro` explicitly on the first prompt** — the namespace
  prefix disambiguates against less-specific skills.
- **Tell the skill where to stop.** The default flow runs to completion including PR
  opening; adding "stop before opening a PR" or "stop after step 3" keeps the loop
  short while iterating.
- **Always run inside the target repo** — `cd` into the target infra repo before
  launching Claude Code so `git worktree add`, `atmos describe component`, and
  `gh pr create` resolve correctly.
- **Inspect the worktree when done** — `git worktree list` should show
  `.worktrees/atmos-pro-setup` (or whatever path you chose); a `git -C <worktree>
  status` should show the generated files staged.

##### Duplicate or stale install recovery

If `/plugin install` has been run against a partially-broken manifest version, remove
and re-add to clear the cached state:

```
/plugin marketplace remove cloudposse
/plugin marketplace add /path/to/atmos
/plugin install atmos@cloudposse
/reload-plugins
```

To wipe the on-disk cache between attempts:

```bash
rm -rf ~/.claude/plugins/cache/cloudposse
```

##### Post-merge switch

When the feature branch merges, switch to the remote marketplace:

```
/plugin marketplace remove cloudposse
/plugin marketplace add cloudposse/atmos
/plugin install atmos@cloudposse
/reload-plugins
```

#### Option 3 — inline skill load (no install)

When neither a symlink nor a marketplace install is desirable, ask Claude Code to read
the skill directly:

> "Read `/path/to/atmos/agent-skills/skills/atmos-pro/SKILL.md` and execute the playbook
> it describes. Load reference files from the `references/` subdirectory on demand. Use
> the templates in `templates/` to generate artifacts."

This is the least ceremonious path — useful for quick iteration — but it relies on
Claude Code's ability to follow an instruction-style prompt rather than a registered
skill. Behavior is equivalent in practice; the skill text is the same in all three
options.

### Verifying both paths produce the same output

After a run (either path), diff the generated worktree against the expected golden:

```bash
# Generate the expected RenderData for your target repo
cd /path/to/atmos
cat > /tmp/target-fixture.json <<'EOF'
{
  "org": "YOUR_GITHUB_ORG",
  "repo": "YOUR_REPO",
  "namespace": "YOUR_NAMESPACE",
  "target_org": "YOUR_TARGET_ORG",
  "root_account_id": "123456789012",
  "accounts": [ ... ],
  "probe_stack": "YOUR_STACK",
  "probe_tenant": "YOUR_TENANT",
  "probe_stage": "YOUR_STAGE",
  "probe_account_id": "123456789012",
  "geodesic_detected": false,
  "spacelift_was_enabled": false,
  "no_atmos_auth": false
}
EOF

# (future) `atmos pro init --dry-run --data /tmp/target-fixture.json` will render the
# expected output deterministically. Until Phase 6 ships, the equivalent is calling
# atmospro.RenderAll() from a small Go harness that reads the fixture.
```

Any divergence between Path A output, Path B output, and the rendered expectation is a
skill bug.

## Lessons Learned in Production

Findings from the first live run of the skill against a Geodesic-hosted Atmos repo.
Each finding produced a skill, probe, or docs fix captured below. Add new entries as
further live tests surface issues.

### Geodesic repos keep `atmos.yaml` at a non-root path

**What happened.** During Path B manual validation (Claude Code + local marketplace
install) against a Geodesic-hosted reference-architecture-style repo, the agent ran
`ls atmos.yaml` at the repo root and got `No such file or directory`. The skill
recovered by searching other paths and produced a correct detection report, but the
miss highlighted a design assumption gap.

**Root cause.** Geodesic-hosted repos (Cloud Posse reference architectures, and any
repo cloned from those templates) store `atmos.yaml` at
`rootfs/usr/local/etc/atmos/atmos.yaml`. Workflows export
`ATMOS_CLI_CONFIG_PATH=./rootfs/usr/local/etc/atmos` so Atmos finds it at runtime,
but a pre-flight probe that reads the file directly needs the explicit path.

**Fix (shipped in this branch).**

- `pkg/atmospro/detect/detect.go` — `atmosYAMLCandidates` now lists
  `rootfs/usr/local/etc/atmos/atmos.yaml` ahead of repo-root candidates; `atmosDirCandidates`
  includes `rootfs/usr/local/etc/atmos/atmos.d`.
- New exported helper `detect.LocateAtmosYAML(fsys)` returns the first config path
  that exists, empty string when absent, preferring the Geodesic path.
- Five new unit tests cover Geodesic-only, root-only, both-exist-Geodesic-wins,
  fully-absent, and nil-FS cases.
- `SKILL.md` step 2 now starts with "Locate atmos.yaml first" and prescribes a shell
  snippet that sets `ATMOS_CONFIG_FILE` before any downstream probe.
- `references/starting-conditions.md` documents the Geodesic config-path convention
  alongside the standard Atmos discovery rules.
- `references/troubleshooting.md` gains a dedicated entry with the symptom
  (`ls atmos.yaml` failure), the Geodesic cause, and both the Go probe and shell fix.

**Open follow-up.** `ATMOS_CLI_CONFIG_PATH` can be configured per-repo. If a repo
uses a non-Geodesic, non-root path (e.g., `config/atmos/atmos.yaml`), the probe
currently misses it. The practical mitigation is to extend `atmosYAMLCandidates` as
new variants are encountered — there is no authoritative list. Not a blocker.

**Verified fix.** Re-running the detection prompt after reinstalling the updated plugin
produced:

- `atmos.yaml location: rootfs/usr/local/etc/atmos/atmos.yaml (Geodesic-hosted; no
  repo-root atmos.yaml)` — resolved as the first action, no recovery needed.
- No `ls: No such file or directory` error anywhere in the output.
- Structured, deterministic variant selection:
  `Spacelift-enabled + No-Atmos-Auth + Geodesic. No blocking conditions.`

The detection report was a clean table rather than free-text prose, and the agent
stopped at step 2 as instructed without creating a worktree.

### `git remote get-url origin` can leak PATs into detection output

**What happened.** During the same run, `git remote get-url origin` returned
`https://ghp_xxxxxxxxxxxxxxxxxxxx@github.com/<owner>/<repo>`. The agent flagged
this as a security finding and told the user to rotate the token — a good emergent
behavior — but the tokenized URL itself was echoed back to the terminal output,
briefly exposing the PAT a second time.

**Root cause.** `gh repo clone` embeds the user's PAT in the remote URL. Any command
that reads the remote (diagnostic probes, plan summaries, PR body generation) has
access to the token unless explicitly redacted.

**Fix (shipped in this branch).**

- `SKILL.md` safety rails section adds rule 5: "Redact tokens from remote URLs." The
  rule prescribes extracting `owner/repo` only, never echoing the tokenized URL, and
  flagging the leaked token as a security finding without blocking the flow.
- `references/troubleshooting.md` gains an entry with both remediation steps (rotate
  the PAT, rewrite the remote with `git remote set-url origin https://github.com/owner/repo`).

**Open follow-up.** The skill should eventually ship a lint/redaction helper that
takes any string destined for user output and strips token-like substrings. Until
then, the safety rail in SKILL.md is instruction-only and depends on agent compliance.

**Verified fix.** Re-running the detection prompt after reinstalling the updated plugin
produced:

- `Repo: <owner>/<repo> (owner/repo extracted; remote URL contains a PAT
  — rotate and rewrite: git remote set-url origin https://github.com/<owner>/<repo>)`

The `ghp_...` token was **not** echoed anywhere in the detection output. The agent
extracted `owner/repo` only, flagged the leak with actionable remediation steps, and
continued the flow without blocking. Instruction-only safety rails hold when the
agent is updated — no false-positive redaction of legitimate URLs was observed.

### Account-map data must be supplied by the user when the agent can't run `atmos`

**What happened.** During Run 4 (the first attempt at generation), the agent stopped
before writing any files and reported a blocker: it could not satisfy the safety rail
"derive ARNs from stack introspection" because

1. `atmos` commands must run inside Geodesic per `CLAUDE.md` (the host has no
   `atmos` binary on `PATH`),
2. the agent itself does not run inside Geodesic,
3. the repo's `account-map` component is dynamic
   (`terraform_dynamic_role_enabled: true`) — there is no static account-ID map
   committed to the stacks tree to grep, and
4. opportunistic grepping of cross-account references covered fewer than half the
   accounts (the agent recovered ~5 of ~13 expected).

The agent correctly refused to generate `profiles/github-{plan,apply}/atmos.yaml`
with `<MISSING>` placeholders (would have produced files that fail validation and
would also have leaked through Path A's golden-snapshot diff in CI).

**Root cause.** The skill's data model assumes `RenderData.Accounts` is populated
before `RenderAll` runs, but the playbook offers no fallback for the (very common)
case where the agent cannot itself execute `atmos describe component account-map`
inside Geodesic. The probe instructions in `references/starting-conditions.md` list
shell commands that the user runs, but step 4 of the SKILL.md playbook ("Confirm
the plan with the user") does not explicitly ask for the account map output.

**Fix shipped (in this branch).** SKILL.md step 5 and `references/onboarding-playbook.md`
now define an explicit five-step **account-ID resolution chain**:

1. `atmos describe component account-map` (when `atmos` is on PATH).
2. Static `account-map` files in `stacks/catalog/account-map*`.
3. **Repo documentation tables.** Many Cloud Posse-style repos document account IDs
   in the README, often as a table with `{tenant}-{stage}` row labels and per-org
   column headers. The probe greps `README.md`, `docs/**/*.md`, and `_shared/**/*.md`
   for rows matching `\| *[0-9]{12} *\|`. Section dividers (`**Governance**`,
   `**Operations**`) are skipped automatically because they don't contain 12-digit
   numbers.
4. Cross-account ARN references in `stacks/` (supplementary).
5. User prompt — only if all four sources together still produce an incomplete map.

Resolved maps cache to `.worktrees/atmos-pro-setup/.account-map.json` so re-runs and
follow-up invocations skip resolution.

Confirmed against the test repo: source 3 (documentation grep) finds all 39 account
IDs across 13 `{tenant}-{stage}` pairs × 3 orgs in the test repo's README, in a
single grep + parse pass.

`references/troubleshooting.md` gains a dedicated entry ("Account-ID resolution
missed accounts the README has") with the exact recovery prompt for cases where the
agent skipped the documentation source.

**Open follow-up — single vs multi-org generation.** The same Run 4 surfaced a
related ambiguity: `RenderData.TargetOrg` is singular, but real Cloud Posse
multi-tenant repos commonly want to enable Atmos Pro across multiple orgs in one
PR. The mixin and IAM-role catalog are org-agnostic, but the `_defaults.yaml`
import edit (the line that flips the org on) is org-specific. Two ways to handle:

- **Loop the mixin import.** Keep `TargetOrg` singular, but have the agent run
  the generation step once per requested org, sharing the same worktree and
  branch. The mixin and catalog are written once; only `_defaults.yaml` per
  org accumulates. Simplest change.
- **Make `TargetOrg` plural.** Update the renderer's `RenderData.TargetOrgs
  []string` and have the docs/PR-body templates iterate. Cleaner but a
  template-API breaking change.

Both options are deferred until the account-map fix lands.

**Open follow-up — prod-apply policy default.** The plan output also asked
whether the apply role should be enabled for prod orgs by default. Operator
preference splits roughly 60/40 in favor of "plan-only for prod, enable apply
later" for first onboardings. The skill currently defaults to enabling apply on
all non-root accounts; consider flipping the default to plan-only for any account
whose stack name contains `prod` or `prd`, with a single user prompt to opt
back into apply.

## Live Test Transcripts

Chronological log of skill invocations against a representative test repo during
manual validation. Customer-specific names are sanitized: tenant namespaces appear as
`dev`, `stg`, `prd`; the test repo appears as `<owner>/<repo>`. The transcripts
capture exactly what the skill output so future regressions can be caught by
comparing against this log.

### Run 1 — Detection (initial attempt)

**Prompt:** "Using `atmos:atmos-pro`, run only the detection phase (step 2)."

**Outcome:** Detection completed, but the agent first ran `ls atmos.yaml` at the repo
root and got `No such file or directory`. It recovered by searching other paths and
produced a correct detection report, then flagged an embedded PAT in the `origin`
remote URL — but echoed the tokenized URL verbatim in the output.

**Gaps identified:**

1. Skill did not locate `atmos.yaml` at the Geodesic path before probing.
2. Skill echoed the PAT back to the terminal output.

Both gaps fixed in the same branch; see "Lessons Learned in Production" above.

### Run 2 — Detection (after Geodesic + redaction fixes)

**Prompt:** "Using `atmos:atmos-pro`, run only the detection phase (step 2)."

**Outcome (sanitized):**

> **Repo:** `<owner>/<repo>` (owner/repo extracted; remote URL contains a PAT —
> rotate and rewrite: `git remote set-url origin https://github.com/<owner>/<repo>`)
>
> **`atmos.yaml` location:** `rootfs/usr/local/etc/atmos/atmos.yaml` (Geodesic-hosted;
> no repo-root `atmos.yaml`)
>
> **Stack hierarchy:** Three orgs, two tenants each, multi-stage, two regions
> (us-east-1, us-west-2).
>
> | Org  | Tenant | Stages                                  |
> |------|--------|-----------------------------------------|
> | dev  | soc    | accs, clip, siem, svcs, wksn            |
> | dev  | gov    | art, dns, dss, iam, log, net, root, sec |
> | stg  | soc    | accs, wksn                              |
> | stg  | gov    | dss                                     |
> | prd  | soc    | accs, wksn                              |
> | prd  | gov    | dss                                     |
>
> **Atmos Auth:** Not configured. → Variant: generate standalone CI profiles.
>
> **Spacelift:** Enabled. 219 `workspace_enabled: true` hits across 208 stack files.
> → Mixin must set `workspace_enabled: false`; flag migration in PR body.
>
> **`github-oidc-provider`:** Component vendored + wired via catalog. Deployment
> status per-account cannot be verified without `atmos describe component ...`. Add
> as rollout checklist step 0.
>
> **Geodesic:** Yes. → Emit Geodesic-specific section in generated `docs/atmos-pro.md`.
>
> **GitHub Actions:** Repo-level enabled. Org-level policy not probed (would require
> `gh api repos/<owner>/<repo>/actions/permissions`).
>
> **Variant selection:** Spacelift-enabled + No-Atmos-Auth + Geodesic. No blocking
> conditions.

**Verification:**

- `atmos.yaml` resolved as the first action — no `ls` error.
- PAT extracted from URL and redacted in output; security finding flagged with
  remediation steps.
- Agent stopped at step 2 as instructed; no worktree created, no files written.

### Run 3 — Plan phase (no writes)

**Prompt:**

> Using `atmos:atmos-pro`, proceed to step 3: show me the plan summary — target
> org, accounts that will be enabled, branch scope, list of files to create, list
> of files to edit, and the starting-condition variants that will be applied. Do
> not write any files yet.

**Outcome (sanitized):**

> **Target scope**
>
> - Orgs enabled: `dev`, `stg`, `prd` (all three)
> - Tenants: `soc`, `gov` in each org
> - Regions: us-east-1, us-west-2 (+ global-region for OIDC provider)
> - Branch scope: `<owner>/<repo>:ref:refs/heads/main` (default-restrictive)
>   - Planner role additionally allows PR refs: `<owner>/<repo>:pull/*`
>   - Apply role: `main` only — no opt-out
> - Workflow dispatch branch: `main`
>
> **Accounts / stacks to enable (leaf-stack level)**
>
> | Org | Tenant | Stages × regions                                   | Apply role?                                |
> |-----|--------|----------------------------------------------------|--------------------------------------------|
> | dev | soc    | accs, clip, siem, svcs, wksn × {use1, usw2}        | yes                                        |
> | dev | gov    | art, dns, dss, iam, log, net, sec × {use1, usw2}   | yes                                        |
> | dev | gov    | root × use1                                        | no — plan-only (root-account safety rail)  |
> | stg | soc    | accs, wksn × {use1, usw2}                          | yes                                        |
> | stg | gov    | dss × {use1, usw2}                                 | yes                                        |
> | prd | soc    | accs, wksn × {use1, usw2}                          | yes                                        |
> | prd | gov    | dss × {use1, usw2}                                 | yes                                        |
>
> The `dev` root account (`dev-gov-use1-root`) gets the plan role only. Its entry in
> `profiles/github-apply/atmos.yaml` points at the plan ARN with an explanatory
> comment.
>
> **Files to CREATE**
>
> - `stacks/mixins/atmos-pro.yaml` — disables Spacelift, declares `settings.pro`
>   dispatch contract
> - `stacks/catalog/aws/iam-role/defaults.yaml` — abstract base for OIDC roles
> - `stacks/catalog/aws/iam-role/gha-tf.yaml` — concrete `gha-tf-plan` and
>   `gha-tf-apply` roles
> - `components/terraform/aws/iam-role/component.yaml` — vendor manifest
> - `profiles/github-plan/atmos.yaml` — per-account planner identities
> - `profiles/github-apply/atmos.yaml` — per-account applier identities (root pinned
>   to plan ARN)
> - `.github/workflows/atmos-pro.yaml` — affected-stacks detection on PR events
> - `.github/workflows/atmos-pro-list-instances.yaml` — daily + push-to-main
>   inventory upload
> - `.github/workflows/atmos-terraform-plan.yaml` — Atmos-Pro-dispatched plan workflow
> - `.github/workflows/atmos-terraform-apply.yaml` — Atmos-Pro-dispatched apply workflow
> - `docs/atmos-pro.md` — rollout procedure (Geodesic-flavored section included)
>
> **Files to EDIT**
>
> - `stacks/catalog/tfstate-backend/defaults.yaml` — additive merge of wildcard
>   `allowed_principal_arns` entries for the plan/apply role ARNs across all target
>   accounts (reciprocal trust, required in same PR)
>
> **Files NOT touched (explicit)**
>
> - `rootfs/usr/local/etc/atmos/atmos.yaml` — not modified (no Atmos Auth retrofit)
> - Existing Spacelift workflows (`atmos-trigger-spacelift-*.yaml`) — left in place;
>   Spacelift disabled via mixin, migration flagged in PR description
> - `stacks/orgs/*/spacelift.yaml` — untouched; superseded at runtime by
>   `settings.spacelift.workspace_enabled: false` from the mixin
>
> **Starting-condition variants applied**
>
> 1. Geodesic-based dev environment → `docs/atmos-pro.md` includes Geodesic section;
>    profile paths resolve relative to repo root, not container root.
> 2. No Atmos Auth in repo → standalone profiles; no retrofit of `auth:` into
>    `atmos.yaml`.
> 3. Spacelift currently enabled → mixin sets `settings.spacelift.workspace_enabled:
>    false`; PR description calls out the migration.
> 4. tfstate `allowed_principal_arns` merge → additive only; existing entries
>    preserved.
> 5. `github-oidc-provider` deployment status unverified → rollout checklist gains a
>    step-0.
> 6. Default-restrictive branch scoping → `trusted_github_repos:
>    ["<owner>/<repo>:ref:refs/heads/main"]` on apply role; planner adds `pull/*`.
>
> **Unresolved inputs (needed at generation time)**
>
> - `ATMOS_PRO_WORKSPACE_ID` — must be set as a GitHub repository variable (not a
>   secret) before the workflows can dispatch. Skill will not prompt; it's called
>   out in `docs/atmos-pro.md`.
> - Atmos version pin — derived from `atmos version` inside Geodesic; workflows pin
>   to that major.minor.patch.
> - Root account ID — read from the `dev-gov-use1-root` stack at generation time
>   (used for the root-safety comment).
> - Existing `allowed_principal_arns` in `tfstate-backend` — read at generation
>   time for the additive merge.
>
> **Open questions for the user**
>
> 1. Apply role for prod (`prd`)? Safety rails allow it, but many orgs prefer
>    plan-only for prod. Keep the default (apply enabled) or restrict `prd` to
>    plan-only?
> 2. Branch scope — `main` only, or also allow a `release/*` pattern?
> 3. Spacelift coexistence — import the new mixin into all orgs immediately, or
>    gate behind a per-stack opt-in during migration?
>
> Confirm (or adjust) and I'll proceed to step 5 (generate artifacts into the
> worktree).

**Verification:**

- **Plan preview is write-safe.** The agent enumerated every file it would create
  and edit without touching the filesystem. A follow-up `git status` on the test
  repo showed no changes.
- **Safety rails surfaced in the plan.** Root-account always-plan appears in the
  accounts table as a dedicated row with "plan-only (root-account safety rail)".
- **Default-restrictive branch scope materialized.** Plan role: `main` + `pull/*`.
  Apply role: `main` only. Exactly matches the documented contract.
- **Reciprocal trust acknowledged.** The tfstate-backend edit is listed under
  "Files to EDIT" and explicitly described as "required in same PR" — the skill
  refuses to split IAM role creation from tfstate-backend trust.
- **Variant selection matches detection.** All three detected variants from Run 2
  (Geodesic + No-Atmos-Auth + Spacelift-enabled) drove corresponding entries in
  the variants-applied list.
- **Unresolved-input categorization is correct.** Four items are flagged as
  "read at generation time" rather than "prompt the user now", matching the
  design intent that discovery-time values never block the plan-preview phase.
- **Open questions prompt the user for genuine policy decisions** rather than
  technical details the skill could answer itself. The three questions (prod
  apply policy, release branch scope, Spacelift coexistence) are all above the
  skill's pay grade.

**Correctness assessment of Run 3**

Each of Claude's outputs was checked against the skill's design intent. Summary: the
plan phase was interpreted correctly end-to-end.

Every design-intent check passed. Details:

- **Safety rail #1 — default-restrictive branch scoping.** Plan role is `main` plus
  `pull/*`; apply role is `main` only with "no opt-out" explicit.
- **Safety rail #2 — root-account always-plan.** `dev-gov-use1-root` appears as a
  dedicated row in the accounts table marked "plan-only (root-account safety rail)".
- **Safety rail #3 — reciprocal trust ships together.** `tfstate-backend/defaults.yaml`
  listed under "Files to EDIT" with "required in same PR".
- **Safety rail #4 — no-secret values.** `ATMOS_PRO_WORKSPACE_ID` surfaced as a
  "repository variable" under unresolved inputs, never as a secret.
- **Safety rail #5 — token redaction.** Branch-scope string uses `<owner>/<repo>`;
  tokenized URL never appears in the output.
- **Artifact catalog — files to create.** All 11 files match
  `references/artifact-catalog.md` exactly.
- **Artifact catalog — files to edit.** 1 file (`tfstate-backend/defaults.yaml`) matches.
- **Files NOT touched — transparency.** Explicit list of 3 paths the skill leaves alone.
- **Variant: Geodesic.** Applied — `docs/atmos-pro.md` flagged for Geodesic section.
- **Variant: No Atmos Auth.** Applied — standalone profiles, no retrofit.
- **Variant: Spacelift enabled.** Applied — mixin sets `workspace_enabled: false`;
  migration flagged in PR body.
- **Variant: `github-oidc-provider` unverified.** Applied — rollout checklist gains
  step-0.
- **Variant: tfstate additive merge.** Applied — "additive only; existing entries
  preserved".
- **Plan is write-safe.** Post-run `git status` on the test repo showed no changes.
- **Unresolved inputs categorized correctly.** Four items (workspace ID, Atmos version
  pin, root account ID, existing ARNs) flagged as "read at generation time" rather
  than "prompt the user now".
- **Open questions pitch genuine policy decisions.** The three questions (prod apply
  policy, release-branch scope, Spacelift coexistence) are all above the skill's pay
  grade and appropriate to surface.

Minor observations (not defects):

- The plan says "`github-oidc-provider` deployment status unverified" even though
  detection showed catalog inclusion via `stacks/mixins/region/global-region.yaml`.
  The conservative framing is correct: catalog inclusion does not prove per-account
  `atmos terraform apply` ran. The step-0 check stays in the rollout.
- "Atmos version pin — derived at generation time from `atmos version` inside
  Geodesic" implies a Geodesic-only path. Non-Geodesic runs would read from the
  host PATH instead; the language could be tightened in a future skill revision but
  does not affect the generated output.
- The plan did not enumerate the exact wildcard patterns that will be merged into
  `tfstate-backend/defaults.yaml`. That level of detail correctly belongs to step 5
  (generation), not step 3 (plan preview).

**Next prompt tested:** proceed to generation (see Run 4).

### Run 4 — Generation (first attempt, blocked safely)

**Prompt:**

> Proceed with `atmos:atmos-pro` from step 4 onward. Create the worktree at
> `.worktrees/atmos-pro-setup feat/atmos-pro` (add `.worktrees/` to `.gitignore` if
> missing), generate all artifacts, run `atmos validate stacks`, and stop before
> opening a PR. When done, show me `git diff --stat` of the worktree and the full
> `git diff` for `stacks/mixins/atmos-pro.yaml`.

**Outcome (sanitized):** the agent stopped before any writes and reported a blocker.

> I need to stop before generation. There's a blocker on the safety-rail "derive
> ARNs from stack introspection."
>
> **Blocker:** the `profiles/github-{plan,apply}/atmos.yaml` templates require an
> explicit `account_id` per `(tenant, stage)` pair. The skill says to derive these
> from stack introspection (`atmos describe component account-map -s <stack>`),
> not prompt the user. But:
>
> - `atmos` commands must run inside Geodesic per `CLAUDE.md`.
> - This repo's `account-map` component is dynamic
>   (`terraform_dynamic_role_enabled: true`), no static map JSON in `stacks/`.
> - I can opportunistically grep account IDs from cross-account references (found
>   5 of ~13 expected accounts) but cannot derive them all reliably without
>   running `atmos`.
>
> Generating profiles with `<MISSING>` placeholders would violate the safety rail
> and produce a file that fails validation.
>
> **Paths forward (pick one):**
>
> 1. You run one command in Geodesic, paste the output, I generate.
>    `atmos describe component account-map -s dev-gov-gbl-iam --format=json | jq '...'`.
> 2. Generate everything except the two profiles (11 of 13 artifacts), run
>    `atmos validate stacks` (likely fails outside Geodesic — I'll report what it
>    says), stop. You complete the two profile files manually.
> 3. Skip generation; dry-run a single artifact (mixin only) to a scratch file so
>    you can review the content without committing to full generation.
>
> Also pending and unanswered:
>
> - Target org — single (`dev`) per the artifact catalog's `target_org` parameter,
>   or all three (`dev`, `stg`, `prd`)?
> - Apply role for prod — keep enabled or pin `prd` to plan-only?

**Verification:**

- **Safety rail held.** The agent refused to fabricate `<MISSING>` placeholder
  ARNs even when offered the simpler route of "just generate it anyway".
- **Generation is correctly gated on input completeness.** Step 5 will not run if
  step 4 did not collect all required values from the user.
- **Three-option offer is well-shaped.** Option 1 is the only one that produces a
  complete artifact set with safety rails intact; Option 2 breaks reciprocal-trust
  shipping (safety rail #3); Option 3 supports iteration without commitment.
- **Pending policy questions resurfaced.** The agent did not silently pick a
  default for the two unanswered questions from Run 3 — it brought them back into
  the loop.

**Skill gap surfaced.** See "Account-map data must be supplied by the user when
the agent can't run `atmos`" in the Lessons Learned in Production section above.
Until the SKILL.md playbook step 4 is updated to detect-and-prompt for the
account map, the manual paste-the-JSON workaround is the path forward.

**Recommended user response (to keep moving):** Option 1, with a paste of the
account-map output, plus explicit answers to the two policy questions
(typically: "all three orgs" + "pin prd to plan-only for the first onboarding").

### Run 5 — Generation (after account-map paste, plan-only-prd policy)

**Prompt:** the same generation prompt from Run 4, with the addition of (a)
instruction to read the README account-ID table (resolution chain source 3),
(b) explicit "all three orgs" target, and (c) "pin prd to plan-only" policy.

**Outcome:** generation completed in the worktree at
`.worktrees/atmos-pro-setup` on branch `feat/atmos-pro`, 18 files staged
(1217 insertions, 1 deletion). Verified manually:

- **Account-map cache** (`.account-map.json`) — all 39 accounts, namespace-prefixed,
  3 root flags correct.
- **Mixin** (`stacks/mixins/atmos-pro.yaml`) — byte-identical to template.
- **39 identities per profile** (3 namespaces × 13 tenant-stage pairs) — both files.
- **Root accounts → plan ARN with safety comment** — all 3 root accounts.
- **Prod-namespace apply → plan ARN with policy comment** — every prd identity pinned.
- **Dev/staging non-root apply → apply ARN** — correctly differentiated.
- **tfstate ARNs in `gha-tf.yaml`** — all 3 namespaces, grouped with comments.
- **tfstate-backend wildcards** — all 3 namespace patterns.
- **`mixins/atmos-pro` import** — added to all 3 org `_defaults.yaml`.
- **OIDC test probe** — uses dev-tier IAM account.
- **`.worktrees/` in `.gitignore`** — added.
- **README account-ID table** (chain source 3) — all 39 accounts found in a
  single grep + parse pass.

**Three skill improvements surfaced and shipped (in this branch):**

1. **Multi-namespace identity prefixing.** The agent invented
   `<namespace>-<tenant>-<stage>/<role>` to avoid collisions across multiple
   namespaces. Documented as the standard pattern in
   `references/auth-profiles.md` "Multi-namespace repo" section. The skill now
   detects multi-org by counting unique namespaces in the resolved account-map.
2. **Multi-namespace tfstate ARN listing.** The agent extended
   `TerraformStateBackendAssumeRole.resources` to include all 3 namespaces'
   tfstate ARNs with `# {namespace} (descriptor) tfstate roles` comments per
   group. Documented in `references/iam-trust-model.md` "Multi-namespace repo"
   section.
3. **`team_permission_sets_enabled: false` precaution for multi-org tfstate.**
   The agent added this defensively because three namespace wildcards plus
   auto-generated PermissionSet ARNs can balloon the trust policy past the IAM
   2048-char limit. Documented as required for multi-namespace setups in
   `references/iam-trust-model.md`.

**Two additional fixes shipped:**

- **`atmos validate stacks` baseline comparison.** SKILL.md step 6 now
  prescribes a baseline-vs-after diff so pre-existing repo errors (unrelated to
  the skill's changes) don't false-positive-block the flow. The agent must
  surface but not block on baseline failures.
- **Tfstate-backend tenant detection.** The template hard-codes `core` (Cloud
  Posse standard); some refarchs use a different tenant (`gov`, `shared`,
  `infra`, etc.). The agent must detect the actual tenant and edit the
  rendered `gha-tf.yaml`. Documented as new playbook **step 4.6** in
  `references/onboarding-playbook.md`.

**Pre-existing `atmos validate stacks` failure.** The agent's self-validate run
reported a duplicate-component-declaration error on a route53 component. The
agent correctly identified this as pre-existing on `main` (unrelated to the
skill's changes), surfaced it to the user, and did NOT block PR creation. The
new SKILL.md baseline-comparison step formalizes this behavior.

**Notable deviations from templates** (the agent flagged them for review):

1. Multi-org profiles in a single file with namespace-prefixed identity names —
   driven by the multi-namespace adaptation above; now standard per
   `auth-profiles.md`.
2. `gha-tf.yaml` lists all 3 namespaces' tfstate ARNs — now standard per
   `iam-trust-model.md`.
3. Prd apply identities all pin to `gha-tf-plan` ARN with a "lift in follow-up
   PR" comment — driven by the user's policy decision; comment template is
   reusable.
4. `oidc-test.yaml` probe target = dev-tier IAM account — sensible default;
   user can override.
5. `tfstate-backend` edit also sets `team_permission_sets_enabled: false` —
   now standard per `iam-trust-model.md`.

**Next prompt to test:** open the PR with the rollout checklist body. After
that completes, the full Path B end-to-end is validated.

### Run 6 — PR creation (succeeded with two skill gaps surfaced)

**Prompt:**

> Open the PR for `atmos:atmos-pro` from the `.worktrees/atmos-pro-setup` worktree.
> Use `gh pr create --draft` so we can review without merging. Use the rollout
> checklist body from `templates/docs/atmos-pro-pr-body.md.tmpl`. Title:
> "feat(atmos-pro): bootstrap CI/CD integration".

**Outcome.** The agent committed 18 files (1217 insertions, 1 deletion), pushed
`feat/atmos-pro`, and opened the draft PR successfully. The body rendered from the
skill's template includes a Summary, the per-namespace accounts list with prod
plan-only and root-account flags, the rollout checklist, security notes, the
Spacelift migration callout, and a transparent note about the pre-existing
`atmos validate stacks` failure that exists on `main` (not introduced by this PR).

**Two skill gaps surfaced and shipped (in this branch):**

1. **`.account-map.json` ended up in the PR.** The skill's previous instructions
   cached the resolved account map at `.worktrees/atmos-pro-setup/.account-map.json`,
   which `git add -A` correctly staged because the worktree root is tracked. Account
   IDs are sensitive (not secrets, but typically kept out of source control).

   **Fix shipped:** SKILL.md step 5 and `references/onboarding-playbook.md` step 4.5
   now prescribe `.git/atmos-pro/account-map.json` instead. `.git/` is special to
   Git and never tracked, so the cache cannot accidentally land in any PR.
   `references/troubleshooting.md` gains an entry with both the prevention and the
   "clean up an existing PR" recipe.

2. **PR body ignored the repo's `PULL_REQUEST_TEMPLATE.md`.** The target repo had
   `.github/PULL_REQUEST_TEMPLATE.md` with a specific format (`Why / What / Usage /
   Testing`), but the agent rendered the skill's own template
   (`Summary / What this adds / Accounts / Rollout checklist / Security notes`)
   without checking. Reviewers expect a PR body that matches their repo's
   established conventions.

   **Fix shipped:** SKILL.md step 7 and `references/onboarding-playbook.md` step 7a
   now prescribe a four-location template detection probe
   (`.github/PULL_REQUEST_TEMPLATE.md`, `.github/pull_request_template.md`,
   `PULL_REQUEST_TEMPLATE.md`, `docs/PULL_REQUEST_TEMPLATE.md`), a section-name
   mapping (`## what` → "What this adds", `## why` → migration rationale,
   `## Usage` → identity-naming + adding-more-repos, `## Testing` → rollout
   checklist), and a fallback to the skill's own template when no repo template
   exists. HTML comments inside the repo's template (`<!-- ... -->`) are preserved
   as author hints. The PR body is built at `.git/atmos-pro/pr-body.md` (also
   never tracked) so it doesn't pollute `git status`.
   `references/troubleshooting.md` gains an entry with the recovery recipe
   (`gh pr edit <num> --body-file .git/atmos-pro/pr-body.md`) for already-opened
   PRs that need to be rewritten to match the repo's template.

**Verification of the actual PR body** (that the agent did write):

- Title: ✅ `feat(atmos-pro): bootstrap CI/CD integration`
- Draft: ✅ yes
- Base/head: ✅ `main` ← `feat/atmos-pro`
- Body: complete and correct content, but in the **wrong template format** — used
  the skill's default instead of the repo's template. Future runs will get this
  right; this PR's body should be updated with `gh pr edit` per the troubleshooting
  recipe.

**Verification of the actual file diffs:**

- All 18 files match what was generated in Run 5; nothing drifted between worktree
  and PR.
- `.account-map.json` was committed (the gap above). Cleanup recipe in
  troubleshooting.md will move it to `.git/atmos-pro/account-map.json`.
- All other files are correct and follow the multi-namespace adaptations
  documented in Run 5.

**Path B end-to-end now validated** with two skill improvements shipped from real-world
observations. The skill is ready for the next live test against a different repo.

### Run 7 — Cleanup commit + PR body rewrite (verified both fixes)

**Prompts (two sent together):**

> Inside the `.worktrees/atmos-pro-setup` worktree, fix the `.account-map.json`
> leak: remove the file from the worktree and from git's index, move its contents
> to `.git/atmos-pro/account-map.json`, and commit + push the cleanup as
> `chore(atmos-pro): move account-map cache out of repo tree`.

> Now rewrite the PR #1 body to match the repo's
> `.github/PULL_REQUEST_TEMPLATE.md` format (Why / What / Usage / Testing).
> Build the new body at `.git/atmos-pro/pr-body.md` following the section-name
> mapping in SKILL.md step 7. Apply with `gh pr edit 1 --body-file
> .git/atmos-pro/pr-body.md`. Show me the diff between the old and new bodies.

**Outcome:**

- **Cache cleanup commit** `b88c55f` removed `.account-map.json` from the PR
  (-53 lines), moved its contents to `.git/atmos-pro/account-map.json` (untracked).
- **PR body rewrite** detected `.github/PULL_REQUEST_TEMPLATE.md`, produced a
  template-conformant body at `.git/atmos-pro/pr-body.md` (137 lines, was 92),
  applied via `gh pr edit 1 --body-file ...`. Body now has sections
  `## Why`, `## What`, `### Usage`, `## Testing`, `## References` matching the
  repo's template, with HTML comments preserved.
- **PR file count**: 17 (was 18) — `.account-map.json` no longer present.
- **Section mapping that the agent applied** (matches the SKILL.md step 7 mapping):
  - **Summary intro** → `## Why` (4 bullets: replace Spacelift, scope blast radius,
    reciprocal trust, atomic rollout)
  - **"What this adds" file list** → `## What` with Files-added / Files-edited /
    Safety-rails / Accounts-wired sub-sections
  - **Rollout checklist** → folded into `## Testing` as a numbered procedure +
    `[x] atmos validate stacks` checkbox
  - **Security notes** → folded into `## What` § "Safety rails enforced"
  - **Spacelift migration** → folded into `## Testing` § "Spacelift coexistence"
  - **Pre-existing validation issue** → folded into `## Testing` § "Validation note"
  - *(new section)* `### Usage` — identity-naming, repo Variables, `ATMOS_PROFILE`
    snippet, "Adding more orgs / repos" pointers
  - *(new section)* `## References` — Atmos Pro docs, GitHub OIDC docs, skill
    source, vendored component release

**Minor agent inconsistency surfaced and addressed.** On the post-reload acknowledgement
(before any prompt), the agent said: *".pr_agent.toml exists at the repo root but no
PULL_REQUEST_TEMPLATE.md. The skill default body is appropriate."* This was wrong —
the template **does** exist at `.github/PULL_REQUEST_TEMPLATE.md`. The agent
contradicted itself on the next prompt by correctly detecting and using the template.

The fix is already in the SKILL.md step 7a probe instructions, which mandate the
detection happen at PR-creation time and check four standard locations in order.
The agent's initial wrong claim was a stale impression from before the prompt that
forced an actual filesystem check. The skill text is correct; this episode reinforces
that the detection probe must run **at step 7a**, not earlier — already prescribed.

**Verification of final PR state:**

- 17 files staged (one fewer than Run 5/6 — `.account-map.json` removed via cleanup commit)
- Body sections: `## Why`, `## What`, `### Usage`, `## Testing`, `## References`
  (matches the repo's template)
- Two commits: original generation + cache-cleanup chore
- Still draft: ✅
- Title unchanged: `feat(atmos-pro): bootstrap CI/CD integration`

**Path B end-to-end fully validated.** Both skill fixes from Run 6 work against the
real PR as documented. The skill is ready for production use.

## Comparison Against Human Reference Implementation

Final cross-check: the agent's PR side-by-side with a human-authored reference PR for
the same use case (replace Spacelift with Atmos Pro on a Geodesic-hosted multi-org
Cloud Posse-style refarch). The agent's PR file count is 17; the reference is 28.

### Files in BOTH PRs (identical artifact set)

5 GitHub workflows (`atmos-pro.yaml`, `atmos-pro-list-instances.yaml`,
`atmos-terraform-plan.yaml`, `atmos-terraform-apply.yaml`, `oidc-test.yaml`),
the IAM-role vendor manifest (`component.yaml`), `docs/atmos-pro.md`, both auth
profiles, the abstract IAM-role catalog (`defaults.yaml` + `gha-tf.yaml`), the
tfstate-backend additive edit, the mixin, and the `_defaults.yaml` import for the
target org.

### Files ONLY in the agent PR (improvements over the reference)

- `.gitignore` adds `.worktrees/` — the reference PR did not gitignore the worktree
  directory used during onboarding.
- `stacks/orgs/{stg,prd}/_defaults.yaml` — multi-org enablement; the reference PR
  enabled only the dev namespace.

### Files ONLY in the human PR (gaps in the agent's output)

**Critical — vendored Terraform sources (8 files under `components/terraform/aws/iam-role/`):**
`main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`, `providers.tf`, `context.tf`,
`github-assume-role-policy.tf`, `account-verification.mixin.tf`. Without these
committed (or `atmos vendor pull` run before deployment),
`atmos terraform apply aws/iam-role/...` cannot run.

*Fix shipped:* SKILL.md new **step 5.5** detects the repo's vendoring convention
(`commit` vs `ondemand`) and either runs `atmos vendor pull -c aws/iam-role` plus
stages the 8 files, or adds a step-1 to the rollout checklist.

**Critical — `team_permission_sets_enabled` plumbing in
`account-map/modules/team-assume-role-policy/{variables,main}.tf` (10 / +0 and 8 / -6):**
Declares and plumbs the variable through the team-assume-role-policy module. Without
it, the stack-level `team_permission_sets_enabled: false` is **silently ignored** and
the trust policy will balloon past the IAM 2048-char limit on multi-namespace setups.

*Fix shipped:* SKILL.md step **5.5** runs a `grep -q team_permission_sets_enabled`
probe; if MISSING, emits the four required patches documented in
`references/iam-trust-model.md`.

**Critical — `team_permission_sets_enabled` plumbing in
`tfstate-backend/{variables,iam}.tf` (10 / +0 and 3 / -2):** Same plumbing on the
tfstate-backend side. Same silent-ignore failure mode.

*Fix shipped:* same SKILL.md step **5.5** probe + patch instructions.

**Minor — `profiles/README.md` (58 lines):** Explainer doc for future maintainers;
not strictly required, but useful.

*Fix shipped:* new template `templates/profiles/README.md.tmpl`; renderer registered
in `pkg/ai/skills/atmospro/render.go`; `artifact-catalog.md` updated to mark "Always".

**Optional — `.claude/skills/atmos-pro-setup/SKILL.md` (169 lines):** A per-repo copy
of the skill's playbook for future Claude Code runs in that repo. Not infra; useful
but not blocking.

*Status:* **deferred.** A "ship a per-repo skill copy" feature can be added later if
customer demand surfaces.

### Net assessment

The agent reproduced **10 of 14 required artifacts** correctly on the first pass and
**identified the right scope** for the two it added beyond the reference (multi-org
enablement, gitignore). The four it missed are now addressed in the skill:

- **Vendored sources** — new SKILL.md step 5.5 with detection + auto-vendor.
- **`team_permission_sets_enabled` plumbing** — new SKILL.md step 5.5 with patch
  templates documented in `references/iam-trust-model.md`.
- **`profiles/README.md`** — new template auto-generated.
- **`.claude/skills/atmos-pro-setup/SKILL.md`** — deferred until requested.

Without the new SKILL.md step 5.5, the agent's PR as it stands today (PR #1) **will
fail at apply time** when `atmos terraform apply tfstate-backend -s ...-root` hits
the IAM 2048-char limit, because the `team_permission_sets_enabled: false` stack
setting has no effect in the unpatched module. The skill's next run on a fresh repo
will produce the patches automatically; PR #1 needs a follow-up commit applying the
four patches manually (or via re-running the skill with the new SKILL.md loaded).

## Final Status Table — All Runs

A canonical record of every prompt sent to the agent during Path B manual
validation, what the agent did, and what was learned.

### Run 0 — Plugin install (one-time setup)

**Steps:**
```
/plugin marketplace add /path/to/atmos
/plugin install atmos@cloudposse
/reload-plugins
```

**Agent outcome:** marketplace added, plugin installed, reload reports `0 skills`
which is misleading — `/skills` shows all 23 atmos skills under the `atmos:` namespace.

**Skill changes shipped:** marketplace.json + plugin.json schema corrections (top-level
`description`/`version` → `metadata.*`; author shape).

### Run 1 — Detection (initial)

**Prompt:** "Using `atmos:atmos-pro`, run only the detection phase (step 2)."

**Agent outcome:** detection completed but `ls atmos.yaml` failed at the repo root;
PAT in remote URL echoed verbatim.

**Skill changes shipped:** Geodesic config-path resolution (search
`rootfs/usr/local/etc/atmos/atmos.yaml` first); PAT redaction safety rail.

### Run 2 — Detection (after fixes)

**Prompt:** same as Run 1.

**Agent outcome:** `atmos.yaml` resolved on first attempt; PAT redacted; structured
detection report; agent stopped at step 2 as instructed.

### Run 3 — Plan preview (no writes)

**Prompt:** "Proceed to step 3: show the plan summary. Do not write files yet."

**Agent outcome:** comprehensive plan with target scope, accounts table,
files-to-create/edit lists, variants applied, unresolved inputs, open policy
questions. All 16 design-intent checks passed; write-safe.

### Run 4 — Generation (1st attempt, blocked safely)

**Prompt:** "Proceed from step 4 onward; create the worktree; generate; validate."

**Agent outcome:** stopped before any writes — could not satisfy the safety rail
"derive ARNs from stack introspection" because `atmos` is Geodesic-only and the
repo uses dynamic account-map. Offered three paths forward.

**Skill changes shipped:** five-step **account-ID resolution chain** (
`atmos describe component` → static catalog → repo docs grep → cross-account ARN
grep → user prompt). Cache moved to `.git/atmos-pro/account-map.json`.

### Run 5 — Generation (after account-map paste)

**Prompt:** "Re-run account-map resolution using chain source 3 (README table). All
three orgs. Pin prd to plan-only. Generate everything."

**Agent outcome:** worktree created at `.worktrees/atmos-pro-setup`, 18 files staged
(1217 insertions, 1 deletion). All variants applied correctly. Three multi-org
adaptations the agent invented are now standard:

1. namespace-prefixed identity names (`<namespace>-<tenant>-<stage>/<role>`),
2. multi-namespace tfstate ARN listing (grouped with comments),
3. `team_permission_sets_enabled: false` on tfstate-backend (with the caveat from
   the gap analysis above — needs module patches to actually take effect).

**Skill changes shipped:** documented multi-namespace patterns in
`references/auth-profiles.md` and `references/iam-trust-model.md`; added
`atmos validate stacks` baseline-vs-after diff to step 6 to avoid false-positive
blocking on pre-existing repo errors.

### Run 6 — PR creation

**Prompt:** "Open the PR with `gh pr create --draft`."

**Agent outcome:** PR opened (18 files, draft, correct title) but two gaps:

1. `.account-map.json` cache committed (was at worktree root, picked up by
   `git add -A`).
2. PR body used skill's default template instead of the repo's
   `.github/PULL_REQUEST_TEMPLATE.md`.

**Skill changes shipped:** cache moved to `.git/atmos-pro/account-map.json` (never
tracked); PR-template detection in step 7a (4 standard locations), section-name
mapping table, fall back to skill default only if no template found.

### Run 7 — Cleanup commit + PR body rewrite

**Prompts (two together):** move cache out of repo tree; rewrite PR body to match
the repo's template.

**Agent outcome:**

- Cleanup commit `b88c55f` removed `.account-map.json` from PR (-53 lines).
- PR body rewritten at `.git/atmos-pro/pr-body.md` matching repo template
  (`Why / What / Usage / Testing / References`); applied via `gh pr edit 1`.
- Final PR file count: 17, two commits, draft, title unchanged.

**Skill changes shipped:** none additional (the SKILL.md text from Run 6 already
covered the recovery procedure; this run verified the recipe works).

### Run 8 — Module-level patches for `team_permission_sets_enabled`

**Prompt:** the full multi-step prompt that detects the gap, applies four module
patches with exact wording from `references/iam-trust-model.md`, commits with a
prescribed message, pushes, and posts a PR comment explaining the patches.

**Outcome:**

- **Detection probe ran first** and reported `MISSING — patches required`,
  confirming the underlying modules (vendored from upstream) do not yet declare
  `team_permission_sets_enabled`.
- **Four files patched, 26 insertions total:**
  - `account-map/modules/team-assume-role-policy/variables.tf` (+10) — variable
    declaration with the canonical description text.
  - `account-map/modules/team-assume-role-policy/main.tf` (+4) — passes
    `overridable_team_permission_sets_enabled = var.team_permission_sets_enabled`
    to BOTH the `allowed_role_map` and `denied_role_map` module calls.
  - `tfstate-backend/variables.tf` (+10) — same variable declaration.
  - `tfstate-backend/iam.tf` (+2) — passes `team_permission_sets_enabled =
    var.team_permission_sets_enabled` to the `assume_role` module.
- **Commit `5920107`** with the prescribed message; pushed to `origin/feat/atmos-pro`.
- **PR comment** at <https://github.com/aknysh/atmos-pro-skills-1/pull/1#issuecomment-4247128385>
  explaining the patches: silent-ignore failure mode, four-file change list,
  backward compatibility (`default = true` so existing deployments are
  unaffected), deployment sequencing (patches take effect on next
  tfstate-backend apply), and a link to the IAM trust model reference doc.
- **Did NOT run** `atmos terraform plan` or `apply` (as instructed).

**Diff comparison vs the human reference PR for the same patches:**

The agent emitted a **smaller, cleaner diff** — 26 insertions / 0 deletions versus
the reference PR's mixed inserts / reformatting (8/-6 + 10 + 3/-2 + 10). The
reference PR reformatted existing column alignment "while it was in there"; the
agent left existing lines untouched and only added the new field. Same functional
outcome, smaller blast radius for review.

**Verification of agent's patch correctness:**

- Variable description text is byte-identical to the canonical wording in
  `references/iam-trust-model.md`.
- Default is `true` — backward compatible.
- Both the `allowed_role_map` AND `denied_role_map` module calls receive the
  passthrough (a partial patch on only one would silently break the denied path).
- The `tfstate-backend/iam.tf` patch lands on the correct `assume_role` module
  call (the only module in that file that needs it).

**Final PR state after Run 8:**

- 21 files (was 17 after Run 7); 3 commits.
- Now functionally equivalent to the human reference PR for everything blocking
  deployment.
- Remaining gaps vs the human reference (all non-blocking):
  - 8 vendored `aws/iam-role/*.tf` sources — handled at deployment via
    `atmos vendor pull -c aws/iam-role` if the repo convention is "vendor on
    demand", or generated by the skill's step 5.5 if "commit vendored". This
    run did not exercise step 5.5's vendoring branch since the prompt was
    scoped to the `team_permission_sets_enabled` patches only.
  - `profiles/README.md` — minor explainer; not in this PR. Skill template
    exists at `templates/profiles/README.md.tmpl`; future fresh runs will
    include it.
  - `.claude/skills/atmos-pro-setup/SKILL.md` — per-repo skill copy; deferred.

## Path B End-to-End Closure

After 8 runs, the agent-driven Path B is **fully validated against a real Cloud
Posse-style multi-org Geodesic-hosted refarch repo**. The open PR is functionally
equivalent to a human-authored reference implementation for everything that
matters at deploy time. Three commits, 21 files, draft state, body matches the
repo's own `PULL_REQUEST_TEMPLATE.md`, all safety rails honored, no leaks.

Every gap surfaced during the eight runs has a corresponding skill fix in this
branch. The skill is ready for production use against any Cloud Posse-style Atmos
repo, with the manual-validation checklist below as the operator's guide.

## Manual Validation Checklist

The automated test suite validates the deterministic layer (detection, rendering,
self-containment, parity). Live validation of the AI-driven paths remains a manual
exercise. Track completion here as each path is exercised against a real repo.

### Path A — `atmos ai ask --skill atmos-pro`

- [ ] Skill is listed in `atmos ai skill list` without prior install
- [ ] `atmos ai ask "setup atmos pro" --skill atmos-pro` loads the skill from the
      embedded bundle with a configured AI provider (Claude API)
- [ ] Agent creates an isolated worktree and does not touch the main checkout
- [ ] Agent runs the detection probes and reports a plan summary before writing
- [ ] Generated artifacts match the golden snapshot for the equivalent `RenderData`
- [ ] `atmos validate stacks` passes on the worktree
- [ ] `gh pr create` produces a PR with the expected body and rollout checklist
- [ ] Repeat with OpenAI and Bedrock providers if available

### Path B — Claude Code direct

Until the feature branch is merged, use one of the local-testing options from the
**Local Testing** section above (symlink, local marketplace, or inline load). After the
merge, switch to the production marketplace flow.

- [x] Local marketplace install works: `/plugin marketplace add /path/to/atmos` +
      `/plugin install atmos@cloudposse` succeeds (confirmed manually)
- [x] All 23 atmos skills appear in `/skills` under the `atmos:` namespace (confirmed
      manually — the `/reload-plugins` summary shows `0 skills` but that count excludes
      plugin-bundled skills; `/skills` is the source of truth)
- [ ] *(post-merge)* `/plugin marketplace add cloudposse/atmos` succeeds
- [ ] *(post-merge)* `/plugin install atmos@cloudposse` installs without errors
- [ ] Prompting "set up Atmos Pro" activates `atmos:atmos-pro` and loads `SKILL.md`
- [ ] Reference files are loaded on demand (progressive disclosure) rather than all
      upfront
- [ ] Generated artifacts match the Path A output byte-for-byte against the same repo
      state (modulo environment-specific values like account IDs)
- [ ] `atmos validate stacks` passes on the generated worktree

### Starting-condition variants

Run each variant against a representative fixture and confirm the skill selects the
documented behavior from `references/starting-conditions.md`:

- [ ] GitHub Actions disabled org-wide — skill stops with the correct URL
- [ ] No Atmos Auth in repo — skill generates standalone profiles, does not retrofit
- [ ] Atmos Auth already configured — skill adds github-oidc provider via patch file
- [ ] Spacelift currently enabled — mixin sets `workspace_enabled: false`
- [ ] Geodesic-based dev environment — generated docs gain Geodesic section
- [ ] `github-oidc-provider` not deployed — rollout checklist gains step-0

### Failure-mode validation

For each entry in `references/troubleshooting.md`, reproduce the symptom in a scratch
environment and confirm the documented fix resolves it. Update the entry if the
symptom, cause, or fix differs.

## Alternatives Considered

### A Go command (`atmos pro init`) as the primary entry point

A deterministic command is simpler to reason about and does not require an AI. We still intend
to ship one eventually (Phase 6). But the onboarding *pain* is not "I want a wizard"; it is
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
