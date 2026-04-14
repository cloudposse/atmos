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
```

Claude Code reads `/path/to/atmos/.claude-plugin/marketplace.json`, resolves the `atmos`
plugin to `./agent-skills` (relative paths resolve from the marketplace root, which is
the directory containing `.claude-plugin/`), and installs every skill under the bundled
skills directory — including `atmos-pro` from the unmerged branch. This mirrors the
production install flow exactly.

If the install fails with "Plugin 'atmos' not found in any marketplace", run the
marketplace validator to surface schema errors:

```
/plugin validate /path/to/atmos
```

Common failure modes:

- **Non-standard fields** — Claude Code's marketplace schema requires `metadata.version`
  and `metadata.description` (not top-level `version` / `description`). The author
  object accepts `name` and optional `email`, not `url`. Both manifests in this repo
  have been corrected.
- **Relative paths require Git-backed marketplace** — adding a marketplace by a direct
  URL to `marketplace.json` skips cloning, so `./agent-skills` never resolves. Use a
  local filesystem path (`/plugin marketplace add /path/to/atmos`) or a Git source.
- **Duplicate installs** — if a previous attempt cached a partial install, remove and
  re-add:
  ```
  /plugin marketplace remove cloudposse
  /plugin marketplace add /path/to/atmos
  /plugin install atmos@cloudposse
  ```

When the feature branch merges, switch to the remote marketplace:

```
/plugin marketplace remove cloudposse
/plugin marketplace add cloudposse/atmos
/plugin install atmos@cloudposse
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

- [ ] Local testing option chosen (symlink / local marketplace / inline) discovers the
      skill inside Claude Code running from the target repo
- [ ] *(post-merge)* `/plugin marketplace add cloudposse/atmos` succeeds
- [ ] *(post-merge)* `/plugin install atmos@cloudposse` installs without errors
- [ ] Claude Code sees `atmos-pro` in the skills list
- [ ] Prompting "set up Atmos Pro" activates the skill and loads `SKILL.md`
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
