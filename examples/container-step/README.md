# Example: Container Step

Build, push, and run containers from Atmos workflows and custom commands.

## Try It

```shell
cd examples/container-step

# Custom command using a container step
atmos container hello

# Build a local image and run it in a later step
atmos container build-run
atmos workflow build-run -f container-step

# Build with Docker Buildx Bake and run the result. Requires Docker Buildx.
atmos workflow bake-build-run -f container-step

# Workflow using a run action
atmos workflow hello -f container-step

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

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Custom commands and workflow base path |
| `Dockerfile` | Tiny image used by the build/run examples |
| `docker-bake.hcl` | Docker Buildx Bake build definition |
| `workflows/container-step.yaml` | Workflow examples for container steps |
