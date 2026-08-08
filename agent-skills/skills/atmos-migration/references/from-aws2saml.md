# Migrating from saml2aws

This reference is the agent's decision guide for users coming from
[saml2aws](https://github.com/Versent/saml2aws) (Versent). There is no standalone prose tutorial
for this migration yet -- for the full auth configuration schema, see the
[atmos-auth](../../atmos-auth/SKILL.md) skill and its
[providers-and-identities.md](../../atmos-auth/references/providers-and-identities.md) reference.

This is the tightest mapping of all the auth migration guides: Atmos's own `aws/saml` provider is
built directly on the `saml2aws` library plus Playwright for browser automation. Frame this
migration to the user as "swap the wrapper, keep the engine" rather than a from-scratch
translation.

## Identifying the User's Shape

| `~/.saml2aws` config has...                              | Maps to                                    |
|--------------------------------------------------------------|-----------------------------------------------|
| One `[default]` section                                      | One `aws/saml` provider                        |
| Multiple named profiles/sections, each a distinct IdP app     | One `aws/saml` provider per section            |
| `role_arn` pinned in the config                               | `aws/assume-role` identity with a fixed `principal.assume_role` |
| No `role_arn` -- role chosen interactively at each login       | Same, but create one identity per role the user actually uses |

## SAML Provider → `aws/saml`

**Before (`~/.saml2aws`, verified against the upstream README's example config):**
```ini
[default]
url                  = https://company.okta.com/app/amazon_aws/abc123/sso/saml
username             = user@company.com
provider             = Okta
mfa                  = Auto
aws_urn              = urn:amazon:webservices
aws_session_duration = 3600
aws_profile          = admin
role_arn             = arn:aws:iam::123456789012:role/AdminRole
region               = us-east-1
```

**After (`atmos.yaml`):**
```yaml
auth:
  providers:
    okta-saml:
      kind: aws/saml
      region: us-east-1
      url: https://company.okta.com/app/amazon_aws/abc123/sso/saml
      driver: Okta   # mapped from saml2aws's `provider` field -- see table below
      session:
        duration: 1h
```

`aws/saml` always requires the next identity in the chain to be `aws/assume-role` -- the SAML flow
itself requires selecting a role to assume, the same way saml2aws does.

## Driver Mapping Table

| saml2aws `provider`                                              | Atmos `driver`  |
|----------------------------------------------------------------------|-------------------|
| `Okta`                                                                | `Okta`             |
| `ADFS`, `ADFS2`                                                       | `ADFS`             |
| `GoogleApps`, `GoogleAppsCERT`                                        | `GoogleApps`       |
| `KeyCloak`, `Ping`, `PingOne`, `Shibboleth`, `NetIQ`, `JumpCloud`, `Akamai`, `F5APM`, `browser` | `Browser` (default) |

`Browser` is Atmos's generic Playwright-based automation (the default driver, requires Playwright)
-- it's the fallback for any saml2aws-supported IdP without a dedicated Atmos driver. Validate it
against the user's actual IdP after migration; not every saml2aws-supported IdP has been
explicitly exercised against Atmos's `Browser` driver.

## Role Selection → `aws/assume-role`

**Before:** either a pinned `role_arn` in `~/.saml2aws`, or an interactive role picker shown at
each `saml2aws login`.

**After:**
```yaml
auth:
  identities:
    admin:
      kind: aws/assume-role
      via:
        provider: okta-saml
      principal:
        assume_role: arn:aws:iam::123456789012:role/AdminRole
```

If the user alternates between multiple roles at the saml2aws interactive prompt, create one
`aws/assume-role` identity per role, all chained `via.provider` from the same `aws/saml` provider,
and mark the most-used one `default: true`.

## Command Equivalence

| saml2aws command                        | `atmos auth` equivalent          |
|--------------------------------------------|-------------------------------------|
| `saml2aws login --profile x`                | `atmos auth login -i x`              |
| `saml2aws exec --profile x -- <cmd>`        | `atmos auth exec -i x -- <cmd>`      |
| `saml2aws script --profile x`               | `atmos auth env -i x`                |
| `saml2aws console --profile x`              | `atmos auth console -i x`            |

Unlike `saml2aws login`, which writes credentials into `~/.aws/credentials` under a named profile
by default, Atmos never touches that file -- `atmos auth env` exports `AWS_CONFIG_FILE`/
`AWS_SHARED_CREDENTIALS_FILE` pointing at Atmos-managed files instead. See
[from-aws-config.md's "Shells, exec, and Your Default AWS Config File"](from-aws-config.md#shells-exec-and-your-default-aws-config-file)
for the full explanation.

## Common Gotchas

- **Each `~/.saml2aws` named profile becomes a separate `providers.<name>` + `identities.<name>`
  pair.** Don't collapse multiple profiles into one provider unless they genuinely share the same
  IdP `url`.
- **MFA prompted by the IdP during the browser flow is handled automatically**, the same as native
  saml2aws -- there's no separate Atmos MFA field for SAML logins.
- **If Playwright/browser automation fails post-migration** (download errors, version mismatches),
  that's an Atmos-side browser-driver concern independent of the SAML config translation itself --
  not something to debug by re-checking the YAML.

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference.
