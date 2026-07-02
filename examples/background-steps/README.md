# Background container services

This example shows how to start a long-running container service in the background,
run work against it, and tear it down — all from a workflow.

A container step with `background: true` starts detached and the workflow continues
to the next step. Atmos reuses the existing container lifecycle to supervise it:

- **Readiness** reuses the container `healthcheck:` (under `with:`). When a health
  check is configured, Atmos blocks until the service is **healthy** before the next
  step (the implicit readiness gate). An explicit `{type: wait, for: [name]}` (or
  `{type: wait-all}`) does the same on demand.
- **Teardown** is an explicit `{type: cancel, for: [name]}` step (stop + remove).
  Anything still running when the workflow ends is **auto-torn-down** — a service
  never exits on its own, so it is never "waited to exit".

> Requires a container runtime (Docker or Podman).

## Run it

```shell
cd examples/background-steps

# Start nginx in the background, wait until healthy, use it, then cancel it.
atmos workflow service -f background

# Start redis + nginx in the background, wait-all, then tear both down.
atmos workflow fanout -f background
```

## How it maps to the syntax

```yaml
- name: cache
  type: container
  action: run
  background: true          # start detached, keep going
  with:                     # all container params live under `with:`
    image: nginx:alpine
    healthcheck:            # the readiness gate (reuses the container health check)
      test: ["CMD-SHELL", "wget -q -O /dev/null http://localhost/ || exit 1"]
      interval: 2s
      retries: 10
- name: use-service
  type: shell
  command: ...              # runs only after `cache` is healthy
- type: cancel
  for: cache                # graceful teardown
```

See the [`parallel`](../parallel-steps) example for the complementary *structured*
concurrency (`parallel`/`matrix`) control steps.
