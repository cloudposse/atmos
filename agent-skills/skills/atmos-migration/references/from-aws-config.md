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

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference; [atmos-secrets](../../atmos-secrets/SKILL.md) if the user wants to store static
credentials as a declared secret rather than relying on the keyring.
