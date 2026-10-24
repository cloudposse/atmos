---
name: atmos-secrets
description: "Atmos Secrets: declarative secrets.vars, !secret, cloud/Vault/1Password/SOPS backends, init/set/get/import/push/pull/shell/exec/validate, masking, and CI validation"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: security
---

# Atmos Secrets

Use this skill for declared secrets, secret backends, and the `atmos secret` command family.
Secrets must be declared before they can be set, read, injected, or resolved with `!secret`.

## Related Skills

| Need | Load |
|---|---|
| Store-backed non-secret values | [atmos-stores](../atmos-stores/SKILL.md) |
| YAML `!secret` and `!store` functions | [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) |
| Auth identity for secret backend access | [atmos-auth](../atmos-auth/SKILL.md) |
| Local Vault/OpenBao testing | [atmos-emulator](../atmos-emulator/SKILL.md) |

## Declare Secrets

Declare secrets under component `secrets.vars` and keep Terraform inputs secret-aware:

```yaml
components:
  terraform:
    app:
      secrets:
        vars:
          DATADOG_API_KEY:
            description: Datadog API key
            store: prod/ssm
            required: true
      vars:
        datadog_api_key: !secret DATADOG_API_KEY
```

`store:` and `sops:` can be a YAML function such as `!aws.cloudformation.output <component> <stack> <Output>`.
It is resolved lazily, only for the declaration a command uses, so an unresolvable selector (for example,
an undeployed producer) fails just that declaration while literal-backend siblings keep working.
`atmos secret list` never evaluates selectors: it shows status `unresolved` and a reason until `--verify`.
`atmos secret validate` fails if a `sops:` selector cannot be resolved, because collisions cannot be ruled out.

Supported backend families include AWS SSM, AWS Secrets Manager, HashiCorp Vault, Azure Key Vault,
GCP Secret Manager, 1Password, and SOPS-encrypted files.

## Commands

| Command | Purpose |
|---|---|
| `atmos secret list -s <stack> -c <component>` | List declared secrets and status |
| `atmos secret init -s <stack> -c <component>` | Provision/rotate declared secrets interactively |
| `atmos secret set <name> -s <stack> -c <component>` | Set a declared secret |
| `atmos secret get <name> -s <stack> -c <component>` | Retrieve a declared secret, masked by default |
| `atmos secret import -s <stack> -c <component>` | Bring existing backend secrets under management |
| `atmos secret pull -s <stack> -c <component>` | Download declared secrets to a local file |
| `atmos secret push -s <stack> -c <component>` | Upload declared secrets from a local file |
| `atmos secret shell -s <stack> -c <component>` | Start a shell with secrets in environment |
| `atmos secret exec -s <stack> -c <component> -- <cmd>` | Run one command with secrets injected |
| `atmos secret validate -s <stack> -c <component>` | CI gate for required initialized secrets |
| `atmos secret keygen ...` | Generate key material for supported vault backends |

Use `--identity` when backend access needs an Atmos Auth identity. `--identity=false` disables Atmos
authentication entirely: a store pinned to an `identity:` then uses ambient credentials, and Atmos warns
once per store. Without an interactive terminal, `atmos secret set` on an existing value and
`atmos secret delete` need `--force`. Use `--type` to disambiguate
component kinds.

## Safety Rules

- Keep secrets declared, not scattered through raw `!store` calls.
- Prefer `!secret` for sensitive values and `!store`/`!store.get` for non-sensitive shared data.
- Leave `--mask` enabled for normal operations; masked inspection should not leak values.
- Use `atmos secret validate` in CI before plan/apply workflows that require secrets.
- Never commit pulled local secret files unless they are intentionally SOPS-encrypted and approved
  by repository policy.
