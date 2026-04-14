# Troubleshooting

Symptoms observed in real onboardings, mapped to causes and fixes.

## `AssumeRoleWithWebIdentity` failed

### Symptom

```
Error: operation error STS: AssumeRoleWithWebIdentity, https response error StatusCode: 403
Not authorized to perform sts:AssumeRoleWithWebIdentity
```

### Causes

1. **OIDC provider not deployed in the target account.** The GitHub OIDC identity provider
   (`token.actions.githubusercontent.com`) must exist in every account the skill targets.
2. **`trusted_github_repos` does not include the current repo.** Check the role's trust
   policy in AWS Console. The `sub` condition must match the workflow's repo/branch.
3. **Running on a branch the role does not trust.** The apply role defaults to `main`-only.
   A workflow run from a PR branch cannot assume the apply role.

### Fix

```shell
# 1. Verify OIDC provider
atmos describe component github-oidc-provider -s <stack>
atmos terraform plan github-oidc-provider -s <stack>

# 2. Check trust policy
aws iam get-role --role-name e98d-gov-gbl-iam-gha-tf-plan \
  --query 'Role.AssumeRolePolicyDocument.Statement[?Sid==`OidcProviderAssume`].Condition' \
  --profile <operator-profile>

# 3. If running from a PR branch, use the plan role (not apply).
```

## `NoCredentialProviders` at plan time

### Symptom

```
Error: NoCredentialProviders: no valid providers in chain.
```

### Causes

1. `ATMOS_PROFILE` is not set on the workflow step.
2. `ATMOS_PROFILE` is set but the profile file is not discoverable (wrong path, missing file).
3. The identity name the stack looks up does not exist in the profile.

### Fix

```shell
# Inside the workflow, echo the profile and verify it resolves:
echo "ATMOS_PROFILE=${ATMOS_PROFILE}"
ls profiles/${ATMOS_PROFILE}/atmos.yaml

# Show what identity the stack expects:
atmos describe component <component> -s <stack> --format=json | jq '.auth'
```

## `AccessDenied` reading tfstate

### Symptom

```
Error loading state: AccessDenied: User: arn:aws:sts::...:assumed-role/.../...-gha-tf-plan
is not authorized to perform: sts:AssumeRole on resource:
arn:aws:iam::<root>:role/e98d-gov-gbl-root-tfstate
```

### Cause

Reciprocal trust is not deployed on one side. Either:

- The IAM role has the `TerraformStateBackendAssumeRole` policy statement but it references
  the wrong tfstate role ARN, **or**
- The tfstate-backend's `allowed_principal_arns` does not match the new IAM role ARN.

### Fix

```shell
# Redeploy both sides:
atmos terraform apply aws/iam-role/gha-tf-plan -s <stack>
atmos terraform apply tfstate-backend -s <root-stack>
```

## Atmos Pro never dispatches a workflow

### Symptom

PR opens. `atmos-pro.yaml` runs and uploads affected stacks. No plan workflows appear in the
Actions tab.

### Causes

1. **Workflows not on the `main` branch yet.** Atmos Pro only dispatches workflows that exist
   on the repository's default branch. If the bootstrap PR hasn't been merged, no dispatch
   happens.
2. **`ATMOS_PRO_WORKSPACE_ID` not set.** The variable must be set at the repo or org level in
   GitHub Actions settings.
3. **Workspace not connected to the repo.** In the Atmos Pro UI, the repo must be linked to
   the workspace with workflow-dispatch permission granted.

### Fix

1. Merge the bootstrap PR to get workflows on `main`.
2. Open a trivial follow-up PR to exercise the dispatch flow.
3. Verify `ATMOS_PRO_WORKSPACE_ID` in repo settings.
4. Confirm workflow-dispatch permission in the Atmos Pro workspace settings.

## `count=0` or empty affected list

### Symptom

`atmos-pro.yaml` runs successfully but `atmos describe affected --upload` reports zero
affected components.

### Causes

1. The PR doesn't actually modify any stack/component files.
2. `fetch-depth: 0` missing from the checkout step — Atmos can't see the base ref.
3. The PR targets a non-default branch the affected-stacks logic doesn't handle.

### Fix

- Check the checkout step:
  ```yaml
  - uses: actions/checkout@v4
    with:
      ref: ${{ github.event.pull_request.head.sha }}
      fetch-depth: 0
  ```
- Confirm the PR modifies files Atmos considers (stack YAML, component source).

## IAM trust policy exceeds 2048 chars

### Symptom

`atmos terraform apply tfstate-backend` fails with:

```
InvalidParameter: LimitExceeded: Cannot exceed quota for PolicySize: 2048
```

### Cause

`team_permission_sets_enabled: true` (the default) auto-generates SSO PermissionSet ARN
patterns from team names in `allowed_roles`. With many teams, the trust policy balloons.

### Fix

Set `team_permission_sets_enabled: false` in the tfstate-backend stack-level vars and rely on
explicit `allowed_permission_sets` instead. The skill's generated tfstate-backend edit does
this automatically if a size-limit risk is detected; in other cases the user may need to
opt in manually.

## GitHub Environment not found

### Symptom

The apply workflow fails immediately with:

```
Error: Environment 'e98d-gov-iam' could not be found.
```

### Cause

The apply flow passes `github_environment: "{{ .vars.tenant }}-{{ .vars.stage }}"` as an input
and uses it as `environment:` in the apply job. That environment must be created manually in
repo Settings → Environments.

### Fix

Create the named environments in repo settings. The `github_environment` pattern is
`{tenant}-{stage}` per account. This is a one-time setup cost; required reviewers and wait
timers can then be configured per environment.

## Skill-generated files don't match `atmos validate stacks`

### Symptom

The skill completes, `atmos validate stacks` fails inside the worktree.

### Cause

Likely one of:

1. A placeholder was not substituted (e.g., `{{ .root_account_id }}` left as literal text).
2. The detected tenant/stage matrix is wrong — the stack naming convention differs from the
   default.
3. The `tfstate-backend` component path is non-standard and the additive edit pointed at the
   wrong file.

### Fix

Re-run the skill with `--verbose` to see the detected values and template output. If the
issue is naming convention, pass `--stack-pattern` to override the default (future option;
v1 does not support overrides — file an issue).

## Detector says "no auth" but my repo has an auth block

### Symptom

The skill generates standalone profiles when you expected it to retrofit the existing
`auth:` block, even though the repo clearly has one.

### Cause

The `atmos-auth` probe matches a **top-level** `auth:` key only. A stray `auth:` nested under
`settings:` or inside a component's vars does not count — by design, to avoid false positives
from component-level auth config.

The probe scans:

- The primary `atmos.yaml` / `.atmos.yaml` (and `.yml` variants)
- Every `*.yaml` / `*.yml` file under `atmos.d/` or `.atmos.d/`

### Fix

If your repo configures Atmos Auth from an unusual location (e.g., a user-configured
`import:` path the probe does not walk), either:

1. Move or add a small auth import under `atmos.d/` so the probe picks it up, or
2. Tell the skill to skip standalone-profile generation and wire OIDC into your existing auth
   block manually. Document the path you took in this file.

## Detector missed a Spacelift-enabled stack

### Symptom

After the skill runs, one or two stacks still try to run through Spacelift and report
`settings.spacelift.workspace_enabled: true` when resolved.

### Cause

The `spacelift` probe does a **conservative text scan** for `workspace_enabled: true` — it
does not resolve inherited stack config. If a stack inherits `workspace_enabled: true` from a
parent `_defaults.yaml` and the override never appears literally in a descendant file, the
probe cannot see it.

### Fix

1. After the skill completes, spot-check the suspect stacks with:

   ```shell
   atmos describe component <component> -s <stack> --format=json | jq '.settings.spacelift.workspace_enabled'
   ```

2. If any still resolve to `true`, the mixin is not imported in that org's `_defaults.yaml`.
   Add the import manually or re-run the skill with the correct target org.

## Detector false-positive on Geodesic

### Symptom

The skill adds the Geodesic section to `docs/atmos-pro.md` even though your team does not use
Geodesic.

### Cause

The `geodesic` probe matches any of:

- `Dockerfile` containing `cloudposse/geodesic`
- `geodesic.mk` existing (any contents)
- `.envrc` containing `geodesic`
- `Makefile` containing `cloudposse/geodesic`

A forgotten example Dockerfile or a `.envrc` in a sibling repo vendored under `vendor/` can
trigger a positive detection.

### Fix

Delete or rename the stale marker file, or edit the generated `docs/atmos-pro.md` to remove
the Geodesic section. This does not affect the rest of the generated output.
