---
name: atmos-emulator
description: "Atmos emulator components: local AWS/GCP/Azure/Kubernetes/Vault/OpenBao/registry emulators, components.emulator, !emulator, identities, persistence, health checks, and emulator commands"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Emulators

Use this skill for local emulators managed by Atmos. Emulators are stack-scoped, persistent
containers that stand in for cloud APIs, Kubernetes, Vault/OpenBao, or OCI registries during local
development and testing.

## Related Skills

| Need | Load |
|---|---|
| Container component lifecycle | [atmos-container](../atmos-container/SKILL.md) |
| YAML `!emulator` references | [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) |
| Secrets backed by Vault/OpenBao/SOPS/cloud stores | [atmos-secrets](../atmos-secrets/SKILL.md) |
| Workflow `emulator` and `wait` steps | [atmos-workflows](../atmos-workflows/SKILL.md) |

## Component Shape

Define emulators under `components.emulator`:

```yaml
components:
  emulator:
    aws:
      driver: localstack/aws
      region: us-east-1
      ephemeral: false
      container:
        healthcheck:
          test: ["CMD", "awslocal", "s3", "ls"]
```

Supported driver families include AWS, GCP, Azure, Kubernetes (`k3s`), Vault/OpenBao, and registry
emulators. Check the local docs for exact driver names before generating config for a specific
driver.

## Commands

| Command | Purpose |
|---|---|
| `atmos emulator up <name> -s <stack>` | Start an emulator |
| `atmos emulator down <name> -s <stack>` | Stop and remove an emulator |
| `atmos emulator reset <name> -s <stack>` | Stop and wipe persisted state |
| `atmos emulator ps -s <stack>` | List configured emulators that are running |
| `atmos emulator list -s <stack>` | List configured emulators and status |
| `atmos emulator logs <name> -s <stack>` | Show emulator logs |
| `atmos emulator exec <name> -s <stack> -- <cmd>` | Run a command in the emulator container |

Use `--dry-run` to preview.

`list` and `ps` are configuration-scoped: emulator components declared in the
current Atmos project are the inventory, and the container runtime only supplies
their status. Use `--runtime` on either command only when diagnosing raw labeled
containers outside the current project configuration. Lifecycle commands prompt
for a stack on an interactive TTY when `-s` is omitted.

## YAML Integration

Use `!emulator` to resolve connection details from stack config instead of hardcoding localhost
ports. This keeps components portable across stacks, CI, and local developer machines.

## Guidance

- Use emulators for local integration tests and agent workflows that need cloud-like APIs without
  cloud credentials.
- Keep emulator state persistent only when tests or development workflows benefit from it.
- Prefer `wait`/`wait-all` workflow steps or health checks before running dependent commands.
- Use local registry emulators when testing container build/push flows.
- Use Vault/OpenBao emulators together with `atmos-secrets` for local secret workflows.
