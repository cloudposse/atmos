# Migrating from Leapp

This reference is the agent's decision guide for users coming from
[Leapp](https://github.com/Noovolari/leapp), a standalone GUI credential manager. For the full
user-facing prose tutorial with screenshots, see
[atmos.tools/tutorials/migrating-from-leapp](https://atmos.tools/tutorials/migrating-from-leapp).
Leapp is used almost exclusively for AWS IAM Identity Center (SSO) sessions in practice, so unlike
the other `from-*` guides this is a single scenario, not a routing table.

## Concept Mapping

Leapp's five concepts map directly onto `atmos auth` config:

| Leapp concept                          | `atmos auth` location                              |
|-----------------------------------------|-----------------------------------------------------|
| Provider (sidebar "Integrations")       | `providers.<name>`                                   |
| Provider's Region                       | `providers.<name>.region`                             |
| Session's account                       | `identities.<name>.principal.account.name` (or `.id`) |
| Session's Identity (permission set)     | `identities.<name>.principal.name`                    |
| Named Profile                           | `identities.<name>` (the YAML key itself)             |

## Session → Identity

**Before:** a Leapp session with Provider=`acme`, Session (account)=`core-identity`,
Identity (permission set)=`IdentityManagersTeamAccess`, Named Profile=`acme-identity`,
Region=`us-east-1`.

**After (`atmos.yaml`):**
```yaml
auth:
  providers:
    acme-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://acme.awsapps.com/start/

  identities:
    acme-identity:
      kind: aws/permission-set
      via:
        provider: acme-sso
      principal:
        name: "IdentityManagersTeamAccess"
        account:
          name: "core-identity"
```

Every other Leapp session the user has becomes another `identities.<name>` block under the same
provider (one provider per Leapp "Integration," not per session) -- avoid creating a new provider
for each session.

## Setting a Default Identity

**Before:** Leapp's implicit "quick launch" / most-recently-used session.

**After:** mark exactly one identity `default: true`:
```yaml
auth:
  identities:
    acme-identity:
      default: true
      kind: aws/permission-set
      via:
        provider: acme-sso
      principal:
        name: "IdentityManagersTeamAccess"
        account:
          name: "core-identity"
```

## Command Equivalence

| Leapp action                                            | `atmos auth` equivalent |
|---------------------------------------------------------|--------------------------|
| Quick-launch a session in the Leapp app                 | `atmos auth login -i <identity>` |
| Leapp's "Open Terminal" for a session                   | `atmos auth shell -i <identity>` |
| Leapp's "Generate Credentials" / copy-to-clipboard       | `eval $(atmos auth env -i <identity>)` |

Unlike Leapp, Atmos does not need to write into `~/.aws/config`/`~/.aws/credentials` to make a
session usable -- see
[from-aws-config.md's "Shells, exec, and Your Default AWS Config File"](from-aws-config.md#shells-exec-and-your-default-aws-config-file)
for exactly how `atmos auth shell`/`exec`/`env` interact with (or rather, don't touch) the user's
default AWS CLI config.

## Common Gotchas

- **Provider name mismatch** ("Provider not found" error) -- `via.provider` must exactly match a
  key under `providers:`, not a Leapp integration display name.
- **Permission set name mismatch** -- `principal.name` must be the exact AWS-side permission set
  name, not whatever alias Leapp showed in its UI.
- **MFA is automatic** -- Atmos Auth handles MFA during the `atmos auth login` browser flow the
  same way Leapp did; there's no separate MFA field to configure.
- **Leapp sessions have no built-in role-chaining beyond one hop.** If the user manually chained
  multiple Leapp sessions (assumed a role from within an already-assumed session), that becomes an
  `aws/assume-role` identity with `via.identity` pointing at the `aws/permission-set` identity --
  see [from-aws-config.md](from-aws-config.md#role-chaining-source_profile--role_arn--viaidentity)
  for the general chaining pattern.

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference.
