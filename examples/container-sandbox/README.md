---
title: Container Sandbox
tags: [DX]
---

# Example: Container Sandbox

Run an entire Atmos workflow inside one shared container sandbox.

Shell steps inherit the workflow-level `container:` configuration and execute in
the same long-lived container. The example writes a file in one containerized
step, reads it from another, opts one step out to run on the host, then cleans up
the generated file.

## Try It

```shell
cd examples/container-sandbox

# Preview the sandbox container and exec commands
atmos workflow sandbox -f container-sandbox --dry-run

# Run the workflow. Requires Docker or Podman.
atmos workflow sandbox -f container-sandbox
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Configures the workflow base path |
| `workflows/container-sandbox.yaml` | Workflow-level container sandbox example |
