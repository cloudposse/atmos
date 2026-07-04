---
title: Container Steps
tags: [Automation]
---

# Example: Container Step

Build, push, and run containers from Atmos workflows and custom commands.

## Container runtime

Global container-runtime defaults live under the top-level `container:` namespace in
`atmos.yaml`:

```yaml
container:
  runtime:
    provider: podman    # docker | podman (default: auto-detect docker, then podman)
    auto_start: true    # auto-init/start the Podman machine when no running runtime is found
```

This example sets `auto_start: true` so it works out of the box on macOS/Linux. The same
controls exist per step (`provider:`, `runtime_auto_start:`) and via the
`ATMOS_CONTAINER_RUNTIME_AUTO_START` env var (which overrides config).

## Try It

```shell
cd examples/container-step

# Custom command using a container step. `example` is a user-defined custom
# command (not a built-in `atmos` subcommand) — the name makes that obvious.
atmos example hello

# Build a local image, render its metadata (inspect step), then run it
atmos example build-run
atmos workflow build-run -f container-step

# Build with Docker Buildx Bake and run the result. Requires Docker Buildx.
atmos workflow bake-build-run -f container-step

# Workflow using a run action
atmos workflow hello -f container-step

# Workflow-level container sandbox shared by shell steps
atmos workflow workflow-container -f container-step

# Show workspace and environment behavior
atmos workflow workspace -f container-step
atmos workflow env -f container-step

# Requires a local registry listening on localhost:5000
atmos workflow push-local-registry -f container-step

# Intentional non-zero exit for scanner/linter style workflows
atmos workflow failing-check -f container-step

# Optional interactive shell
atmos workflow shell -f container-step
```

## Pushing to a Private Registry (ECR) with an Identity

Container steps don't implement registry login themselves. Set `identity:` on the
step (or pass `--identity`) and Atmos's auth integrations materialize the
credentials: authenticating the identity auto-provisions any linked
`auto_provision` integration, and the `aws/ecr` integration performs the Docker
login the push then uses.

The `auth:` block in `atmos.yaml` links the `dev-admin` identity to an `aws/ecr`
integration. Replace the provider, account, and region with your own, then:

```shell
# Requires AWS access and a real ECR registry (see the auth: block in atmos.yaml).
atmos workflow push-ecr -f container-step --identity dev-admin
```

The flow: `identity` → authenticate → `aws/ecr` integration logs in to ECR → the
`push` step's `docker push` uses that login. No explicit `docker login` step is
needed. See [Auth & Integrations](https://atmos.tools/cli/configuration/auth) and
[`atmos aws ecr login`](https://atmos.tools/cli/commands/aws/ecr-login).

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Custom commands, workflow base path, and the ECR `auth:` example |
| `Dockerfile` | Tiny image used by the build/run examples |
| `docker-bake.hcl` | Docker Buildx Bake build definition |
| `workflows/container-step.yaml` | Workflow examples for container steps and workflow-level container sandboxes |
