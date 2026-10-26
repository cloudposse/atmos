# PRD: Native Container Actions, Step Outputs, Workflows, and Custom Commands

## Summary

Add first-class container lifecycle support to Atmos workflows and custom commands through the shared step library. `type: container` is a container action family with namespaced `build`, `push`, and `run` blocks.

Also formalize step outputs so one step can build an image, later steps can push it, and subsequent steps can run or deploy the exact produced artifact.

## Goals

- Run containerized tools inline from workflows and custom commands.
- Build, push, and run images with Docker or Podman.
- Reuse the existing Atmos container/devcontainer runtime foundation.
- Expose declared step outputs through `{{ .steps.<name>.outputs.<key> }}`.
- Keep existing `{{ .steps.<name>.value }}` references working.
- Preserve flat `type: container` run fields as backward-compatible shorthand.

## Non-Goals

- V1 does not include `action: build-run`.
- V1 does not persist outputs across separate Atmos invocations.
- V1 does not implement daemonless OCI registry APIs; push uses Docker/Podman.
- V1 does not require users to configure `devcontainer:` blocks.
- V1 does not support Docker Buildx Bake through Podman. Bake is Docker Buildx-backed.
- Container steps do not model long-running named services. Persistent container lifecycle belongs to [container components](container-components.md) and [compositions](compositions.md).

## Public Interface

### Step Outputs

```yaml
steps:
  - name: build
    type: container
    action: build
    build:
      context: .
      tags:
        - 123456789012.dkr.ecr.us-east-2.amazonaws.com/app:{{ .env.GIT_SHA }}
    outputs:
      image: "{{ .metadata.image }}"

  - name: push
    type: container
    action: push
    identity: dev-admin
    push:
      image: "{{ .steps.build.outputs.image }}"
    outputs:
      image: "{{ .metadata.image }}"
      digest: "{{ .metadata.digest }}"

  - name: run
    type: container
    action: run
    run:
      image: "{{ .steps.push.outputs.image }}"
      command: uname -a
```

Every named step exposes:

- `value`
- `values`
- `metadata`
- `outputs`
- `skipped`
- `error`

Command-like steps expose standard metadata:

- `stdout`
- `stderr`
- `exit_code`

### Container Step

```yaml
type: container
action: build | push | run
```

Build:

```yaml
- name: build
  type: container
  action: build
  build:
    runtime: docker
    runtime_auto_start: false
    engine: buildx
    context: .
    dockerfile: Dockerfile
    tags:
      - app:local
    build_args:
      VERSION: "{{ .env.VERSION }}"
    target: runtime
    no_cache: false
    pull: false
```

Docker Buildx Bake:

```yaml
- name: build
  type: container
  action: build
  build:
    runtime: docker
    engine: buildx
    tags:
      - ghcr.io/acme/app:{{ .env.GIT_SHA }}
    bake:
      file: docker-bake.hcl
      target: app
      vars:
        TAG: ghcr.io/acme/app:{{ .env.GIT_SHA }}
      load: true
```

Push:

```yaml
- name: push
  type: container
  action: push
  identity: dev-admin
  push:
    runtime: docker
    runtime_auto_start: false
    image: app:local
    tags:
      - ghcr.io/acme/app:{{ .env.GIT_SHA }}
```

Run:

```yaml
- name: smoke
  type: container
  action: run
  run:
    runtime: docker
    runtime_auto_start: false
    image: ghcr.io/acme/app:{{ .env.GIT_SHA }}
    command: uname -a
    shell: /bin/sh
    pull: missing
    workspace: /workspace
    workspace_read_only: false
    cleanup: always
    user: "1000:1000"
    run_args: []
    mounts: []
    ports: []
```

Compatibility:

```yaml
- name: hello
  type: container
  image: alpine:latest
  command: echo hello
```

This remains valid shorthand for `action: run`.

### Related: stack-scoped components

Container steps are procedural workflow actions — ephemeral, like `docker run --rm`. They are the
right tool for inline build/push/run inside a workflow or custom command.

For long-running, stack-scoped containers with their own identity and label-based discovery, use a
[container component](container-components.md). For an existing native Compose project, use a
[compose component](compose-components.md). To operate a set of components together as a system, see
[compositions](compositions.md).

### Planned: `type: compose` step

A sibling `type: compose` step is planned for this step library — the procedural, inline counterpart of
the [compose component](compose-components.md), for operating a Compose project as a workflow step
(e.g. spin up ephemeral test dependencies, run tests, tear down). It will mirror the `type: container`
shape with `action` (`up`/`down`/`ps`/`logs`/`exec`/`restart`/`build`/`pull`/`config`), `files`,
`project_name`, `env`, and Docker/Podman runtime detection. Specified here when implemented; see
[compose components](compose-components.md) for the declarative, stack-scoped counterpart.

## Behavior

- `build` uses Docker/Podman runtime build.
- `build.engine: buildx` uses Docker Buildx and requires `runtime: docker` in V1.
- `build.bake` invokes `docker buildx bake`; if `runtime: podman`, validation fails with a clear error.
- `push` uses Docker/Podman runtime push.
- `run` uses the one-shot lifecycle: pull if needed, create, start, exec, remove.
- `identity` is the canonical per-step auth field.
- Registry auth flows through prepared identity environment and runtime credential stores.
- Podman machine init/start is opt-in via `runtime_auto_start: true`.
- Default workspace mount is read-write and maps the resolved working directory to `/workspace`.
- Podman builds use `podman build`, `podman tag`, and `podman push`; Bake can be added later if Podman/Buildah gains native support.
- Standalone container steps consume explicit env and step context; they do not pretend to be stack component instances. Component-scoped secrets belong on component config (see [container components](container-components.md)).

## Implementation Notes

- Extend shared workflow/custom-command schema with `outputs`, `action`, and `build`/`push`/`run` blocks.
- Evaluate declared outputs after successful handler execution.
- Store step results for registered step handlers.
- Add runtime build metadata, tag, push, and image inspection helpers.
- Add Docker Buildx Bake as a first-class build mode while keeping the generic Docker/Podman build path separate.
- Keep persistent devcontainer commands separate from inline container actions.

## Test Plan

- Schema decode tests for outputs and container action blocks.
- Task/workflow conversion tests for all new fields.
- Output evaluation tests for `value`, `metadata`, `stdout`, `stderr`, `exit_code`, and prior step references.
- Validation tests for missing action-specific fields.
- Docker and Podman arg construction tests for build, tag, push, and run.
- Docker Buildx Bake arg construction tests, plus validation that Podman Bake is rejected.
- Runtime lifecycle tests for build -> push -> run propagation.
- Runtime-gated integration test with a tiny image and optional local registry fixture.
