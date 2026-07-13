# Migrating from okta-aws-cli

This reference is the agent's decision guide for users coming from
[okta-aws-cli](https://github.com/okta/okta-aws-cli), Okta's official tool for generating
temporary AWS credentials. Unlike the other `from-*` guides, **this one is partial support, not a
full mapping** -- Atmos supports one of okta-aws-cli's two integration modes today and explicitly
does not support the other yet. Get this distinction right before proposing any migration; getting
it wrong sends the user down a path that will not work.

For the full auth configuration schema, see the [atmos-auth](../../atmos-auth/SKILL.md) skill and
its [providers-and-identities.md](../../atmos-auth/references/providers-and-identities.md)
reference.

## Identifying the User's Shape (this determines whether migration is even possible today)

`okta-aws-cli` works against one of two different Okta AWS app integration types. The user's Okta
admin console tells you which one they have -- check the AWS app's **Sign On** tab:

| Okta AWS app type                                                    | okta-aws-cli mode                     | Atmos support today |
|--------------------------------------------------------------------------|------------------------------------------|------------------------|
| Classic **SAML 2.0** app                                                  | SAML-based credential retrieval           | **Supported** -- see [SAML (Supported Today)](#saml-supported-today--awssaml--driver-okta) |
| Newer **AWS Account Federation** app (OIDC)                               | OAuth 2.0 Device Authorization Grant      | **Not supported yet** -- see [OIDC Device Authorization Grant (Not Yet Supported)](#oidc-device-authorization-grant-not-yet-supported) |
| Okta tokens used for Azure/GCP federation, direct Okta API calls, or third-party OIDC services | N/A (not AWS-specific)         | **Not supported yet** -- same gap, see below |

Don't guess -- ask the user (or have them check) which app type they're on before proposing a
migration path. If they're not sure, "was I ever prompted for a device code and a URL to visit in
a browser on another device?" is the OIDC device-flow tell; the SAML app just redirects straight
through a normal browser SSO flow.

## SAML (Supported Today) → `aws/saml` + `driver: Okta`

**Before:** `okta-aws-cli` (or the org's classic Okta SAML AWS app, or bare `saml2aws --provider
Okta`) against a SAML 2.0 AWS app in Okta.

**After (`atmos.yaml`):**
```yaml
auth:
  providers:
    okta-saml:
      kind: aws/saml
      region: us-east-1
      url: https://company.okta.com/app/amazon_aws/abc123/sso/saml
      driver: Okta

  identities:
    admin:
      kind: aws/assume-role
      via:
        provider: okta-saml
      principal:
        assume_role: arn:aws:iam::123456789012:role/AdminRole
```

This drives Okta's `/api/v1/authn` API directly (verified in `pkg/auth/providers/aws/saml.go`,
backed by the vendored `saml2aws` Okta provider) -- it's an API/HTTP flow, not browser automation,
and needs no Playwright drivers the way the generic `Browser` driver does. It supports the full
range of Okta MFA factors: Push (Okta Verify), TOTP (Google Authenticator or Okta Verify), SMS,
Symantec VIP, FIDO WebAuthn, YubiKey hardware tokens, and Duo. This is a full replacement for
`okta-aws-cli` in SAML mode -- `atmos auth login/exec/shell/console` drives the same Okta
authentication the CLI would, then stores the resulting AWS credentials via Atmos's keyring
instead of a separate credential cache.

## OIDC Device Authorization Grant (Not Yet Supported)

**Before:** `okta-aws-cli` in its default/primary mode -- OAuth 2.0 Device Authorization Grant
against Okta's newer "AWS Account Federation" OIDC app, producing a device code and a
`https://.../activate` URL the user visits to approve the login.

**After: there is no Atmos equivalent today.** Do not fabricate a mapping -- `aws/saml` +
`driver: Okta` is SAML-based under the hood and cannot talk to an OIDC AWS Account Federation app.
This is a confirmed, explicitly tracked gap:

- `docs/prd/okta-auth-identity.md` lists the current state's limitations verbatim: *"SAML-only:
  Only supports SAML assertions for AWS, not OAuth/OIDC tokens... No device authorization flow:
  Cannot use modern OAuth Device Authorization Grant."*
- The roadmap tracks the fix as **"Native Okta Authentication (Device Code Flow)"**
  (`website/src/data/roadmap.js`, status: planned, quarter: Q1 2026, PRD: `okta-auth-identity`) --
  a dedicated `okta/*` identity provider using Device Authorization Grant, intended to federate
  into AWS, Azure, and GCP plus direct Okta API access.

Give the user honest options, not a workaround dressed up as a solution:

1. **If their org can reconfigure the Okta AWS app as classic SAML instead of OIDC federation**,
   migrate via the SAML path above.
2. **Otherwise, keep using `okta-aws-cli` standalone** alongside Atmos until the native `okta/*`
   provider ships -- there's no harm in running both tools during the gap.
3. **Point them at the roadmap item** if they want to track when this lands.

## Non-AWS Okta Token Use (Also Not Yet Supported)

Users who want an Okta access token for Azure federated workload identity, GCP workload identity
federation, direct Okta API calls (user info, groups), or third-party OIDC services hit the same
gap -- Atmos's only Okta integration point today is the AWS-specific SAML `driver: Okta`. There is
no general-purpose Okta identity provider yet. This is the same `okta-auth-identity` PRD/roadmap
item as above; don't propose a workaround using `aws/saml` for non-AWS targets, it doesn't apply.

## Common Gotchas

- **`driver: Okta` is SAML, not OIDC** -- despite needing no browser/Playwright drivers (it's an
  API/HTTP flow), it produces a SAML assertion for `AssumeRoleWithSAML`, not an OIDC token for
  `AssumeRoleWithWebIdentity`. Don't conflate "no browser needed" with "OIDC support."
  `aws/assume-role`'s `principal.assume_role` still expects a role ARN either way, but the
  underlying trust relationship on the AWS side must be configured for SAML federation, not OIDC.
- **Confirm the Okta AWS app type before promising a migration path.** This is the single most
  common way to send a user down a dead end -- always check the Sign On tab first.
- **The MFA factor list is often the deciding factor** for whether a user trusts dropping their
  CLI. If they ask "does this support my hardware key/Duo/push," the answer is yes -- list the
  specific factors above rather than a vague "yes, MFA is supported."

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference; [from-aws2saml.md](from-aws2saml.md) for the same underlying `aws/saml` provider
mechanism from a different source tool's perspective.
