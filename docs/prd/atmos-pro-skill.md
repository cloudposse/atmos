# PRD: Atmos Pro Onboarding Skill

## Status

Shipped. Validated end-to-end against a multi-org, Geodesic-hosted Atmos refarch using
Claude Code (Path B) and via `atmos ai ask --skill atmos-pro` (Path A). Both paths
produce byte-identical output.

## Problem

Onboarding an existing Atmos repo to Atmos Pro requires assembling a non-trivial set
of artifacts across every target account: GitHub OIDC trust, an IAM role catalog,
two auth profiles, a `settings.pro` mixin, four GitHub Actions workflows, and
reciprocal tfstate-backend trust. A single working configuration spans roughly 1,900
added lines across ~28 files.

Starting conditions vary widely and each variant changes what needs to be generated:

- Atmos Auth may or may not be configured.
- Spacelift may be active and need disabling.
- GitHub Actions may be disabled org-wide.
- Federated IAM may come from Identity Center or an external IdP (Okta).
- The repo may run inside Geodesic (non-standard `atmos.yaml` location).
- `github-oidc-provider` may or may not be deployed.
- tfstate-backend `allowed_principal_arns` may have existing entries to preserve.

Before this work, onboarding was a multi-day, hand-assembled exercise. Even experienced
operators could not, by inspection, identify which `_defaults.yaml` to edit or which
accounts needed the IAM role.

## Goal

Ship an agent skill that encodes the full onboarding contract — detection, generation,
validation, PR creation — so any conforming AI tool (Claude Code, Codex, Gemini, Grok)
can take an Atmos repo from zero to a deploy-ready PR, with all safety rails honored,
in a handful of prompts.

### Non-Goals

- **Not a deterministic code generator.** The skill guides an AI agent; a pure-Go
  `atmos pro init` command is possible future work.
- **Not an IAM-role provisioner.** The skill writes Terraform and YAML; the user
  still runs `atmos terraform apply` to create roles in AWS.
- **Not an Atmos Auth adopter for local dev.** If a repo has no `auth:` config, the
  skill generates standalone CI profiles; it does not retrofit Atmos Auth onto
  developer workflows.
- **Not cloud-agnostic.** v1 targets AWS + GitHub Actions. Azure/GCP/GitLab are
  future work.
- **Not a Spacelift migrator.** The skill can disable Spacelift but does not move
  state or reproduce Spacelift policy graphs.

## Solution

A single skill, `atmos-pro`, invocable two ways:

- **Path A — Atmos-dispatched:** `atmos ai ask "setup atmos pro" --skill atmos-pro`.
  Atmos loads the skill from its embedded bundle and runs it through the AI provider
  configured in `atmos.yaml`. Self-contained; no other AI CLI required.
- **Path B — Claude Code direct:** the `atmos` plugin is installed from the Cloud
  Posse marketplace; Claude Code loads `SKILL.md` and executes using its native
  Read/Write/Edit/Bash tools.

Both paths share the same templates and produce byte-identical output (enforced by
cross-path parity tests).

### Invocation styles (Claude Code)

The skill supports two invocation styles under Path B:

- **Step-by-step** — one prompt per phase (`detect` → `plan` → `generate` → `open PR`).
  Lets the operator inspect detection and the generated plan before any writes.
- **One-shot** — a single prompt asks the agent to run the full flow end-to-end,
  pausing only at the plan-approval step and for any open policy questions. Example:

  > Using `atmos:atmos-pro`, run the full onboarding end-to-end: detect the repo
  > shape, show me the plan for approval, then generate everything in a worktree,
  > validate, and open a draft PR using the repo's `PULL_REQUEST_TEMPLATE.md`.
  > Pause only at the plan-approval step and for any policy questions.

Both styles execute the same six-step flow; safety rails (explicit plan approval,
policy-question prompts) remain intact in the one-shot style.

### The flow the skill prescribes

1. **Create an isolated worktree** (`.worktrees/atmos-pro-setup`) so the main checkout
   is untouched.
2. **Detect the repo shape** — stack hierarchy, Atmos Auth, Spacelift, Geodesic,
   `github-oidc-provider`, tfstate backend trust, GitHub Actions org policy.
3. **Confirm the plan** — print target scope, accounts table, files to create/edit,
   variants applied, open policy questions. The user approves before any writes.
4. **Generate artifacts** from templates, filled in from detected values.
5. **Self-validate** — `atmos validate stacks` + `atmos describe component` on the
   generated config. Stops on failure.
6. **Open a PR** with a rollout checklist body, using the repo's
   `PULL_REQUEST_TEMPLATE.md` if present.

### Safety rails (non-negotiable defaults)

- **Default-restrictive branch scoping.** `trusted_github_repos` defaults to
  `main`-only. Planner role explicitly opts out (needs PR branches); apply role
  never does.
- **Root account always-plan.** The apply profile pins the root-account identity to
  the plan role's ARN. Automation never applies to root.
- **Reciprocal trust ships together.** The IAM role changes and the tfstate-backend
  edit appear in the same PR. The skill refuses to generate one without the other.
- **No secrets in manifests.** `ATMOS_PRO_WORKSPACE_ID` is a repo variable, not a
  secret.
- **Token redaction.** Never echo PATs from `git remote get-url`; extract owner/repo
  only.

## Implementation

### Skill layout

```text
agent-skills/skills/atmos-pro/
  SKILL.md                          # top-level decision tree
  references/
    onboarding-playbook.md          # step-by-step playbook
    starting-conditions.md          # repo-shape detection and variant selection
    artifact-catalog.md             # every file the skill generates, with why/when
    settings-pro-contract.md        # the settings.pro dispatch contract in full
    iam-trust-model.md              # OIDC sub-claim scoping, reciprocal tfstate
    auth-profiles.md                # identity naming, deep-merge, no-Atmos-Auth path
    geodesic-integration.md         # running the skill inside Geodesic
    troubleshooting.md              # failure-mode catalog
  templates/
    mixins/atmos-pro.yaml.tmpl
    catalog/iam-role-{defaults,gha-tf}.yaml.tmpl
    component/iam-role-component.yaml.tmpl
    profiles/github-{plan,apply}.yaml.tmpl
    profiles/README.md.tmpl
    workflows/{atmos-pro,atmos-pro-list-instances,atmos-terraform-plan,atmos-terraform-apply,oidc-test}.yaml.tmpl
    docs/atmos-pro.md.tmpl
    docs/atmos-pro-pr-body.md.tmpl
    tfstate-backend-edit.yaml.tmpl  # additive merge fragment
```

### Go packages

Deterministic template rendering, testable in isolation, shared by both paths:

- **`pkg/atmospro/detect`** — filesystem-based pre-flight probes (Atmos Auth,
  Spacelift, Geodesic). Pure Go, `fs.FS`-based, no external binaries.
- **`pkg/ai/skills/atmospro`** — template renderer with a typed `RenderData`
  context and custom `<<...>>` delimiters so `{{ }}` and `${{ }}` in the output
  pass through untouched (Atmos/GHA/vendor-manifest literals).
- **`pkg/ai/skills/embedded`** — exposes every built-in skill to the registry so
  Path A can load the skill without a filesystem read.

### Testing

- **Unit**: template rendering tests for each generated file, covering variant
  paths (no Atmos Auth, Spacelift present/absent, root account present/absent).
- **Golden snapshots**: `tests/fixtures/scenarios/atmos-pro-setup/golden/`
  captures byte-stable rendering output; refactors must regenerate intentionally.
- **Cross-path parity**: tests prove the on-disk templates (Path B) and the
  embedded templates (Path A) are byte-identical.

### Starting-condition branches

The skill explicitly handles every variant observed in real onboardings:

| Variant                                   | Skill behavior                                                           |
|-------------------------------------------|--------------------------------------------------------------------------|
| GitHub Actions disabled org-wide          | Stop and print the exact org-settings URL                                |
| No Atmos Auth                             | Generate standalone CI profiles; no retrofit into `atmos.yaml`           |
| Atmos Auth already configured             | Add `github-oidc` provider via a patch file; preserve existing config    |
| Federated IAM via external IdP            | CI-side profiles only; document that local dev continues with the IdP   |
| Geodesic dev environment                  | Search `rootfs/usr/local/etc/atmos/atmos.yaml`; emit Geodesic doc section |
| Spacelift enabled                         | Set `workspace_enabled: false`; flag migration in PR body                |
| `github-oidc-provider` not deployed       | Add a step-0 to the rollout checklist                                    |
| Custom tfstate `allowed_principal_arns`   | Merge additively — never overwrite                                       |
| Multi-namespace repo                      | Prefix identity names with `<namespace>-`; list tfstate ARNs per ns     |
| Missing `team_permission_sets_enabled`    | Emit module-level patches for `account-map` and `tfstate-backend`        |

## Results

Validated end-to-end against a multi-org, Geodesic-hosted Atmos refarch via
Claude Code (Path B) and confirmed identical behavior under Path A.

- **Prompts to deploy-ready PR:** 4 (detect → plan → generate → open PR), plus a
  one-time plugin install.
- **Artifacts produced:** complete artifact set — IAM role catalog, both auth
  profiles, five GitHub workflows, the mixin, the additive tfstate-backend edit,
  the vendor manifest, and the generated `docs/atmos-pro.md`.
- **Safety rails honored:** root-account always-plan, default-restrictive branch
  scoping, reciprocal tfstate trust, no committed secrets, PAT redaction.
- **PR body:** automatically mapped to the repo's `PULL_REQUEST_TEMPLATE.md`
  sections when present.
- **Multi-org / multi-namespace:** agent auto-applied namespace-prefixed identity
  names and multi-namespace tfstate ARN listing; both patterns are now documented
  as standard.
- **Cross-path parity:** Path A and Path B produce byte-identical output, enforced
  by test.

Further comparison against a hand-authored reference implementation for the same
use case confirmed the skill's output is functionally equivalent; the handful of
additional module-level patches required for certain refarchs
(`team_permission_sets_enabled` plumbing) are now emitted automatically via a
detection probe.

## Alternatives Considered

- **A Go command (`atmos pro init`) as the primary entry point.** Deterministic but
  brittle — cannot adapt to repo-specific variants without encoding every shape in
  code. The skill approach lets the agent reason about novel variants and degrade
  gracefully. A Go command remains viable as a future complement for users who want
  reproducibility over flexibility.
- **A template repo.** Users would copy and fill in. Fails on the hardest part —
  identifying which stacks, accounts, and files to touch.
- **A Terraform module.** Provisions IAM but doesn't emit Atmos stack config, auth
  profiles, or workflows. Half the problem.
- **Docs-only.** The failed onboarding sessions that motivated this work followed
  the docs; the docs are not the bottleneck.

## References

- Skill source: `agent-skills/skills/atmos-pro/`
- Renderer: `pkg/ai/skills/atmospro/`
- Detection probes: `pkg/atmospro/detect/`
- Golden snapshots: `tests/fixtures/scenarios/atmos-pro-setup/golden/`
- Atmos AI: `/ai`
- Agent Skills open standard: https://agentskills.io
- Atmos Pro: https://atmos-pro.com
