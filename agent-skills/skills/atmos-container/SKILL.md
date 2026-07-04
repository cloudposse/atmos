---
name: atmos-container
description: "Atmos container components: components.container, build/run/push/pull/up/down/list/ps/logs/exec, stack-scoped persistent containers, container workflow steps, compositions, and hooks"
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

## Operational Guidance

- Use stack names to isolate container instances.
- Prefer declared `image`, `build`, `run`, and `env` blocks over ad hoc shell `docker` commands.
- Use `composition` when a container fulfills a named service in a system.
- Use hooks for pre/post actions such as scans, artifact publication, or store writes.
- Use registry auth skills such as `atmos-aws-ecr` when pushing to private registries.
