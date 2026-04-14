# Atmos Pro Onboarding Playbook

This is the step-by-step the skill follows. It matches what a human operator would do, made
precise enough for an AI agent to execute without interpretation.

## Prerequisites (caller must confirm)

- The user has created an Atmos Pro workspace at <https://atmos-pro.com/start> and has the
  `ATMOS_PRO_WORKSPACE_ID` to hand.
- The user has set `ATMOS_VERSION` and `ATMOS_PRO_WORKSPACE_ID` as **repository variables**
  (not secrets) in GitHub Actions settings.
- GitHub Actions is enabled for the repository. Verify via
  `gh api repos/:owner/:repo/actions/permissions`.
- The user has `gh` CLI authenticated with permission to push branches and open PRs.

If any of these are missing, stop and report exactly what's needed.

## Step 1: Isolated worktree

```shell
git worktree add .worktrees/atmos-pro-setup feat/atmos-pro
cd .worktrees/atmos-pro-setup
```

`.worktrees/` is the default (tool-agnostic, common in open-source Git workflows). If the repo
already uses a different convention — `.conductor/` for Conductor-managed worktrees, a sibling
`../atmos-pro-setup` layout, or something else — match that instead. Ensure the chosen path is
in `.gitignore` (or `.git/info/exclude`) so it never gets committed; add the entry if missing.

## Step 2: Repo detection

Run each probe. Record results in a JSON object the agent keeps in memory for Step 4.

```shell
# Stack hierarchy
atmos list stacks --format=json

# Atmos Auth presence
grep -r "^auth:" atmos.yaml atmos.d/ 2>/dev/null

# Spacelift usage
grep -r "spacelift.*workspace_enabled" stacks/ | grep -v "false"

# OIDC provider deployment (probe one stack)
STACK=$(atmos list stacks | head -1)
atmos describe component github-oidc-provider -s "$STACK" --format=json 2>&1

# tfstate-backend state
atmos describe component tfstate-backend -s "$STACK" --format=json 2>&1 | jq '.vars.access_roles'

# GitHub Actions enablement
gh api repos/:owner/:repo/actions/permissions

# Geodesic detection
test -f Dockerfile && grep -l "cloudposse/geodesic" Dockerfile
test -f geodesic.mk && echo "geodesic"
```

## Step 3: Variant selection

Walk `references/starting-conditions.md` and record which variant(s) apply. Multiple can apply
simultaneously (e.g., Spacelift present + Geodesic used).

## Step 4: Plan confirmation

Print a human-readable summary. Example:

```text
Atmos Pro setup plan:

  Target org: dev
  Accounts to enable (13):
    core: root, identity, audit, network, dns, security, artifacts
    plat: dev, staging, prod, sandbox

  Branch scope (trusted_github_repos):
    Plan role: any ref (required for PRs)
    Apply role: main only (default)

  Files to create (11):
    stacks/mixins/atmos-pro.yaml
    stacks/catalog/aws/iam-role/defaults.yaml
    stacks/catalog/aws/iam-role/gha-tf.yaml
    components/terraform/aws/iam-role/component.yaml
    profiles/github-plan/atmos.yaml
    profiles/github-apply/atmos.yaml
    .github/workflows/atmos-pro.yaml
    .github/workflows/atmos-pro-list-instances.yaml
    .github/workflows/atmos-terraform-plan.yaml
    .github/workflows/atmos-terraform-apply.yaml
    docs/atmos-pro.md

  Files to edit (2):
    stacks/catalog/tfstate-backend/defaults.yaml
    stacks/orgs/dev/_defaults.yaml  (add "mixins/atmos-pro" to imports)

  Detected conditions:
    - Spacelift enabled  => will set workspace_enabled: false
    - No Atmos Auth      => generating standalone profiles
    - Geodesic detected  => adding Geodesic section to docs/atmos-pro.md
    - OIDC provider deployed to all probed accounts  OK

Proceed? [y/N]
```

In `--approve-all` mode, skip the prompt and proceed.

## Step 4.5: Resolve the account-ID map

Account IDs must come from a deterministic source — never fabricate `<MISSING>`
placeholders. Try sources in order, stop at the first complete map:

1. **`atmos describe component account-map`** — only works if `atmos` is on PATH (typically
   not the case for Path B Claude Code runs outside Geodesic):
   ```bash
   atmos describe component account-map -s <one-account-stack> --format=json \
     | jq '.outputs.account_info_map // .vars.account_map'
   ```
2. **Static account-map files** in the catalog:
   ```bash
   grep -rE '^\s+[a-z0-9-]+:\s+"?[0-9]{12}"?' stacks/catalog/account-map* 2>/dev/null
   ```
3. **Repo documentation tables** — common Cloud Posse pattern; account IDs documented in
   the repo README or under `docs/`:
   ```bash
   grep -lE '\| *[0-9]{12} *\|' README.md docs/**/*.md _shared/**/*.md 2>/dev/null
   ```
   Parse matched tables: row label = `{tenant}-{stage}`; numeric cells per column header
   = `{org}` account IDs. Markdown section dividers (`**Governance**`, etc.) are skipped
   because they don't contain 12-digit numbers.
4. **Cross-account ARN references in stacks** (supplementary only):
   ```bash
   grep -rEho 'arn:aws:iam::[0-9]{12}' stacks/ | sort -u
   ```
5. **User prompt** — if any account is still missing, stop and ask. Do not generate
   profile files with placeholder ARNs.

Cache the resolved map to **`.git/atmos-pro/account-map.json`** (NOT under the worktree
root). `.git/` is never tracked by Git, so the cache cannot accidentally end up in the
PR. Re-runs and follow-up invocations check this cache before re-running the chain.

If a previous skill run accidentally left `.account-map.json` in the worktree root,
remove it before staging:

```bash
rm -f .account-map.json
git rm --cached .account-map.json 2>/dev/null || true
```

## Step 4.6: Locate the tfstate-backend tenant

The `gha-tf.yaml` template assumes the `tfstate-backend` component lives in the
`core` tenant (Cloud Posse standard convention). If the target repo uses a
different tenant for tfstate (some refarchs use `gov`, `shared`, `infra`, etc.),
the agent must override the literal `core` in the rendered
`stacks/catalog/aws/iam-role/gha-tf.yaml` to match.

Detect the actual tenant by inspecting where the `tfstate-backend` component is
deployed:

```bash
atmos describe component tfstate-backend -s "$(atmos list stacks | head -1)" \
  --format=json | jq -r '.vars.tenant // "core"'
```

Or, when `atmos` is unavailable, grep for the component's stack manifest:

```bash
grep -rEl 'component:\s*tfstate-backend' stacks/ 2>/dev/null \
  | head -1 \
  | xargs dirname \
  | sed -E 's|.*orgs/[^/]+/([^/]+)/.*|\1|'
```

If the detected tenant is anything other than `core`, edit the rendered
`gha-tf.yaml` and replace `-core-gbl-root-tfstate` with
`-{detected-tenant}-gbl-root-tfstate` (and the matching `-tfstate-ro` line).
Group entries by namespace with comments so future readers can tell which is
which (see `references/iam-trust-model.md` "Multi-namespace repo" for the
expected structure).

## Step 5: Generate artifacts

Copy templates from `../templates/` into the worktree. For each template:

1. Read the `.tmpl` file.
2. Substitute placeholders using the detection results.
3. Write to the target path.
4. Stage the file with `git add`.

Placeholders used across templates:

| Placeholder               | Source                                                       |
|---------------------------|--------------------------------------------------------------|
| `{{ .org }}`              | `git remote get-url origin` → parse owner                    |
| `{{ .repo }}`             | `git remote get-url origin` → parse repo name                |
| `{{ .target_org }}`       | User-confirmed org identifier from Step 4                    |
| `{{ .accounts }}`         | List of `{tenant, stage, account_id}` from stack detection   |
| `{{ .root_account_id }}`  | Account ID for the root-account stack                        |
| `{{ .root_account_key }}` | `{tenant}-{stage}` for the root account                      |
| `{{ .tfstate_role_arns }}`| Read/write + read-only tfstate role ARNs from tfstate-backend|
| `{{ .atmos_version }}`    | `atmos version` (major.minor.patch)                          |
| `{{ .branch_scope }}`     | Default `main`; opt-in Environment via Step 4                |

## Step 6: Self-validation

```shell
atmos validate stacks
```

Then, for one representative stack (pick the first `{tenant}-{stage}` from the detected
accounts):

```shell
atmos describe component aws/iam-role/gha-tf-plan -s "$STACK" > /dev/null
atmos describe component aws/iam-role/gha-tf-apply -s "$STACK" > /dev/null
```

Non-zero exit → print the output, stop, do not open a PR.

## Step 7: PR creation

### 7a. Build the PR body

Detect the repo's PR template first (case-insensitive, common locations):

```shell
for p in .github/PULL_REQUEST_TEMPLATE.md .github/pull_request_template.md \
         PULL_REQUEST_TEMPLATE.md docs/PULL_REQUEST_TEMPLATE.md; do
  [ -f "$p" ] && { TEMPLATE_PATH="$p"; break; }
done
```

**If a template exists:** copy its structure into `.git/atmos-pro/pr-body.md` and fill
in the sections with skill content. Map the common section names as follows:

| Template section          | Fill with                                                    |
|---------------------------|--------------------------------------------------------------|
| `## what` / `## What`     | "What this adds" list from the artifact catalog              |
| `## why` / `## Why`       | One paragraph: enable Atmos Pro CI/CD, replace Spacelift     |
| `## references`           | https://atmos-pro.com + skill source URL                     |
| `## Usage`                | Identity Naming and Adding More Repos mini-sections          |
| `## Testing` / Test plan  | The rollout checklist from the skill's default template      |

Preserve `<!-- ... -->` comments — they are author hints intended to stay in place.

**If no template exists:** copy `templates/docs/atmos-pro-pr-body.md.tmpl` rendered
output to `.git/atmos-pro/pr-body.md`.

### 7b. Stage, commit, push

```shell
# Sanity check: never commit the account-map cache
test -f .account-map.json && rm -f .account-map.json && git rm --cached .account-map.json 2>/dev/null
git add -A
git commit -m "feat(atmos-pro): bootstrap CI/CD integration

Generated by the atmos-pro skill."

git push -u origin feat/atmos-pro
```

### 7c. Create the PR

```shell
gh pr create --draft \
  --title "feat(atmos-pro): bootstrap CI/CD integration" \
  --body-file .git/atmos-pro/pr-body.md \
  --base main --head feat/atmos-pro
```

Both the body file and the account-map cache live under `.git/atmos-pro/` — `.git/`
is never tracked by Git, so neither file can leak into the PR.

## Post-PR: rollout checklist (user actions)

The agent does not execute these. They appear in the PR body for the user to work through:

1. Review the generated workflows, mixin, and profiles.
2. Deploy `github-oidc-provider` to all target accounts (if not already deployed).
3. Deploy IAM roles — one `aws/iam-role/gha-tf-plan` per account, one `aws/iam-role/gha-tf-apply`
   per non-root account.
4. Deploy the tfstate-backend trust update (`atmos terraform apply tfstate-backend -s <root-stack>`).
5. Run the `oidc-test.yaml` workflow manually from GitHub Actions to verify OIDC.
6. Merge the PR.
7. Open a trivial test PR to exercise the full Atmos Pro dispatch flow.
