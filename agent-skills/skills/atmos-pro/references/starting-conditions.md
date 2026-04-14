# Starting-Condition Variants

Every real Atmos Pro onboarding starts from a different shape of repo. This document enumerates
the variants we have seen, how to detect each one, and what the skill must do differently.

Multiple variants can apply at once. Apply the union of behaviors.

## Decision table

| Variant | Detection | Skill behavior |
|---------|-----------|----------------|
| GitHub Actions disabled org-wide | `gh api repos/:owner/:repo/actions/permissions` returns `enabled: false` or `allowed_actions: none` | **Stop.** Print the org settings URL (`https://github.com/organizations/{org}/settings/actions`) and tell the user to enable Actions for the repo. Do not generate files. |
| GitHub Actions enabled per-repo but restricted | Same probe returns `allowed_actions: selected` with no list | Warn. Generate files but tell user to allowlist `actions/checkout` and `aws-actions/configure-aws-credentials` |
| No Atmos Auth in repo | No `auth:` key in `atmos.yaml` or any imported `atmos.d/*.yaml` | Generate standalone `profiles/github-{plan,apply}/atmos.yaml`. Do **not** retrofit an `auth:` block into `atmos.yaml`. Emit a "next steps" section in `docs/atmos-pro.md` describing how to adopt Atmos Auth for local dev later. |
| Atmos Auth already configured | `auth:` block present with at least one provider | Add the `github-oidc` provider to the existing `auth.providers` block via a patch file `atmos.d/atmos-pro.yaml`. Do not edit the user's primary `atmos.yaml`. |
| Federated IAM via Okta (not Identity Center) | SSO login URL in docs or auth config references IdP directly, not Identity Center | Skip the local-dev Atmos Auth recommendation. Generate only CI-side profiles. Note in `docs/atmos-pro.md` that local dev continues using the customer's existing Okta federation. |
| Geodesic-based dev environment | `Dockerfile` references `cloudposse/geodesic` OR `geodesic.mk` exists OR `.envrc` references `geodesic` | Add a Geodesic section to `docs/atmos-pro.md`: bind-mount the worktree, `gh` token passthrough (`-e GITHUB_TOKEN`), `atmos ai` invocation inside the shell. |
| Spacelift currently enabled | Any stack has `settings.spacelift.workspace_enabled: true` (either literal or resolved) | Generate the mixin with `settings.spacelift.workspace_enabled: false`. Add a "Spacelift migration" section to the PR body listing the stacks that were overridden. Do not migrate state or mirror Spacelift policies. |
| `github-oidc-provider` not deployed | `atmos describe component github-oidc-provider -s <stack>` errors, or the Terraform state has no `oidc_provider_arn` output | Generate files, but add a **step-0** to the rollout checklist: "Deploy `github-oidc-provider` to every target account before applying IAM roles." |
| `github-oidc-provider` component not even present in the repo | `atmos list components` does not include `github-oidc-provider` | Add a pre-flight note to the PR body with a link to vendor the component. Do not vendor it automatically — the repo layout for that component varies. |
| tfstate-backend has no `allowed_principal_arns` | Component's resolved `access_roles.default.allowed_principal_arns` is empty or missing | Add the new wildcard patterns additively. Warn the user that prior-existing human roles now share `allowed_principal_arns` with the new bot roles. |
| tfstate-backend `allowed_principal_arns` custom | Already contains non-empty entries | Merge additively. Never overwrite. Print a diff of the final list. |
| tfstate trust policy at IAM size limit | `team_permission_sets_enabled: true` + many teams | Set `team_permission_sets_enabled: false` in the tfstate-backend stack-level vars. Document this in the PR body. |
| Multi-tenant stack hierarchy | `atmos list stacks` yields more than one `{tenant}-{stage}` prefix | Default to **one org only** per invocation. Ask which org to enable. Do not fan out unless `--all-orgs` is passed. |
| Single-account repo | Only one account in the hierarchy | Still generate both plan and apply profiles, but the apply profile's root-account entry points to the plan role (safety rail applies regardless of count). |
| No `stacks/mixins/` convention | No existing `stacks/mixins/` directory | Create the directory. Emit a one-line note in the PR body explaining the mixin pattern. |
| Custom stack layout (non-Cloud-Posse) | `atmos.yaml` `stacks.base_path` is not `stacks/` | Use the configured `stacks.base_path` for all generated stack files. Never hard-code `stacks/`. |
| Branch protection on `main` already strict | `gh api repos/:owner/:repo/branches/main/protection` returns 200 with required reviews | Mention in the PR body: "Atmos Pro will push commits via OIDC; ensure the apply role ARN is allowed to bypass or is excluded from required-signature rules." |
| Required GitHub environments missing | Apply flow uses `github_environment` input but no matching env exists | Warn. Explain that environments are created manually in Settings → Environments; the skill cannot create them via API without admin scope. |

## Deterministic probe package

Filesystem probes are implemented in Go at `pkg/atmospro/detect` and are
invokable from either the skill (via Bash wrapping a future `atmos pro detect`
subcommand) or directly from a Go caller. They cover the three probes that do
not need network access:

| Probe                 | Go function                       | Returns                                           |
|-----------------------|-----------------------------------|---------------------------------------------------|
| Atmos Auth            | `detect.AtmosAuth(fsys)`          | `Result{Detected, Evidence[], Hint}`              |
| Spacelift             | `detect.Spacelift(fsys, dir)`     | `Result{Detected, Evidence[stacks], Hint}`        |
| Geodesic              | `detect.Geodesic(fsys)`           | `Result{Detected, Evidence[markers], Hint}`       |

`detect.All(fsys, "stacks")` runs all three and returns results in deterministic
order. The skill and the future `atmos pro init` command share these probes so
that Path A, Path B, and the deterministic CLI all agree on what a repo looks
like.

Network-dependent probes (GitHub Actions enablement, `github-oidc-provider`
deployment, tfstate-backend introspection) remain shell commands documented
below — the skill's Bash tool runs them directly.

## Shell probes in detail

### GitHub Actions permissions

```shell
gh api repos/:owner/:repo/actions/permissions
```

Expected response fields:

- `enabled` (bool) — must be `true`
- `allowed_actions` (string) — `all`, `local_only`, or `selected`. `selected` means the user
  has an allowlist and must add `actions/checkout` and `aws-actions/configure-aws-credentials`.

Org-level check (if repo-level is inherited):

```shell
gh api orgs/:org/actions/permissions
```

### Atmos Auth presence

Look for `auth:` at the top level of:

- `rootfs/usr/local/etc/atmos/atmos.yaml` (Geodesic config path — check this first)
- `atmos.yaml` / `.atmos.yaml`
- Any file under `rootfs/usr/local/etc/atmos/atmos.d/` or `atmos.d/`
- Any file imported via `import:` in the resolved atmos.yaml

A stray `auth:` under `settings:` does not count — it must be the top-level auth config.

**Geodesic note:** In Geodesic-hosted repos (Cloud Posse reference-architecture-style and other Cloud Posse reference
stacks), `atmos.yaml` does not live at the repo root. Workflows set
`ATMOS_CLI_CONFIG_PATH=./rootfs/usr/local/etc/atmos` and expect the config there.
Agents must resolve the actual config path before any probe that reads it — running
`ls atmos.yaml` at the repo root will fail silently on Geodesic repos. Use
`detect.LocateAtmosYAML(fsys)` from the Go probe package, or the shell snippet in
SKILL.md step 2.

### Spacelift usage

Resolve one stack and inspect:

```shell
atmos describe component <any-component> -s <any-stack> --format=json | \
  jq '.settings.spacelift.workspace_enabled'
```

If `true`, Spacelift is in use for that stack. Walk a sample of stacks to confirm it's not
already partially disabled.

### OIDC provider deployment

```shell
STACK=$(atmos list stacks | head -1)
atmos describe component github-oidc-provider -s "$STACK" --format=json 2>&1
```

Three outcomes:

- Exit 0 with `oidc_provider_arn` in outputs → deployed.
- Exit 0 but outputs are empty → component is configured but not yet applied.
- Non-zero exit with "component not found" → component is not even in the catalog.

### Geodesic detection

Any of:

- `grep -q "FROM.*cloudposse/geodesic" Dockerfile 2>/dev/null`
- `test -f geodesic.mk`
- `grep -q "geodesic" .envrc 2>/dev/null`
- `grep -q "cloudposse/geodesic" Makefile 2>/dev/null`

If any match, treat the repo as Geodesic-hosted.
