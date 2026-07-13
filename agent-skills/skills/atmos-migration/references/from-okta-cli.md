# Migrating from okta-aws-cli

This reference is the agent's decision guide for users coming from
[okta-aws-cli](https://github.com/okta/okta-aws-cli), Okta's official tool for generating
temporary AWS credentials. **Correction to an earlier version of this guide:** okta-aws-cli does
not target a separate "OIDC AWS app" distinct from a "classic SAML app" -- verified against the
upstream README, there is only one Okta AWS Federation Application type, and it is SAML-based.
Both `okta-aws-cli web` and Atmos's `aws/saml` provider ultimately authorize AWS access the same
way: `AssumeRoleWithSAML` against that one app. The real fork is **how the human authenticates to
Okta**, not which AWS-side app type is configured. Get this distinction right; the app-type framing
this guide used before would have sent users down a dead end diagnosing the wrong thing.

For the full auth configuration schema, see the [atmos-auth](../../atmos-auth/SKILL.md) skill and
its [providers-and-identities.md](../../atmos-auth/references/providers-and-identities.md)
reference.

## Don't Confuse This With Okta's Other CLIs

"Okta CLI" is overloaded across three distinct, unrelated tools -- verify which one a user
actually means before assuming this guide applies:

| Tool | Purpose | Relevant here? |
|------|---------|----------------|
| `okta-aws-cli` (github.com/okta/okta-aws-cli) | Generates temporary AWS IAM credentials via Okta SSO/SAML/OIDC federation | **Yes -- this is what this guide covers** |
| "Okta CLI" (`cli.okta.com`, `okta start`/`okta apps create`) | Scaffolds sample OIDC apps and registers free developer orgs. Deprecated 2025-07-18. | No |
| "Okta CLI Client" (github.com/okta/okta-cli-client, the deprecation notice's official replacement) | General Okta Management API client -- create/manage users, groups, and apps *within* an Okta org, authenticated to Okta itself via an API token or OAuth2 client-credentials/private-key JWT | No |

The latter two never talk to AWS, never produce AWS credentials, and have no concept of a
"profile" or "role" the way `okta-aws-cli`, saml2aws, or Granted do -- there is nothing in
`atmos auth` that overlaps with "manage users/groups/apps in my Okta org," so no migration guide
applies to them. Don't invent one if a user asks; confirm which tool they mean first.

## What okta-aws-cli Actually Does (verified against the upstream README)

`okta-aws-cli` has three commands, not one:

| Command  | Mechanism                                                                 |
|----------|----------------------------------------------------------------------------|
| `web`    | Human-oriented. OIDC device-authorization/browser flow authenticates the human to Okta; the CLI never prompts for a password directly. Requires **Okta Identity Engine (OIE) -- does not work with Classic orgs.** Result is exchanged via `AssumeRoleWithSAML` against Okta's AWS Federation Application (SAML-based). |
| `m2m`    | Machine-to-machine. Headless, private-key JWT client-assertion auth -- no human, no browser, no password. |
| `direct` | Human or headless, using Okta's newer Direct Authentication (out-of-band MFA) grant. |

Atmos's `aws/saml` + `driver: Okta` provider (built on the vendored `saml2aws` library) is a
**different client-side authentication method against the same kind of SAML AWS Federation App**:
it calls Okta's classic `/api/v1/authn` API directly with username/password plus an MFA challenge
(Push, TOTP, SMS, WebAuthn, YubiKey, Duo), then exchanges the resulting session for the same SAML
assertion `okta-aws-cli web` would get.

## Identifying the User's Shape

| User has...                                                              | Maps to |
|--------------------------------------------------------------------------|---------|
| Uses `okta-aws-cli web`, and their org allows direct password+MFA API login (true for Classic orgs; true for many OIE orgs unless locked down) | [Full Replacement](#full-replacement--awssaml--driver-okta) -- just try it |
| Uses `okta-aws-cli web`, and the org enforces browser-only/phishing-resistant login (blocks direct `/api/v1/authn` password auth) | [Not Yet Supported: OIDC Device-Flow Login](#not-yet-supported-oidc-device-flow-login) |
| Uses `okta-aws-cli m2m` (headless, private-key JWT)                      | [Not Yet Supported: m2m / Direct Authentication](#not-yet-supported-m2m--direct-authentication) |
| Uses `okta-aws-cli direct` (Direct Authentication / out-of-band MFA)     | [Not Yet Supported: m2m / Direct Authentication](#not-yet-supported-m2m--direct-authentication) |
| Wants an Okta token for Azure/GCP federation or direct Okta API calls (not AWS) | [Non-AWS Okta Token Use](#non-aws-okta-token-use-also-not-yet-supported) |

There's no reliable way to tell from the outside whether an org blocks direct password API auth --
**the practical test is to just try `atmos auth login` with `driver: Okta`** and see if it
succeeds. If it does, migrate. If Okta rejects the direct API auth attempt (commonly surfaced as
an Okta policy/factor error rather than a generic auth failure), that's the signal the org has
locked this down and the OIDC device-flow gap applies.

## Full Replacement → `aws/saml` + `driver: Okta`

**Before:** `okta-aws-cli web --oidc-client-id ... --aws-acct-fed-app-id ... --org-domain ...`

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

## Command Equivalence

| okta-aws-cli command                                                      | `atmos auth` equivalent |
|---------------------------------------------------------------------------|--------------------------|
| `okta-aws-cli web --oidc-client-id ... --aws-acct-fed-app-id ... --org-domain ...` | `atmos auth login -i <identity>` -- **only if** the org allows direct password+MFA API auth, see [Identifying the User's Shape](#identifying-the-users-shape) |
| `okta-aws-cli web --write-aws-credentials --profile <name>`               | Not needed -- Atmos never writes to `~/.aws/credentials`; use `atmos auth exec -i <identity> -- <cmd>` or `eval $(atmos auth env -i <identity>)` instead |
| `okta-aws-cli list-profiles`                                               | `atmos auth list` |
| `okta-aws-cli m2m` / `okta-aws-cli direct`                                  | **Not supported yet** -- see [Not Yet Supported: m2m / Direct Authentication](#not-yet-supported-m2m--direct-authentication) |

Get the SAML SSO URL from the same Okta AWS Federation Application's Sign On tab that
`--aws-acct-fed-app-id` points at -- it's the same app either way. This drives Okta's
`/api/v1/authn` API directly -- an API/HTTP flow, not browser automation, needing no Playwright
drivers -- and supports the full range of Okta MFA factors: Push (Okta Verify), TOTP (Google
Authenticator or Okta Verify), SMS, Symantec VIP, FIDO WebAuthn, YubiKey hardware tokens, and Duo.
`atmos auth login/exec/shell/console` replaces `okta-aws-cli web`, and Atmos never needs to write
into `~/.aws/config`/`~/.aws/credentials` to make a session usable -- see
[from-aws-config.md's "Shells, exec, and Your Default AWS Config File"](from-aws-config.md#shells-exec-and-your-default-aws-config-file)
for exactly how `atmos auth shell`/`exec`/`env` work instead.

**Note:** `okta-aws-cli` itself requires Okta Identity Engine -- it does not work at all with
Classic orgs. If the user is migrating away from `okta-aws-cli`, their org is necessarily on OIE.
Whether the SAML path above works depends on whether that specific OIE org still permits direct
password+MFA API authentication (see the shape table above), not on the org being OIE vs Classic.

## Not Yet Supported: OIDC Device-Flow Login

**Before:** `okta-aws-cli web`, in an org that has locked down direct password API auth and
requires the OIDC device-authorization/browser flow specifically (increasingly common under strict
phishing-resistant-auth policies).

**After: there is no Atmos equivalent today.** Do not fabricate a mapping -- if `atmos auth login`
with `driver: Okta` fails because the org blocks direct API auth, there's no workaround via
`aws/saml`. This is a confirmed, publicly acknowledged gap, not a guess: Atmos's own team has
scoped a dedicated `okta/*` identity provider using OAuth 2.0 Device Authorization Grant, intended
to federate into AWS, Azure, and GCP plus direct Okta API access, as a planned (not yet shipped)
feature -- check [atmos.tools/roadmap](https://atmos.tools/roadmap) for current status, since this
guide's own state can drift out of date.

Give the user honest options:

1. **Keep using `okta-aws-cli web` standalone** alongside Atmos until native Okta device-flow
   support ships -- no harm running both during the gap.
2. **Point them at the public roadmap** if they want to track when this lands.
3. Do not suggest asking their org to weaken its auth policy just to unblock this migration --
   that trades a real security control for CLI convenience.

## Not Yet Supported: m2m / Direct Authentication

`okta-aws-cli m2m` (headless private-key JWT client assertion) and `okta-aws-cli direct` (Direct
Authentication / out-of-band MFA) are architecturally distinct from both the `web` command and
from Atmos's `aws/saml` provider -- neither is a "different driver setting," they're different
Okta grant types entirely. There is no Atmos equivalent for either today. Same gap and roadmap
pointer as above; don't propose `aws/saml` as a substitute for these, it doesn't apply.

## Non-AWS Okta Token Use (Also Not Yet Supported)

Users who want an Okta access token for Azure federated workload identity, GCP workload identity
federation, direct Okta API calls (user info, groups), or third-party OIDC services hit the same
gap -- Atmos's only Okta integration point today is the AWS-specific SAML `driver: Okta`. There is
no general-purpose Okta identity provider yet. Same planned-but-not-shipped roadmap item as above.

## Common Gotchas

- **There is one Okta AWS Federation Application type, not two.** Don't ask users to check for a
  "SAML app vs. OIDC app" -- that distinction doesn't exist on the Okta side. The fork is entirely
  about which Okta-side auth policy applies to `/api/v1/authn` for their org.
- **`driver: Okta` is SAML under the hood** -- it produces a SAML assertion for
  `AssumeRoleWithSAML`, same as `okta-aws-cli web` ultimately does, just reached via a different
  client-side login method (direct password+MFA vs. OIDC device/browser flow).
- **The only real way to know if the SAML path works for a given org is to try it.** Org auth
  policies (FastPass enforcement, phishing-resistant-only rules) aren't visible from outside Okta
  admin settings, and even those don't map cleanly to "will `/api/v1/authn` accept a password."
- **`m2m` and `direct` are not the same gap as `web`'s device-flow requirement** -- don't conflate
  all three okta-aws-cli commands into one bucket when explaining what's missing.
- **The MFA factor list is often the deciding factor** for whether a user trusts dropping their
  CLI for the SAML path. If they ask "does this support my hardware key/Duo/push," the answer is
  yes -- list the specific factors above rather than a vague "yes, MFA is supported."

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference; [from-aws2saml.md](from-aws2saml.md) for the same underlying `aws/saml` provider
mechanism from a different source tool's perspective.
