# Migrating from Granted

This reference is the agent's decision guide for users coming from
[Granted](https://github.com/common-fate/granted) (the `assume` CLI). There is no standalone
prose tutorial for this migration yet -- for the full auth configuration schema, see the
[atmos-auth](../../atmos-auth/SKILL.md) skill and its
[providers-and-identities.md](../../atmos-auth/references/providers-and-identities.md) reference.

Granted is a terminal-first session manager built **on top of standard AWS SSO profiles** in
`~/.aws/config` (plus `~/.granted/config.yaml` for browser preferences). This means migrating the
underlying credentials is exactly the SSO and role-chaining migration already covered in
[from-aws-config.md](from-aws-config.md) -- this guide only adds the command-equivalence table and
the Granted-specific gotchas. Don't duplicate the schema mapping here; link to it.

## Identifying Granted-Managed Profiles

Signs the user is on Granted rather than raw AWS CLI:

- `~/.granted/config.yaml` exists (browser preferences, custom SSO browser settings).
- `~/.aws/config` profiles named `<account>.<role>` (Granted's default naming from
  `granted sso populate`).
- The user talks about running `assume <profile>` rather than `export AWS_PROFILE=<profile>`.

Once identified, treat the underlying `~/.aws/config` profiles exactly as in
[from-aws-config.md](from-aws-config.md) -- SSO profiles become `aws/iam-identity-center` +
`aws/permission-set`, role-chained profiles become `aws/assume-role` with `via.identity`.

## Bulk Profile Generation → `auto_provision_identities`

**Before:**
```bash
granted sso populate --sso-region us-east-1 --sso-start-url https://acme.awsapps.com/start
```
This bulk-generates one `~/.aws/config` profile per account/permission-set combination the user
can access.

**After:** set `auto_provision_identities: true` on the `aws/iam-identity-center` provider --
Atmos's equivalent bulk-discovery behavior (requires `sso:ListAccounts` and
`sso:ListAccountRoles` on the calling identity):
```yaml
auth:
  providers:
    acme-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://acme.awsapps.com/start
      auto_provision_identities: true
```

## Command Equivalence

| Granted command                          | `atmos auth` equivalent            |
|--------------------------------------------|--------------------------------------|
| `assume <profile>`                          | `atmos auth shell -i <identity>`      |
| `assume --export <profile>` / `assume -e`   | `atmos auth env -i <identity>`        |
| `assume -c <profile>` / `assume console <profile>` | `atmos auth console -i <identity>` |
| `assume -a` (fuzzy picker, no profile given) | any `atmos auth` command with `-i` omitted (interactive picker when no `default: true` identity is set) |
| `granted sso populate`                      | `auto_provision_identities: true`     |

## Common Gotchas

- **Granted's simultaneous multi-account browser sessions map to `atmos auth console --isolated`.**
  Each identity opens in its own isolated Chrome/Chromium profile (a per-realm+identity
  `--user-data-dir`), so multiple accounts' consoles can be open and authenticated concurrently --
  the direct equivalent of Granted's Firefox Multi-Account Containers:
  ```bash
  atmos auth console --identity dev-account --isolated
  atmos auth console --identity staging-account --isolated
  atmos auth console --identity prod-account --isolated
  ```
  Set `auth.console.isolated: true` in `atmos.yaml` to make this the default for every
  `atmos auth console` call instead of passing `--isolated` each time. Requires Chrome/Chromium --
  falls back to the default browser (single-session) with a warning if neither is found.
- **Role-chained profiles follow the exact same `via.identity` chaining** as raw AWS CLI config --
  see [from-aws-config.md](from-aws-config.md#role-chaining-source_profile--role_arn--viaidentity),
  don't re-derive it here.
- **If Granted was registered as a `credential_process`** for other tooling, the same limitation
  documented in [from-aws-config.md](from-aws-config.md#no-equivalent-credential_process) applies
  -- there's no Atmos equivalent for `credential_process`-based integration.

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference; [from-aws-config.md](from-aws-config.md) for the underlying SSO/role-chaining schema
this guide builds on.
