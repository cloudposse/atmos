---
name: atmos-container
description: "Atmos container components: components.container, Docker Compose migration, build/run/push/pull/up/down/list/ps/logs/exec, stack-scoped persistent containers, container workflow steps, compositions, and hooks"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Container Components

Use this skill for first-class Atmos containers. A container component is a stack-scoped service:
one component maps to one container, with image build/push/pull and optional persistent runtime.

## Related Skills

| Need | Load |
|---|---|
| Grouping services into systems | [atmos-compositions](../atmos-compositions/SKILL.md) |
| Workflow `container` steps | [atmos-workflows](../atmos-workflows/SKILL.md) |
| Lifecycle hooks around components | [atmos-hooks](../atmos-hooks/SKILL.md) |
| Local cloud/API emulators | [atmos-emulator](../atmos-emulator/SKILL.md) |
| Secret and env migration | [atmos-secrets](../atmos-secrets/SKILL.md) |

## Component Shape

Define containers under `components.container` in stack manifests.

```yaml
components:
  container:
    api:
      image: ghcr.io/acme/api:latest
      build:
        context: services/api
        dockerfile: Dockerfile
        tags:
          - ghcr.io/acme/api:latest
      run:
        ports:
          - host: 8080
            container: 8080
        command: ./api
      env:
        LOG_LEVEL: info
      composition: app
```

Container components can participate in hooks, compositions, workflows, and stack-specific config
the same way other Atmos component types do.

## Commands

| Command | Purpose |
|---|---|
| `atmos container build <name> -s <stack>` | Build the component image |
| `atmos container push <name> -s <stack>` | Push the image to its registry |
| `atmos container pull <name> -s <stack>` | Pull the image |
| `atmos container run <name> -s <stack>` | Run one foreground container |
| `atmos container up <name> -s <stack>` | Create/start the persistent container |
| `atmos container down <name> -s <stack>` | Stop and remove the persistent container |
| `atmos container ps -s <stack>` | Show running state |
| `atmos container list -s <stack>` | List container components and state |
| `atmos container logs <name> -s <stack>` | Show logs |
| `atmos container exec <name> -s <stack> -- <cmd>` | Execute inside the container |

`atmos container` also supports `attach`, `restart`, `start`, `stop`, and `rm`. Use `--dry-run` to
preview operations.

## Workflow Steps

Use the workflow `container` step type when a workflow should build, run, push, or operate a
container as part of orchestration. Use `components.container` when the container is a reusable
stack-scoped component with persistent lifecycle.

## Migrating from Docker Compose

When replacing `docker compose` with Atmos containers, translate one Compose service at a time
into `components.container.<service>`. Keep multi-service grouping with `composition`, not by
collapsing several services into one container component.

| Docker Compose field | Atmos container mapping |
|---|---|
| `services.<name>.image` | `components.container.<name>.image` |
| `build.context`, `build.dockerfile` | `build.context`, `build.dockerfile` |
| `ports` | `run.ports` |
| `environment`, `env_file` | `env`, stack `vars`, or `secrets.vars` with `!secret` |
| `command`, `entrypoint` | `run.command` or the supported runtime command fields |
| `volumes` | runtime mount settings supported by the container component |
| `depends_on` | workflow/composition ordering, readiness checks, `wait`, or `wait-all` |
| Compose project name | shared `composition: <name>` across related container components |

Migration process:

1. Inventory Compose services and split long-lived services into separate container components.
2. Move shared `.env` values into stack vars, component env, declared secrets, or `!secret`
   references.
3. Use `composition: <name>` so former Compose services validate and run as one system.
4. Replace `docker compose up/down/logs/exec/ps` with the matching `atmos container` commands.
5. Use workflow `container`, `wait`, `wait-all`, and explicit dependencies for startup order
   instead of Compose-only `depends_on` assumptions.
6. Prefer first-class `components.container` for Atmos-managed services. Keep a native Compose
   file only when the project must remain compatible with external Compose tooling.

## Operational Guidance

- Use stack names to isolate container instances.
- Prefer declared `image`, `build`, `run`, and `env` blocks over ad hoc shell `docker` commands.
- Use `composition` when a container fulfills a named service in a system.
- Use hooks for pre/post actions such as scans, artifact publication, or store writes.
- Use registry auth skills such as `atmos-aws-ecr` when pushing to private registries.
