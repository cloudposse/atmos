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
git worktree add .conductor/atmos-pro-setup feat/atmos-pro
cd .conductor/atmos-pro-setup
```

If `.conductor/` doesn't exist and the repo uses a different convention (`.worktrees/`, sibling
directory), use that. The worktree isolates generated changes from the user's main checkout.

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

  Target org: e98d
  Accounts to enable (13):
    gov: root, iam, dss, art, dns, log, net, sec
    soc: accs, clip, siem, svcs, wksn

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
    stacks/orgs/e98d/_defaults.yaml  (add "mixins/atmos-pro" to imports)

  Detected conditions:
    - Spacelift enabled  => will set workspace_enabled: false
    - No Atmos Auth      => generating standalone profiles
    - Geodesic detected  => adding Geodesic section to docs/atmos-pro.md
    - OIDC provider deployed to all probed accounts  OK

Proceed? [y/N]
```

In `--approve-all` mode, skip the prompt and proceed.

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

```shell
git add -A
git commit -m "feat(atmos-pro): bootstrap CI/CD integration

Generated by the atmos-pro skill."

git push -u origin feat/atmos-pro

gh pr create --title "feat(atmos-pro): bootstrap CI/CD integration" --body-file .github/atmos-pro-pr-body.md
```

The PR body is derived from `templates/docs/atmos-pro-pr-body.md.tmpl` and includes the
rollout checklist.

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
