# Migrating from AWS CLI Config

This reference is the agent's decision guide for users coming from `~/.aws/config` and
`~/.aws/credentials`. There is no standalone prose tutorial for this migration yet -- for the full
auth configuration schema, see the [atmos-auth](../../atmos-auth/SKILL.md) skill and its
[providers-and-identities.md](../../atmos-auth/references/providers-and-identities.md) reference.

## Identifying the User's Shape

A `~/.aws/config`/`~/.aws/credentials` setup is rarely just one thing -- most users have a mix.
Identify each profile's shape before proposing YAML:

| Profile has...                                                     | Maps to                                          |
|----------------------------------------------------------------------|--------------------------------------------------|
| `sso_start_url`/`sso_region`/`sso_account_id`/`sso_role_name`, or an `[sso-session]` block | [SSO Profiles](#sso-profiles--awsiam-identity-center--awspermission-set) |
| `aws_access_key_id`/`aws_secret_access_key` in `~/.aws/credentials`  | [Static Access Keys](#static-access-keys--awsuser) |
| `source_profile` + `role_arn` (one or more hops)                     | [Role Chaining](#role-chaining-source_profile--role_arn--viaidentity) |
| The above plus `mfa_serial`                                          | [MFA-Gated Role Assumption](#mfa-gated-role-assumption-mfa_serial) |
| `credential_process`                                                 | [No Equivalent: credential_process](#no-equivalent-credential_process) |

Most real-world configs combine two or three of these (e.g., one SSO base profile, several
`role_arn`/`source_profile` profiles chained off it). Migrate the base profile first, then layer
the chains on top -- don't try to translate the whole file in one pass.

## Command Equivalence

| Old AWS CLI workflow                                                   | `atmos auth` equivalent |
|----------------------------------------------------------------------------|----------------------------|
| `export AWS_PROFILE=x` then run `aws ...`                                  | `atmos auth exec -i x -- aws ...` (one-off) or `atmos auth shell -i x` then `aws ...` (session) |
| `aws sso login --profile x`                                                | `atmos auth login -i x` |
| `aws sts get-caller-identity --profile x`                                  | `atmos auth whoami -i x` |
| `aws configure list-profiles`                                              | `atmos auth list` |
| Manually running `aws sts assume-role --role-arn ... --profile base` and exporting the result | No manual step -- `via.identity` chaining does this automatically on every `atmos auth login`/`exec`/`shell` |
| `eval $(aws configure export-credentials --profile x --format env)`        | `eval $(atmos auth env -i x)` |
| Switching profiles between terminal tabs by re-exporting `AWS_PROFILE`     | `atmos auth shell -i x` per tab (isolated) or `atmos auth env -i x` per tab |

## Shells, `exec`, and Your Default AWS Config File

**By default, Atmos does not read or write your system's default `~/.aws/config` or
`~/.aws/credentials`.** It manages its own credential files elsewhere and keeps them out of the
files the raw AWS CLI reads by default -- so migrating to Atmos doesn't clobber any profiles the
user (or other tools) still rely on outside of Atmos.

The two recommended patterns for running commands under an identity:

- **`atmos auth shell -i <identity>`** -- launches a subshell with that identity's credentials
  active. Nothing outside the subshell is affected; exiting it cleans up.
- **`atmos auth exec -i <identity> -- <command>`** -- runs a single command with the identity's
  credentials injected, no subshell needed.

**If the user wants their normal shell (and tools that assume the default file locations) to
"just work"** without a subshell or command wrapper, `atmos auth env` is the answer:

```bash
eval $(atmos auth env --identity dev-admin)
aws s3 ls   # picks up Atmos-managed credentials transparently
```

This doesn't write into `~/.aws/config`/`~/.aws/credentials` either -- it exports
`AWS_CONFIG_FILE` and `AWS_SHARED_CREDENTIALS_FILE` pointing at Atmos-managed files (plus
`AWS_PROFILE`/`AWS_REGION`), which the AWS CLI and SDKs honor transparently. The user's own
default files stay untouched; `atmos auth env` just redirects where the *current shell* looks.
Safe to put `eval $(atmos auth env)` in `.bashrc`/`.zshrc` -- it doesn't trigger a login prompt by
itself (add `--login` if the user wants that too).

## SSO Profiles → `aws/iam-identity-center` + `aws/permission-set`

**Before (`~/.aws/config`):**
```ini
[profile dev-admin]
sso_start_url = https://company.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789012
sso_role_name = AdminAccess
region = us-east-1
```

**After (`atmos.yaml`):**
```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start
      auto_provision_identities: true   # optional: bulk-discover accounts/permission sets

  identities:
    dev-admin:
      kind: aws/permission-set
      default: true
      via:
        provider: company-sso
      principal:
        name: AdminAccess
        account:
          id: "123456789012"   # or account.name, if resolved via SSO ListAccounts
```

One `aws/iam-identity-center` provider serves every profile that shares the same
`sso_start_url`/`sso_region` (including newer `[sso-session]`-based configs, which are the same
shape -- the session block's `sso_start_url`/`sso_region` become the provider, each profile that
references the session becomes an `aws/permission-set` identity). Don't create one provider per
profile.

## Static Access Keys → `aws/user`

**Before (`~/.aws/credentials`):**
```ini
[emergency-access]
aws_access_key_id = AKIA...
aws_secret_access_key = ...
```

**After (`atmos.yaml`):**
```yaml
auth:
  identities:
    emergency-access:
      kind: aws/user
      credentials:
        access_key_id: !env AWS_ACCESS_KEY_ID
        secret_access_key: !env AWS_SECRET_ACCESS_KEY
        region: us-east-1
```

`aws/user` is the only identity kind that needs no `via` -- credentials are inline. Prefer
`atmos auth user configure --identity emergency-access` over hand-writing keys into `atmos.yaml`
or shell env files; it stores them in the OS keyring (or the configured keyring backend) instead
of a config file a teammate or CI log could leak.

## Zero-Config Alternative: Browser-Based Login (No Static Credentials At All)

Be explicit with users that static keys are no longer required, full stop -- this is a bigger
improvement over raw AWS CLI profiles than the sections above suggest. `aws/user` identities fall
back automatically to a browser OAuth2 PKCE flow against AWS's own sign-in service whenever no
static credentials and no keyring entry are configured:

```yaml
auth:
  identities:
    dev:
      kind: aws/user   # no `credentials:` block at all
```

`atmos auth login -i dev` opens the default browser, completes AWS's PKCE flow, and caches a
refresh token for 12-hour session reuse (credentials auto-refresh every 15 minutes within that
window). In non-interactive environments (CI, remote servers) Atmos prints a URL to open manually.
This is the same convenience SSO users already had -- IAM users and even root accounts no longer
need `aws_access_key_id`/`aws_secret_access_key` anywhere.

**This changes the migration recommendation:** for a user's static-key profiles that aren't
break-glass/emergency credentials, prefer pointing them at this zero-config path over migrating
the keys at all. Reserve inline `credentials:` (or `atmos auth user configure`) for cases that
specifically need long-lived, non-interactive credentials -- CI service accounts, break-glass
access, or environments where a browser genuinely isn't reachable even for the manual-URL fallback.

Set `credentials.webflow_enabled: false` to disable this fallback if the user wants strict
static-only or keyring-only resolution (e.g., to fail loudly instead of prompting a browser during
automation).

## Role Chaining (`source_profile` + `role_arn`) → `via.identity`

**Before (`~/.aws/config`):**
```ini
[profile base]
sso_start_url = https://company.awsapps.com/start
sso_region = us-east-1
sso_account_id = 111111111111
sso_role_name = AdminAccess

[profile prod]
role_arn = arn:aws:iam::999999999999:role/ProductionAdmin
source_profile = base
role_session_name = terraform
```

**After (`atmos.yaml`):**
```yaml
auth:
  identities:
    base:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: AdminAccess
        account:
          id: "111111111111"

    prod:
      kind: aws/assume-role
      via:
        identity: base
      principal:
        assume_role: arn:aws:iam::999999999999:role/ProductionAdmin
        session_name: terraform
```

Multi-hop chains (`profile-a` → `profile-b` → `profile-c`) translate the same way: each identity's
`via.identity` points at the previous hop. `via.provider` and `via.identity` are mutually
exclusive -- only the first identity in a chain uses `via.provider`.

## MFA-Gated Role Assumption (`mfa_serial`)

**Before:** a `source_profile`/`role_arn` profile with `mfa_serial` set.

**After:** put `mfa_arn` on the *upstream* `aws/user` identity, not on the `aws/assume-role`
identity -- `aws/assume-role` has no MFA field. The already-MFA'd session from the upstream
identity satisfies any `aws:MultiFactorAuthPresent` condition on the assumed role's trust policy:

```yaml
auth:
  identities:
    base:
      kind: aws/user
      credentials:
        access_key_id: !env AWS_ACCESS_KEY_ID
        secret_access_key: !env AWS_SECRET_ACCESS_KEY
        region: us-east-1
        mfa_arn: arn:aws:iam::123456789012:mfa/username   # MFA lives here

    prod:
      kind: aws/assume-role
      via:
        identity: base
      principal:
        assume_role: arn:aws:iam::999999999999:role/ProductionAdmin
```

The same rule applies when the upstream identity is `aws/permission-set` instead of `aws/user` --
MFA (if the IdP requires it) happens during the SSO browser login, not as a separate config field.

## No Equivalent: `credential_process`

Atmos Auth does not execute `credential_process` commands -- it deliberately ignores that key when
reading a legacy `~/.aws/config` for compatibility purposes. There is no drop-in mapping. Instead:

- If the process wraps SSO or role assumption (a custom script, `aws-vault`, a Leapp/Granted/
  saml2aws shim), migrate to the native kind that process was wrapping -- see the SSO and
  role-chaining sections above, or [from-granted.md](from-granted.md) /
  [from-aws2saml.md](from-aws2saml.md) if that's literally what it was calling.
- If it wraps something bespoke (an internal secrets broker, a Vault dynamic AWS secrets engine),
  there's currently no native Atmos Auth kind for that. Don't fabricate a mapping -- tell the user
  this profile has no equivalent yet.

## Beyond Plain AWS CLI Config: Native Integrations

Plain `~/.aws/config` has no notion of these -- worth surfacing during a migration conversation
since they often replace scripts the user built by hand.

- **ECR login, automated.** Instead of scripting
  `aws ecr get-login-password | docker login --username AWS --password-stdin`, wire an `aws/ecr`
  integration to an identity and Atmos writes `~/.docker/config.json` automatically on
  `atmos auth login`:
  ```yaml
  auth:
    integrations:
      ecr:
        kind: aws/ecr
        via:
          identity: dev-admin
        spec:
          registry:
            account_id: "123456789012"
            region: us-east-2
  ```
- **EKS kubeconfig, automated.** `atmos auth login` configures kubeconfig to call
  `atmos aws eks token` as a kubectl exec-credential plugin using the active identity -- no more
  manually running `aws eks update-kubeconfig` and hoping the right profile/role is active.
- **`aws/assume-root` -- a capability plain profiles can't express at all.** AWS's newer
  centralized-root-access feature (temporary, task-scoped root credentials via AWS Organizations)
  has no representation in classic `~/.aws/config` -- there's no `role_arn` for "become root for
  15 minutes to run one IAM audit task." Atmos supports it as a first-class identity kind:
  ```yaml
  auth:
    identities:
      root-audit:
        kind: aws/assume-root
        via:
          identity: admin-base
        principal:
          target_principal: "123456789012"
          task_policy_arn: arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials
          duration: 15m   # AWS-enforced hard max
  ```
  Only surface this if the user specifically needs AWS Organizations centralized root access
  tasks (credential recovery, root password creation/deletion, S3/SQS policy unlocking) -- it's
  niche, not a general migration target, and requires AWS Organizations centralized root access to
  be enabled.

## Common Gotchas

- **`principal.name` must be the exact permission set name** from IAM Identity Center, not a
  display name or alias the user made up locally.
- **`aws/assume-role` has no `mfa_arn` field.** MFA always lives on the upstream `aws/user` (or is
  handled transparently during SSO login) -- putting `mfa_arn` on an `aws/assume-role` identity is
  silently ignored.
- **One provider per SSO start URL, not one per profile.** Users with a dozen SSO profiles need
  one `aws/iam-identity-center` provider and a dozen `aws/permission-set` identities, not a dozen
  providers.
- **`credential_process` is silently ignored, not translated.** If a profile still has it after
  migration, `atmos auth` will not pick up whatever it produced.
- **Static keys in `atmos.yaml` are a downgrade from `atmos auth user configure`.** Steer users
  away from `!env`-referenced keys committed to a shared repo when the keyring-backed command
  achieves the same result more safely.
- **Static keys aren't even required anymore.** An `aws/user` identity with no `credentials:`
  block at all falls back to the browser OAuth2 PKCE flow automatically -- don't default to
  migrating every static-key profile as static keys in Atmos too; ask whether the user actually
  needs long-lived non-interactive credentials before reaching for `credentials:` or
  `atmos auth user configure`.

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference; [atmos-secrets](../../atmos-secrets/SKILL.md) if the user wants to store static
credentials as a declared secret rather than relying on the keyring.
