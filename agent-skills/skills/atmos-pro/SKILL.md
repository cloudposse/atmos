---
name: atmos-pro
description: "Atmos Pro onboarding: generate GitHub OIDC IAM roles, auth profiles, CI workflows, mixins, and open a PR that wires an Atmos repo to Atmos Pro"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/onboarding-playbook.md
  - references/starting-conditions.md
  - references/artifact-catalog.md
  - references/settings-pro-contract.md
  - references/iam-trust-model.md
  - references/auth-profiles.md
  - references/geodesic-integration.md
  - references/troubleshooting.md
---

# Atmos Pro Onboarding

Use this skill when a user asks to set up Atmos Pro in an existing Atmos repo, add a new repo to
an existing Atmos Pro workspace, or diagnose an Atmos Pro onboarding failure. The skill encodes
the full contract (IAM trust, auth profiles, `settings.pro` dispatcher, four GitHub workflows,
reciprocal tfstate trust) and prescribes a generation flow that opens a PR.

## When to use this skill

Activate on any of these prompts:

- "set up Atmos Pro"
- "configure Atmos Pro for this repo"
- "enable Atmos Pro for org X"
- "add a new repo to Atmos Pro"
- "why won't Atmos Pro dispatch a plan workflow"
- "Atmos Pro says `AssumeRoleWithWebIdentity` failed"

Do **not** activate for generic GitOps questions (`atmos describe affected` in a non-Atmos-Pro
workflow, Spacelift migration without Atmos Pro) — use `atmos-gitops` or a provider-specific
skill instead.

## Invocation

This skill supports two invocation paths. The generation output must be **byte-identical**
between them — both paths use the same templates and the same safety rails.

### Path A — `atmos ai ask --skill` (Atmos-dispatched)

```shell
atmos ai ask "setup atmos pro" --skill atmos-pro
```

The Atmos binary loads the skill (from its embedded bundle) and sends the system prompt to
the configured AI provider. The provider must execute the flow below using its own tool-use
primitives. Non-interactive mode:

```shell
atmos ai ask "setup atmos pro" --skill atmos-pro --non-interactive --approve-all
```

### Path B — Claude Code direct (loaded from the repo)

The user tells Claude Code to load this skill. Claude Code fetches
`agent-skills/skills/atmos-pro/` (via the plugin marketplace or a direct repo reference) and
executes the flow using its native tools: Read, Write, Bash, Edit, Glob, Grep, and
the TodoWrite plan tracker.

When running under Path B, the agent should:

- Use the **Read** tool to open reference files from `references/` on demand (progressive
  disclosure) — do not load all references upfront unless a specific task requires them.
- Use **Bash** for all `atmos`, `git`, and `gh` commands described in the playbook.
- Use **Write** / **Edit** to place generated files under the worktree. Prefer **Edit** over
  **Write** when the file already exists (e.g., the tfstate-backend merge fragment applied to
  an existing `defaults.yaml`).
- Use **TodoWrite** to track the 7-step flow and report progress to the user.

Output written by Path B must match the golden-snapshot contract at
`tests/fixtures/scenarios/atmos-pro-setup/golden/` when the same fixture inputs are used.
Any deviation is a skill bug, not a Claude-Code-specific variation.

## The flow (strict order)

1. **Create an isolated worktree.** Never touch the user's main checkout.
   ```shell
   git worktree add .conductor/atmos-pro-setup feat/atmos-pro
   cd .conductor/atmos-pro-setup
   ```
   If `.conductor/` is not conventional for the user's repo, use `.worktrees/` or another
   sibling path. Always work in a branch named `feat/atmos-pro` (or `feat/atmos-pro-<org>` for
   follow-up invocations).

2. **Detect the repo shape.** See `references/starting-conditions.md` for the full probe
   catalog. At minimum, answer:

   - **Stack hierarchy** — run `atmos list stacks` and `atmos describe stacks --format=json |
     jq 'keys'` to enumerate tenants × stages × regions.
   - **Atmos Auth present?** — look for `auth:` in `atmos.yaml` or imported files.
   - **Spacelift present?** — grep for `settings.spacelift.workspace_enabled: true` in stacks.
   - **`github-oidc-provider` deployed?** — `atmos describe component github-oidc-provider -s <one-account-stack>`
     must resolve and the account's Terraform state must have `oidc_provider_arn`.
   - **tfstate-backend** — find the component and read its `allowed_principal_arns`.
   - **GitHub Actions enabled on the repo?** — `gh api repos/:owner/:repo/actions/permissions`.
   - **Running inside Geodesic?** — look for `Dockerfile`, `geodesic.mk`, `.envrc` with
     `direnv` + `geodesic`.

3. **Select the starting-condition variant.** Use the decision table in
   `references/starting-conditions.md`. If a blocking variant is detected (e.g., GitHub Actions
   disabled org-wide), stop and report the required user action with the exact URL or command
   to fix it.

4. **Confirm the plan with the user.** Print a summary:
   - Target org(s) and account stacks that will be enabled
   - Branch scope that will be applied (default: `main`)
   - List of files the agent will create or edit
   - Whether Atmos Auth will be retrofitted (default: no — generate standalone profiles)

   Wait for user approval in interactive mode. In `--approve-all` mode, proceed.

5. **Generate artifacts.** Copy files from `templates/` into the worktree, filling in:
   - `{{ .org }}`, `{{ .repo }}` — from `git remote get-url origin`
   - `{{ .accounts }}` — from the detected tenant/stage matrix
   - `{{ .root_account_id }}` — from the root stack, for the root-account safety rail
   - `{{ .tfstate_role_arns }}` — from the tfstate-backend component
   - `{{ .atmos_version }}` — from `atmos version` (major.minor.patch)

   The artifact list and why each exists is in `references/artifact-catalog.md`. The templates
   directory mirrors that catalog.

6. **Self-validate.** Inside the worktree:
   ```shell
   atmos validate stacks
   atmos describe component aws/iam-role/gha-tf-plan -s <one-stack>
   atmos describe component aws/iam-role/gha-tf-apply -s <one-stack>
   ```
   If any fail, print the error and stop. Do not open a PR with a broken configuration.

7. **Open a PR (optional).** If `gh` is authenticated:
   ```shell
   gh pr create --title "feat(atmos-pro): bootstrap CI/CD integration" --body "$(cat <<'EOF'
   ## Summary
   Generated by the `atmos-pro` skill.

   ## Rollout checklist
   - [ ] Review generated workflows and mixin
   - [ ] Deploy `github-oidc-provider` to all target accounts (if not already)
   - [ ] Deploy IAM roles: `atmos terraform apply aws/iam-role/gha-tf-plan -s <stack>` per account
   - [ ] Deploy IAM roles: `atmos terraform apply aws/iam-role/gha-tf-apply -s <stack>` per non-root account
   - [ ] Deploy the tfstate-backend trust update
   - [ ] Run `oidc-test.yaml` workflow manually to verify OIDC
   - [ ] Open a test PR to exercise the Atmos Pro dispatch flow
   EOF
   )"
   ```

## Safety rails (non-negotiable)

These are defaults the skill must enforce. See `references/iam-trust-model.md` for rationale.

1. **Default-restrictive branch scoping** — `trusted_github_repos` defaults to `{org}/{repo}:main`.
   The planner role explicitly opts out (needs PR branches). The apply role never opts out.
2. **Root-account always-plan** — the root account's entry in `profiles/github-apply/atmos.yaml`
   points at the **plan** role's ARN, not the apply role's. A comment in the generated file
   explains why.
3. **Reciprocal trust together** — the IAM role changes and the tfstate-backend edit must appear
   in the same PR. The skill refuses to generate one without the other.
4. **No secret values in the mixin or profiles** — `ATMOS_PRO_WORKSPACE_ID` is a GitHub
   repository variable, not a secret. Do not prompt for it as a secret; tell the user to set it
   as a variable.

## Starting-condition variants (summary)

Full table in `references/starting-conditions.md`. Headline cases:

| Variant                                 | Skill behavior                                                      |
|-----------------------------------------|---------------------------------------------------------------------|
| GitHub Actions disabled org-wide        | Stop; print the org settings URL and required permissions           |
| No Atmos Auth in repo                   | Generate standalone profiles; do not retrofit `auth:` into atmos.yaml |
| Federated IAM via Okta (not Identity Center) | Generate only CI-side profiles; local dev keeps customer's Okta flow  |
| Geodesic-based dev environment          | Emit Geodesic-specific `docs/atmos-pro.md` section                  |
| Spacelift currently enabled             | Set `workspace_enabled: false`; flag migration in PR description    |
| `github-oidc-provider` not deployed     | Add a step-0 to the rollout checklist                               |
| tfstate `allowed_principal_arns` custom | Merge additively; never overwrite                                   |

## What this skill generates

The full artifact set is in `references/artifact-catalog.md`. Summary:

- `stacks/mixins/atmos-pro.yaml` — single import point, disables Spacelift, declares `settings.pro`
- `stacks/catalog/aws/iam-role/{defaults,gha-tf}.yaml` — abstract base + two concrete OIDC roles
- `components/terraform/aws/iam-role/component.yaml` — vendor manifest for the IAM role component
- `profiles/github-plan/atmos.yaml`, `profiles/github-apply/atmos.yaml` — per-account identities
- `.github/workflows/atmos-pro.yaml` — affected-stacks detection on PR events
- `.github/workflows/atmos-pro-list-instances.yaml` — daily + push-to-main full inventory upload
- `.github/workflows/atmos-terraform-plan.yaml` — dispatched by Atmos Pro for plan
- `.github/workflows/atmos-terraform-apply.yaml` — dispatched by Atmos Pro for apply
- Edit to `stacks/catalog/tfstate-backend/defaults.yaml` adding wildcard `allowed_principal_arns`
- `docs/atmos-pro.md` — generated README describing what was produced and the rollout procedure

## Out of scope (do not do)

- Do not run `atmos terraform apply`. The user rolls out manually per the PR checklist.
- Do not retrofit Atmos Auth onto local dev if it was not already present.
- Do not attempt to move Spacelift state or mirror Spacelift stack policies.
- Do not prompt for AWS credentials or IAM role ARNs — derive them from stack introspection.
- Do not generate Azure, GCP, or GitLab variants in this skill.

## Troubleshooting

When the user reports a failure, load `references/troubleshooting.md` and match the symptom
against the catalog. The most common failure modes:

- `AssumeRoleWithWebIdentity` failed → OIDC provider missing, `trusted_github_repos` scope wrong,
  or subject-claim format mismatch
- `NoCredentialProviders` at plan time → `ATMOS_PROFILE` not set, or profile file not discovered
- `AccessDenied` on tfstate → reciprocal trust not deployed on one side
- Atmos Pro never dispatches → workflows not on `main` branch yet, or `ATMOS_PRO_WORKSPACE_ID`
  not set as a repository variable

## References

Loaded on demand by the agent:

- [`references/onboarding-playbook.md`](references/onboarding-playbook.md) — step-by-step playbook
- [`references/starting-conditions.md`](references/starting-conditions.md) — repo-shape detection and variant selection
- [`references/artifact-catalog.md`](references/artifact-catalog.md) — every file the skill generates, with why/when
- [`references/settings-pro-contract.md`](references/settings-pro-contract.md) — the `settings.pro` dispatch contract in full
- [`references/iam-trust-model.md`](references/iam-trust-model.md) — OIDC sub-claim scoping, reciprocal tfstate, root safety
- [`references/auth-profiles.md`](references/auth-profiles.md) — identity naming, deep-merge mechanics, no-Atmos-Auth path
- [`references/geodesic-integration.md`](references/geodesic-integration.md) — running the skill inside Geodesic
- [`references/troubleshooting.md`](references/troubleshooting.md) — failure-mode catalog with symptoms and fixes
