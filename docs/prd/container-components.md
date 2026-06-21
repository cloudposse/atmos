# PRD: Container Components

## Summary

Add a `container` component kind to Atmos. A container component is a stack-scoped, Atmos-native
container: one component is one service. It owns a container image artifact (build/push/pull) and an
optional long-running container lifecycle (up/ps/logs/exec/restart/stop/rm/down), discovered by labels
derived from the component instance address.

A `container` component is the per-service, Atmos-native building block. A set of container components
grouped by a [composition](compositions.md) is effectively "our own Compose" — Atmos orchestrates a
multi-container local system with no `compose.yaml`. When you already have a native Compose project,
use a [compose component](compose-components.md) instead.

> A component is one unit. To run a *set* of units together as a system, see
> [compositions](compositions.md).

## Goals

- Provide a first-class `container` component kind, addressable like any other component instance.
- Build, push, and pull a component's image with Docker or Podman.
- Run a component as a one-shot foreground process (`run`) or a long-running named container (`up`).
- Discover and operate managed containers by labels derived from the canonical component instance
  address, not from local state files.
- Reuse the existing `pkg/container` runtime foundation.
- Participate in compositions via the first-class `composition` membership field.

## Non-Goals

- V1 does not model multi-service projects in one component — that is a
  [compose component](compose-components.md) or a [composition](compositions.md) of container
  components.
- V1 does not replace the procedural [`type: container` step](container-actions-and-step-outputs.md);
  the step is ephemeral and workflow-scoped, the component is stack-scoped and long-lived.
- V1 does not introduce a daemonless OCI path; build/push/pull use Docker/Podman.

## Public Interface

`image`, `build`, and `run` are **first-class component sections** — siblings of `composition`/`env`/
`metadata`, **not** nested under `vars` (which is reserved for arbitrary template variables). The
`build`/`run` shapes are the same structs as the [`type: container` workflow step](container-actions-and-step-outputs.md)
(`ContainerBuildStep`/`ContainerRunStep`), so component and step configuration stay consistent. Ports
and mounts are structured. Container application env comes from the component `env:` section (resolved
with secrets), not from `run`.

```yaml
components:
  container:
    api:
      composition: storefront            # first-class membership (see compositions.md)
      image: "localhost:5001/api:{{ .git.sha }}"

      build:
        context: app
        dockerfile: Dockerfile
        tags:
          - "localhost:5001/api:{{ .git.sha }}"

      run:
        command: ./api
        ports:
          - host: 8080
            container: 8080
        mounts:
          - source: .
            target: /workspace

      secrets:
        vars:
          NPM_TOKEN:
            store: app-secrets
            required: true
      env:
        PORT: "8080"
        NPM_TOKEN: !secret NPM_TOKEN
```

Component config is the canonical home for component-scoped secrets, env, and identity/auth — exactly
like other component kinds. Inheritance, catalogs, and deep-merge apply normally (`metadata.inherits`
deep-merges `build`/`run`/`image` from abstract base components).

## Lifecycle

```bash
atmos container build api -s dev
atmos container push api -s dev
atmos container pull api -s dev
atmos container run api -s dev
atmos container up api -s dev
atmos container ps api -s dev
atmos container logs api -s dev
atmos container exec api -s dev -- sh
atmos container restart api -s dev
atmos container stop api -s dev
atmos container rm api -s dev
atmos container down api -s dev
```

- `build` / `push` / `pull` operate the image artifact.
- `run` is a foreground one-shot execution using `vars.run`.
- `up` creates or starts a named long-running container.
- `down` is `stop` plus `rm`.

## Component Instance Identity

Atmos identifies a component instance as the combination of stack, component kind, and component name.
Container components use that canonical identity for runtime discovery:

```text
<stack>/<component_type>/<component>
```

Example:

```text
dev/container/api
```

The runtime container name is a sanitized projection of the component instance address:

```text
atmos-dev-container-api
```

Runtime labels preserve the canonical fields:

```text
com.cloudposse.atmos.stack=dev
com.cloudposse.atmos.component_type=container
com.cloudposse.atmos.component=api
com.cloudposse.atmos.instance=dev/container/api
```

Lifecycle commands such as `ps`, `logs`, `exec`, `restart`, `stop`, `rm`, and `down` discover
containers by these labels instead of relying on separate state files.

## Relationship to the Container Step

The [`type: container` step](container-actions-and-step-outputs.md) and the `container` component are
complementary, not redundant:

| Aspect       | Container step                          | Container component                     |
|--------------|-----------------------------------------|-----------------------------------------|
| Lifetime     | Ephemeral (`docker run --rm`)           | Stack-scoped, long-lived                |
| Scope        | Workflow / custom command               | Stack component instance                |
| Identity     | Step name                               | `<stack>/container/<name>` + labels     |
| Secrets      | Explicit step env                       | Component `secrets`/`env`               |
| Discovery    | None (runs and exits)                   | Label-based                             |

The step is procedural sequencing; the component is declarative, addressable infrastructure.

## Implementation Notes

- Register `container` in `schema.Components` and `Components.GetComponentConfig`.
- Reuse `pkg/container` runtime for build, push, pull, create, start, exec, logs, and remove.
- Add a long-running named-lifecycle path with label-based discovery, distinct from the step's
  ephemeral runner (`RunEphemeralContainer`).
- Derive runtime names and labels from the canonical component instance address.
- Resolve the first-class `composition` membership field via normal stack processing (see
  [compositions.md](compositions.md)).

## Test Plan

- Schema decode tests for the `container` component kind and `composition` field.
- Deterministic runtime-name generation and canonical label construction.
- Label-based discovery for `ps`/`logs`/`exec`/`down`.
- Lifecycle verb tests: `build`, `push`, `pull`, `run`, `up`, `down`.
- Docker and Podman argument construction tests.
- Runtime-gated integration test with a tiny image.
