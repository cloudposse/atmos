# PRD: Atmos Compositions

## Summary

Introduce `compositions:` as a top-level Atmos primitive that groups the components making up a system
so they can be operated together. A composition declares the **authoritative superset** of services a
system can be made of; each stack fulfills whatever **subset** it actually provides.

A composition is membership + identity only. It does not own environment variation (that is
**stacks**), build/deploy/run mechanics (that is **components** such as
[container](container-components.md), [compose](compose-components.md), terraform, helmfile,
kubernetes, ecs), or sequencing (that is **workflows** and steps). It is the cross-section that ties
them together.

## Goals

- Define a system once as a set of member services, with no per-environment repetition.
- Let each environment fulfill a different subset across different component kinds.
- Operate a whole system with one interface: `atmos composition <verb> <name> -s <stack>`.
- Reuse existing Atmos primitives: stacks for environment variation, components for build/deploy/run,
  `dependencies.components` for ordering, workflows/steps for sequencing.
- Make membership a first-class component property, validated against an authoritative contract.

## Non-Goals

- V1 does not introduce per-target service graphs, `vars`, overrides, or a second deep-merge system.
- V1 does not make a composition a component kind.
- V1 does not implement deployments (promotion/audit); those reference `composition + stack` later.

## Design

### §A What a composition is

A composition declares all **possible** services (the env-agnostic universe). It is defined once and
carries no environment graph.

```yaml
compositions:
  storefront:
    description: Storefront web app, API, and database
    services:                 # all POSSIBLE services (the universe), env-agnostic
      - frontend
      - api
      - database
      - mock-stripe           # only some environments fulfill these
      - mock-s3
      - local-infra
```

### §B Membership is a first-class component property, resolved per stack

`composition` is a first-class per-component field — a sibling to `vars`, `env`, `settings`,
`metadata`, and `dependencies` — not a key under `settings`. It accepts a string or a list
(multi-membership). A component declares its membership where it is already configured:

```yaml
components:
  ecs:
    frontend:
      composition: storefront
    api:
      composition: storefront
      dependencies:                        # v2 ordering (settings.depends_on is DEPRECATED)
        components:
          - component: postgres
  terraform:
    postgres:
      composition: storefront
```

`atmos composition <verb> storefront -s <stack>` selects all components in that stack whose
`composition` includes `storefront`, ordered by `dependencies.components`.

Membership lives on the component (not as a fixed list on the composition) because the per-stack
selector is what makes asymmetry free: each stack answers "is this part of the app *here*?" in place,
with no override blocks. Service name = component name; a component may set an explicit `role` only
when its name differs from the declared service name.

### §C Asymmetry and validation (the contract)

The `services:` list is a **closed contract for membership, open for fulfillment**:

- **Unfulfilled declared service** — no component in a stack provides it → **allowed, never an error**
  by default. The list defines what is possible, not a per-stack requirement.
- **Unknown membership** — a component declares `composition` for a service (or composition name) not
  declared in `compositions.<name>.services`, or for a composition that does not exist → **HARD
  ERROR** at stack processing. The composition is not prepared to accept it.
- **Soft report** — `atmos composition validate <name> -s <stack>` lists fulfilled vs.
  not-provided-here services. A future `--strict` flag may require full fulfillment.

### §D Environment variation is stacks

Each service's shape is defined once in a catalog (abstract base components); each stack imports the
backing it wants:

- `dev` → `components.ecs.frontend`, `components.ecs.api`, `components.terraform.postgres`
- `staging` / `prod` → `components.kubernetes.frontend`, `...api`, `components.terraform.postgres`
- `local` → `components.compose.local-infra`, `components.container.frontend`, `...api`

`dev`/`staging`/`prod` being mostly identical is free via inheritance/deep-merge. A composition member
is referenced by name and resolves to whatever kind the target stack defines. Resolution rule:
component names are unique within a stack regardless of kind; ambiguity is an error. (This is the one
isolated accommodation for component kind being part of the component address; it is contained to the
resolver and removable if kind later becomes an attribute.)

### §E Lifecycle fan-out

Composition verbs dispatch to each member's native component lifecycle, in `dependencies.components`
order:

| verb     | container / compose          | terraform | kubernetes / helmfile | ecs       |
|----------|------------------------------|-----------|-----------------------|-----------|
| `deploy` | build+push+run / `up -d`     | `apply`   | `apply` / `sync`      | `apply`   |
| `up`     | container run / `compose up` | apply/skip| apply                 | apply     |
| `down`   | `rm` / `compose down`        | `destroy` | `delete`              | `destroy` |
| `build`  | build (members with `build`) | —         | —                     | —         |
| `push`   | push                         | —         | —                     | —         |
| `logs`   | container / compose logs     | —         | kubectl logs          | ecs logs  |
| `ps`     | container / compose ps       | describe  | get pods              | describe  |

Image passing (build → deploy) rides existing step outputs / stack vars; not a new mechanism.

### §F Operate surface

```bash
atmos composition list
atmos composition deploy storefront -s plat-ue2-dev
atmos composition up storefront -s local
atmos composition down storefront -s local
atmos composition logs storefront api -s local
atmos composition ps storefront -s local
atmos composition validate storefront -s prod
```

Shared flags: `-s/--stack`, `--identity`, `--runtime docker|podman`, `--dry-run`. `--profile` /
`ATMOS_PROFILE` remain the Atmos config profile and are not overloaded for Compose profiles.

Workflows and custom commands operate compositions through the same step type:

```yaml
steps:
  - name: deploy
    type: composition
    composition: storefront
    stack: plat-ue2-dev
    action: deploy
```

### §G The full picture (worked example)

A single `storefront` system, end to end, showing how container components, compose components, and
remote kinds compose across environments through one interface.

Composition (defined once):

```yaml
compositions:
  storefront:
    description: Storefront web app, API, and database
    services: [frontend, api, database, mock-stripe, mock-s3, local-infra]
```

`local` stack — Compose for shared infra, containers for the app:

```yaml
# stacks/deploy/local.yaml
components:
  compose:
    local-infra:                       # see compose-components.md
      composition: storefront
      files: [compose.local.yaml]      # registry + postgres + k3s, native Compose
  container:
    frontend:                          # see container-components.md
      composition: storefront
      vars: { build: { context: ./frontend, tags: ["localhost:5001/frontend:{{ .git.sha }}"] } }
      dependencies: { components: [{ component: local-infra }] }
    api:
      composition: storefront
      vars: { build: { context: ./api, tags: ["localhost:5001/api:{{ .git.sha }}"] } }
      dependencies: { components: [{ component: local-infra }] }
```
```bash
atmos composition up storefront -s local      # compose up + container run
```

`dev` stack — ECS app, managed Postgres, plus mocks (extra services, dev only):

```yaml
# stacks/deploy/dev.yaml
components:
  ecs:
    frontend: { composition: storefront }
    api:      { composition: storefront, dependencies: { components: [{ component: postgres }] } }
  terraform:
    postgres: { composition: storefront }
  container:
    mock-stripe: { composition: storefront }
    mock-s3:     { composition: storefront }
```
```bash
atmos composition deploy storefront -s dev
```

`staging` / `prod` stacks — Kubernetes app, managed Postgres, no mocks:

```yaml
# stacks/deploy/staging.yaml
components:
  kubernetes:
    frontend: { composition: storefront }
    api:      { composition: storefront, dependencies: { components: [{ component: postgres }] } }
  terraform:
    postgres: { composition: storefront }
```
```bash
atmos composition deploy storefront -s staging
```

The same command operates the system in every environment. Membership is asymmetric by construction —
`local` provides 4 services, `dev` 5, `staging`/`prod` 3 — with no override syntax. This walkthrough is
mirrored by the runnable `examples/container-step`.

### Deferred

- **Labels**: a general Atmos labels concept is planned for a later iteration and is the likely future
  home for selection/grouping (and could generalize composition membership). No one-off selection
  mechanism is introduced now.
- **Deployments**: a promotion/audit/history layer that references `composition + stack`. It must not
  redefine the membership graph.

## Implementation Notes

- Add top-level `compositions` schema: a minimal `Composition{Description, Services}`. Remove the
  legacy `targets`/`services`-graph model and its custom `UnmarshalYAML`.
- Add the first-class `composition` field to per-component config (`pkg/schema/instance.go`); normalize
  string|list to a slice. Enforce the §C hard-error contract during stack processing.
- Composition resolution: stack → select by `composition` field → order by `dependencies.components`
  → dispatch per member kind. Reuse component-kind execution paths (container/compose/terraform/
  helmfile/kubernetes/ecs).
- `atmos composition` command namespace and the `type: composition` step use `-s/--stack` (not
  `--target`). Use `flags.NewStandardParser()`; never `viper.BindEnv`/`BindPFlag` directly.

## Test Plan

- Schema decode/round-trip for `Composition{Services}` and the first-class `composition` field
  (scalar + list).
- Selection resolves the correct component set per stack; `dependencies.components` ordering.
- Validation, both directions: hard error on undeclared membership / non-existent composition; a
  declared-but-unfulfilled service resolves cleanly; extra env-only services (dev mocks) are included.
- Per-kind verb → action mapping (table-driven; mock runtime for container/compose, mock runner for
  atmos kinds).
- CLI tests (`cmd.NewTestKit`) for `list`, `deploy`, `up`, `down`, `logs`, `ps`, `validate` with `-s`.
- Step handler tests for `type: composition` with `stack`.
- Runtime-gated local smoke: `atmos composition up storefront -s local` brings up compose + container
  members; `down` tears down.
