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
   git worktree add .worktrees/atmos-pro-setup feat/atmos-pro
   cd .worktrees/atmos-pro-setup
   ```
   `.worktrees/` is the default because it is tool-agnostic and common in open-source
   Git workflows. If the repo already uses a different convention (e.g., `.conductor/`
   for Conductor-managed worktrees, or a `../`-sibling layout), match it. Always work
   in a branch named `feat/atmos-pro` (or `feat/atmos-pro-<org>` for follow-up
   invocations).

   Make sure `.worktrees/` is in the repo's `.gitignore` (or `.git/info/exclude`) so
   the worktree directory is never accidentally committed. Add it if missing.

2. **Detect the repo shape.** See `references/starting-conditions.md` for the full probe
   catalog. At minimum, answer:

   - **Locate `atmos.yaml` first.** Many Cloud Posse-style repos are Geodesic-hosted and
     keep `atmos.yaml` at `rootfs/usr/local/etc/atmos/atmos.yaml`, not at the repo root.
     Before any probe that reads the config, resolve its actual path:
     ```bash
     for p in rootfs/usr/local/etc/atmos/atmos.yaml atmos.yaml atmos.yml .atmos.yaml; do
       [ -f "$p" ] && { export ATMOS_CONFIG_FILE="$p"; break; }
     done
     echo "atmos.yaml at: ${ATMOS_CONFIG_FILE:-<NOT FOUND>}"
     ```
     Use `ATMOS_CONFIG_FILE` for every subsequent read. If Atmos needs to resolve it,
     set `ATMOS_CLI_CONFIG_PATH` to the directory containing it.
   - **Stack hierarchy** — run `atmos list stacks` and `atmos describe stacks --format=json |
     jq 'keys'` to enumerate tenants × stages × regions.
   - **Atmos Auth present?** — look for `auth:` at the top level of `$ATMOS_CONFIG_FILE`
     and in imported files under `atmos.d/` (both repo-root and under
     `rootfs/usr/local/etc/atmos/atmos.d/` for Geodesic repos).
   - **Spacelift present?** — grep for `settings.spacelift.workspace_enabled: true` in stacks.
   - **`github-oidc-provider` deployed?** — `atmos describe component github-oidc-provider -s <one-account-stack>`
     must resolve and the account's Terraform state must have `oidc_provider_arn`.
   - **tfstate-backend** — find the component and read its `allowed_principal_arns`.
   - **GitHub Actions enabled on the repo?** — `gh api repos/:owner/:repo/actions/permissions`.
   - **Running inside Geodesic?** — look for `Dockerfile` (with `cloudposse/geodesic`),
     `geodesic.mk`, `.envrc` referencing `geodesic`, or a `rootfs/` overlay directory.
     Presence of `rootfs/usr/local/etc/atmos/` alone is a strong Geodesic signal.

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
   - `{{ .org }}`, `{{ .repo }}` — from `git remote get-url origin` (with token redaction)
   - `{{ .accounts }}` — from the **account-ID resolution chain** below
   - `{{ .root_account_id }}` — from the root stack, for the root-account safety rail
   - `{{ .tfstate_role_arns }}` — from the tfstate-backend component
   - `{{ .atmos_version }}` — from `atmos version` (major.minor.patch)

   **Account-ID resolution chain.** Account IDs must come from a deterministic source —
   never fabricate `<MISSING>` placeholders. Try sources in order, stopping at the first
   that produces a complete map:

   1. **`atmos describe component account-map`** (deterministic, structured). Run inside
      the user's Geodesic shell if available:
      ```bash
      atmos describe component account-map -s <one-account-stack> --format=json \
        | jq '.outputs.account_info_map // .vars.account_map'
      ```
      If `atmos` is not on PATH (most Path-B Claude Code runs), skip to step 2.
   2. **Static account-map files** in `stacks/catalog/account-map/` or
      `stacks/catalog/account-map.yaml` — grep for top-level `account_map:` keys with
      12-digit values.
   3. **Repo documentation tables.** Many Cloud Posse-style repos document account IDs
      in the README, often as a table with `{tenant}-{stage}` row labels and per-org
      column headers. Search:
      ```bash
      grep -lE '\| *[0-9]{12} *\|' README.md docs/**/*.md _shared/**/*.md 2>/dev/null
      ```
      Parse any matching tables: row label → `{tenant}-{stage}`; numeric cells per
      column header → `{org}` account IDs.
   4. **Cross-account ARN references in stacks** — grep `stacks/` for
      `arn:aws:iam::<12-digit>:role/...` patterns to recover individual account IDs.
      This is partial coverage; only use as supplementary data, not as the primary
      source.
   5. **User prompt** — if the chain produces an incomplete map, stop and ask the user
      to paste the missing entries. Do NOT proceed with placeholders.

   Always report which source(s) were used and the confidence level. Cache the resolved
   map at `.git/atmos-pro/account-map.json` (inside `.git/` is never tracked by Git, so
   the cache file can never accidentally end up in the PR). Do **not** write the cache
   under the worktree root — `git add -A` will pick it up.

   Re-runs and follow-up invocations read this cache before re-running the resolution
   chain.

   The artifact list and why each exists is in `references/artifact-catalog.md`. The templates
   directory mirrors that catalog.

5.5. **Vendor pull the IAM role component sources.** The skill writes only the
   vendor manifest (`components/terraform/aws/iam-role/component.yaml`); the actual
   Terraform sources are pulled from upstream. Some Cloud Posse refarch repos
   commit vendored sources for deterministic deploys; others vendor on-demand at
   apply time. Detect the convention:

   ```shell
   # If the existing iam-role/ component (or any other vendored component)
   # has its .tf sources committed, the repo's convention is "commit vendored".
   ls components/terraform/iam-role/*.tf 2>/dev/null && CONV=commit || CONV=ondemand
   ```

   - If the repo commits vendored sources: run `atmos vendor pull -c aws/iam-role`
     and stage the resulting `.tf` files (8 files: `main.tf`, `variables.tf`,
     `outputs.tf`, `versions.tf`, `providers.tf`, `context.tf`,
     `github-assume-role-policy.tf`, `account-verification.mixin.tf`).
   - If the repo vendors on-demand: do nothing here, but add a step-1 to the
     rollout checklist: "Run `atmos vendor pull -c aws/iam-role` before deploying
     IAM roles".

   Either way, also detect whether the underlying `tfstate-backend` and
   `account-map/modules/team-assume-role-policy` modules support the
   `team_permission_sets_enabled` variable (the stack-level setting in step 5
   is silently ignored if they don't):

   ```shell
   grep -q "team_permission_sets_enabled" \
     components/terraform/tfstate-backend/variables.tf \
     components/terraform/account-map/modules/team-assume-role-policy/variables.tf \
     2>/dev/null && echo "OK" || echo "MISSING — patch required"
   ```

   If MISSING, emit the four patches documented in
   [`references/iam-trust-model.md`](references/iam-trust-model.md) under
   `team_permission_sets_enabled` requires module-level support.

6. **Self-validate.** Inside the worktree:
   ```shell
   atmos validate stacks
   atmos describe component aws/iam-role/gha-tf-plan -s <one-stack>
   atmos describe component aws/iam-role/gha-tf-apply -s <one-stack>
   ```

   **Always baseline against `main` before flagging failures.** Some Atmos repos have
   pre-existing `atmos validate stacks` errors (duplicate component declarations,
   abandoned imports, etc.) that have nothing to do with the skill's changes. To
   avoid false-positive blocking, capture the baseline first:

   ```shell
   # In a temporary checkout of main (or with `git stash`)
   atmos validate stacks > /tmp/atmos-pro-baseline.txt 2>&1 || true
   # Then in the worktree
   atmos validate stacks > /tmp/atmos-pro-after.txt 2>&1 || true
   diff /tmp/atmos-pro-baseline.txt /tmp/atmos-pro-after.txt
   ```

   If the diff is empty (or only adds lines about the new components), the skill's
   changes did not introduce new validation errors — proceed. If the diff shows
   *new* errors that mention paths the skill wrote (`stacks/mixins/atmos-pro.yaml`,
   `stacks/catalog/aws/iam-role/*`, `profiles/github-*`, etc.), stop and report.

   Pre-existing failures must be surfaced to the user (so they know the repo has
   them) but must not block PR creation.

   `atmos` may not be on PATH if the run is outside Geodesic. Check first:
   ```shell
   command -v atmos && atmos version
   ```
   If not found, skip the self-validation and document the gap in the PR body
   ("Skill could not run `atmos validate stacks` — Geodesic-only binary not on
   host PATH. Please run `atmos validate stacks` inside Geodesic before merging.").

7. **Open a PR (optional).** If `gh` is authenticated, follow the repo's PR template
   convention:

   **a) Detect the repo's PR template** (case-insensitive, both common locations):

   ```shell
   for p in .github/PULL_REQUEST_TEMPLATE.md .github/pull_request_template.md \
            PULL_REQUEST_TEMPLATE.md docs/PULL_REQUEST_TEMPLATE.md; do
     [ -f "$p" ] && { TEMPLATE="$p"; break; }
   done
   echo "PR template: ${TEMPLATE:-<none — using skill default>}"
   ```

   **b) If the repo has a template,** populate **its** sections with rollout content
   instead of the skill's default body. Common section names map as follows:

   | Template section          | Skill content to put in it                                   |
   |---------------------------|--------------------------------------------------------------|
   | `## what` / `## What`     | "What this adds" list from the artifact catalog              |
   | `## why` / `## Why`       | One paragraph: enable Atmos Pro CI/CD, replace Spacelift, etc. |
   | `## references`           | Link to https://atmos-pro.com and the skill source           |
   | `## Usage`                | The "Identity Naming" or "Adding more repos" mini-section    |
   | `## Testing` / Test plan  | The rollout checklist (the same numbered steps from `docs/atmos-pro.md`) |

   Preserve any HTML comments (`<!-- ... -->`) the template uses as section
   instructions — leave them in place; the rendered PR keeps them as hidden author
   guidance.

   **c) If no template exists,** fall back to `templates/docs/atmos-pro-pr-body.md.tmpl`
   from the skill (the "Summary / What this adds / Accounts / Rollout checklist /
   Security notes" format).

   Then create the PR:

   ```shell
   gh pr create --draft \
     --title "feat(atmos-pro): bootstrap CI/CD integration" \
     --body-file .git/atmos-pro/pr-body.md \
     --base main --head feat/atmos-pro
   ```

   Use `.git/atmos-pro/pr-body.md` (inside `.git/`, untracked) for the body file so
   it does not appear in `git status` or get accidentally committed.

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
5. **Redact tokens from remote URLs.** `git remote get-url origin` on a repo cloned with
   `gh` often returns a URL of the form `https://ghp_xxx@github.com/owner/repo`. Never
   display that URL verbatim in plan summaries, PR bodies, worktree paths, or log output.
   Extract `owner/repo` only. Flag the leaked token to the user as a security finding
   (rotate the PAT, rewrite the remote with `git remote set-url origin https://github.com/owner/repo`)
   but do not block the flow — the user can rotate the token in parallel.

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
