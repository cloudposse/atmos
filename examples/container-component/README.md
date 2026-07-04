---
title: Container Components
tags: [Components]
cast:
  file: /casts/examples/container-component/lifecycle.cast
  title: atmos container component lifecycle
---

# Container Components

This example demonstrates the **`container` component kind**: stack-scoped,
Atmos-native, **persistent** containers. One component is one service. Atmos owns
the image artifact (build/push/pull) and a long-running named container lifecycle
(up/ps/logs/exec/restart/stop/rm/down), discovered by labels derived from the
canonical component instance address — not from local state files.

> This is different from the ephemeral [`type: container` step](../container-step)
> (`docker run --rm`, workflow-scoped). The component is declarative, addressable
> infrastructure; the step is procedural sequencing.

## Layout

```text
atmos.yaml                      # container.runtime + compositions
Dockerfile                      # image for the `worker` component
stacks/deploy/dev.yaml          # container.api + container.worker
```

Two container components are defined in the `dev` stack:

- **`api`** — a long-running web service from the public `nginx:alpine` image,
  published on `localhost:8080`.
- **`worker`** — a background worker built from the local `Dockerfile`.

Both declare `composition: storefront`.

## Lifecycle

```shell
# Build the worker image from the local Dockerfile.
atmos container build worker -s dev

# Start the long-running containers.
atmos container up api -s dev
atmos container up worker -s dev

# See which containers are running (look for the ● running indicator).
atmos container list

# Inspect and operate the running containers (discovered by label).
atmos container ps api -s dev
atmos container logs worker -s dev
atmos container exec api -s dev -- sh -c 'nginx -v'

# Tear down (stop + remove).
atmos container down api -s dev
atmos container down worker -s dev

# Now they show as stopped.
atmos container list
```

## Runtime name and labels

The `api` component in the `dev` stack maps to:

```text
instance:   dev/container/api
name:       atmos-dev-container-api
labels:     tools.atmos.stack=dev
            tools.atmos.component_type=container
            tools.atmos.component=api
            tools.atmos.instance=dev/container/api
```

All lifecycle commands discover the container by these labels, so there is no
local state file to lose or corrupt.

## Compositions

`atmos.yaml` declares a `storefront` composition with services `api`, `worker`,
and `database`. The first two are fulfilled by components in this stack;
`database` is declared but not provided here — which is allowed (membership is a
closed contract, but fulfillment is open). Declaring `composition:` for a service
**not** listed under `compositions.storefront.services` is a hard error.

## Requirements

A working Docker or Podman runtime. With Podman, `container.runtime.auto_start`
in `atmos.yaml` initializes/starts the Podman machine automatically.
