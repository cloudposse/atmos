# PRD: Compose Components

## Summary

Add a `compose` component kind to Atmos. A compose component wraps an existing native
`compose.*.yaml` project (one or more services, e.g. registry + postgres + k3s) as a single Atmos
component. It is the bring-your-own / ecosystem-compatible sibling of the
[container component](container-components.md): Atmos does not reinvent Compose, it runs the project
and layers Atmos config (env/vars interpolation, secrets, identity/auth, and composition membership)
on top.

> A component is one unit. To run a *set* of units together as a system, see
> [compositions](compositions.md).

## Goals

- Operate a native Docker/Podman Compose project as a first-class Atmos component.
- Keep the Compose file 100% native — Atmos never rewrites Compose syntax.
- Layer Atmos config on top: variable interpolation, env, secrets, identity, inheritance/catalogs.
- Provide a deterministic, environment-scoped project name by convention.
- Participate in compositions via the first-class `composition` membership field.

## Non-Goals

- V1 does not surface Docker Compose `profiles` as an Atmos field or flag (see Deferred). Native
  `profiles:` inside the user's Compose file still work — Atmos just runs the project.
- V1 does not translate Compose into another backend; a compose component always runs through the
  Compose CLI.
- V1 does not model per-service Atmos identity for services inside the Compose file; the component is
  the unit of Atmos addressing.

## Public Interface

```yaml
components:
  compose:
    local-infra:
      composition: storefront            # first-class membership (see compositions.md)
      metadata:
        inherits: [compose-defaults]     # catalogs / inheritance like any component
      files:                             # native compose file(s), untouched
        - compose.local.yaml
      project_name: storefront           # OPTIONAL; default = sanitized stack name (see below)
      env_file:
        - .env.local
      vars:                              # interpolated into compose ${...}
        POSTGRES_VERSION: "16"
      env:
        POSTGRES_PASSWORD: !secret pg_password
```

The referenced `compose.local.yaml` stays native Compose:

```yaml
# compose.local.yaml
services:
  registry:
    image: registry:2
    ports: ["5001:5000"]
  postgres:
    image: "postgres:${POSTGRES_VERSION}"
    environment:
      POSTGRES_PASSWORD: "${POSTGRES_PASSWORD}"
    ports: ["5432:5432"]
  k3s:
    image: rancher/k3s:latest
    privileged: true
    ports: ["6443:6443"]
```

## Lifecycle

```bash
atmos compose up local-infra -s local
atmos compose ps local-infra -s local
atmos compose logs local-infra postgres -s local
atmos compose exec local-infra postgres -s local -- psql
atmos compose restart local-infra -s local
atmos compose down local-infra -s local
```

Verb → Compose subcommand mapping:

| atmos verb         | compose command              |
|--------------------|------------------------------|
| `up` / `deploy`    | `compose up -d`              |
| `down` / `destroy` | `compose down` (`--volumes`) |
| `ps`               | `compose ps`                 |
| `logs`             | `compose logs [service]`     |
| `exec`             | `compose exec <svc> -- …`    |
| `restart`          | `compose restart [service]`  |
| `build` / `pull`   | `compose build` / `pull`     |
| `config`           | `compose config` (render)    |

These verbs match the [composition](compositions.md) fan-out so a compose component operates the same
whether invoked standalone or as a composition member.

## Project Name Convention

`project_name` is a composed convention: derived by default and overridable.

- **Default = the sanitized stack name** (lowercase, `[a-z0-9_-]`, e.g. `plat/ue2-dev` →
  `plat-ue2-dev`).
- This makes the Compose project equal to the environment, so `compose ps` / `logs` show the whole
  local system for that stack together.
- **Tradeoff**: multiple compose components (or compositions) in one stack share that project
  namespace — usually desirable for local dev. Set `project_name` explicitly to separate them.

Container components stay per-component (`atmos-<stack>-container-<name>`) because each is one service;
the compose project groups at the environment level because Compose owns multi-service grouping
natively.

## `container` vs `compose`

| | `container` component | `compose` component |
|---|---|---|
| Unit | One Atmos-native service | A native multi-service Compose project |
| Lifecycle owner | Atmos (labels, named lifecycle) | Compose CLI |
| When to use | Atmos-native per-service control | You already have a `compose.yaml` |
| Grouping | A composition of containers | The Compose project itself |

Both are ordinary stack components with the same component model, membership, and composition fan-out.

## Compose Step (addition to the step library)

Alongside the `compose` component kind, a procedural `type: compose` step **will be added to the shared
step library** — the inline, ephemeral counterpart, mirroring how `type: container` complements the
[container component](container-components.md). It is for operating a Compose project as a step inside a
workflow or custom command (e.g. spin up ephemeral test dependencies, run tests, tear down), without
modeling it as a stack component.

```yaml
steps:
  - name: deps-up
    type: compose
    action: up
    files: [compose.test.yaml]
    project_name: itest

  - name: integration
    type: shell
    command: go test ./it/...

  - name: deps-down
    type: compose
    action: down
    files: [compose.test.yaml]
    project_name: itest
```

The step mirrors the component verb mapping (`up`/`down`/`ps`/`logs`/`exec`/`restart`/`build`/`pull`/
`config`) and the same Docker/Podman runtime detection. Because it is a step, it belongs in the shared
step library — its full field reference will be specified in
[Container Actions, Step Outputs, Workflows, and Custom Commands](container-actions-and-step-outputs.md),
next to `type: container`, not in this component PRD. This keeps the procedural step and the declarative
component as separate, complementary surfaces.

| Surface              | Shape                          | Use                                   |
|----------------------|--------------------------------|---------------------------------------|
| `type: compose` step | Procedural, inline, ephemeral  | Workflow/custom-command Compose ops   |
| `compose` component  | Declarative, stack-scoped      | Composition member, `atmos compose …` |

## Implementation Notes

- Register `compose` in `schema.Components` and `Components.GetComponentConfig` (sibling of
  terraform/helmfile/packer/ansible/container; may begin via the `Plugins` `",remain"` map).
- Back the kind with a `pkg/compose` package; re-home the existing `composeArgs()` and
  `DetectRuntimeWithPreferenceAndRecovery` from `pkg/composition/service.go`.
- Resolve `vars`/`env` (incl. secrets) into Compose interpolation env and `--env-file`.
- Default `project_name` to the sanitized stack name when unset.
- Resolve the first-class `composition` membership field via normal stack processing.

## Deferred

- **Docker Compose profiles**: not surfaced as an Atmos field or flag in V1. "Profile" conflicts with
  the Atmos / Atmos Pro `--profile` (`ATMOS_PROFILE`) config-profile concept. Native compose-file
  `profiles:` still work inside the user's Compose file. A general
  [labels](compositions.md#deferred) concept is the likely future home for this kind of selection.

## Test Plan

- Schema decode tests for the `compose` component kind and `composition` field.
- Verb → Compose argument construction tests for Docker and Podman.
- `project_name` default sanitization tests (stack name → valid project name).
- Variable / env / secret interpolation into Compose invocation.
- Runtime-gated local smoke test bringing up and tearing down a tiny Compose project.
