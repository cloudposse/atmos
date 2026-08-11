# Migrating from Taskfile.yml (go-task)

This guide shows how to move Task's tasks to Atmos. Find the correct shape for the Taskfile
below. Then follow the matching steps. For the full tutorial, see
[atmos.tools/migration/taskfile](https://atmos.tools/migration/taskfile).

Task and Atmos both use declarative YAML. This makes the migration mostly mechanical. It is the
simplest of the three task-runner migrations this skill covers. If the Taskfile also selects a
Terraform environment, also use [from-native-terraform.md](from-native-terraform.md) for the
Terraform-specific steps.

## Find the Shape of the Taskfile

| Shape                                                   | Steps                             |
|------------------------------------------------------------|--------------------------------------|
| Simple task (`desc`/`cmds`)                                 | [Shape A](#shape-a-simple-tasks) |
| Task with `deps:` (parallel by default)                     | [Shape B](#shape-b-task-dependencies-parallel-by-default) |
| `vars:`/`env:` and `sources:`/`generates:`                  | [Shape C](#shape-c-variables-and-up-to-date-checks) |
| `includes:` (multi-file composition)                        | see [Common Problems](#includes-multi-file-composition) |

`deps:` maps to command-level `dependencies.commands` (concurrent by default, deduped -- the
direct match). `sources:`/`generates:` maps to step-level `inputs.sources`/`artifacts.paths`
(implicit `when: checksum.changed` -- the direct match). Neither is a gap; both need the user to
add the matching field during migration, since neither carries over automatically.

## Shape A: Simple Tasks

**Before:**
```yaml
version: '3'

tasks:
  build:
    desc: Compile the deployable artifact
    cmds:
      - go build -o bin/handler ./cmd/handler

  lint:
    desc: Run static analysis
    cmds:
      - golangci-lint run ./...
```

**Steps:**

1. Turn `desc:` into the command's `description:` field.
2. Turn each entry in `cmds:` into a `type: shell` step. If the line is a native Atmos verb, such
    as `terraform plan` or `terraform apply`, use a `type: atmos` step instead. `type: atmos` is
    reserved for native Atmos verbs only. If the line calls another custom command (for example
    `atmos build`), keep it as a `type: shell` step with `command: atmos build` -- do not use
    `type: atmos` for that.
3. Set `internal: true` on a command created from an `internal: true` task. It runs normally
    (`atmos <name> ...`, as a `default:` target, or from another command's steps) but is excluded
    from `atmos --help` listings and completion suggestions. Only inline the task's body into a
    caller's step when it is genuinely single-caller logic with no reason to be invoked on its own.

```yaml
commands:
  - name: build
    description: Compile the deployable artifact
    steps:
      - type: shell
        command: go build -o bin/handler ./cmd/handler

  - name: lint
    description: Run static analysis
    steps:
      - type: shell
        command: golangci-lint run ./...
```

## Shape B: Task Dependencies (Parallel by Default)

**Before:**
```yaml
tasks:
  test:
    desc: Run unit tests
    deps: [build]
    cmds:
      - go test ./...

  deploy:
    desc: Plan and apply the given environment
    deps: [test, lint]
    cmds:
      - terraform -chdir=terraform apply -var-file=envs/dev.tfvars
```

Task runs `deps:` at the same time by default. Atmos custom-command and workflow steps run one
after another by default -- so a `deps:` entry is not a step and never becomes one. It maps to
the command-level `dependencies.commands` field, which resolves through the same DAG scheduler as
`parallel`/`matrix` `needs:` and runs concurrently by default -- matching Task's `deps:` behavior
directly, not working around it with a hand-built `parallel` step:

```yaml
commands:
  - name: deploy
    description: Plan and apply the given environment
    dependencies:
      commands: [test, lint]
    steps:
      - type: atmos
        command: terraform apply infra -s dev
```

`infra` is a placeholder Atmos component name, not the `terraform` verb repeated. Move the
task's Terraform code to `components/terraform/infra/` (the default
`components.terraform.base_path` is `components/terraform`), then swap `infra` for whatever the
user actually names the component.

`dependencies.commands` also matches a behavior Task itself has that a hand-rolled `parallel`
step does not: if two commands both depend on the same one -- for example both `test` and `lint`
depending on `build` -- Atmos runs `build` exactly once and dedups it, the same as Task's own
`deps:` graph. A `parallel` step calling `atmos build` from two different places would run it
twice. If one dependency itself depends on another (`lint` depends on `build`, and `deploy`
depends on `test` and `lint`), declare that directly on `lint`'s own `dependencies.commands` --
the scheduler resolves the whole transitive graph itself, still deduping `build` to a single run.

Reach for a `parallel` step instead of `dependencies.commands` only for concurrency inside a
single command's own steps, not between named commands -- for example, running several shell
commands side by side that were never their own Task tasks to begin with.

## Shape C: Variables and Up-to-Date Checks

**Before:**
```yaml
vars:
  ENV: '{{.ENV | default "dev"}}'

tasks:
  build:
    desc: Compile the deployable artifact
    cmds:
      - go build -o bin/handler ./cmd/handler
    sources:
      - cmd/**/*.go
    generates:
      - bin/handler
```

**Steps:**

- Turn `vars: ENV: '{{.ENV | default "dev"}}'` into a command `flags:` entry with
  `default: "dev"`. Task's Sprig `default` filter becomes the plain `default:` field.
- Turn `env:` into an `env:` map. The two are almost identical.

### `sources:`/`generates:` becomes `inputs`/`artifacts`

Task skips a task's `cmds:` when its `sources:` files match its `generates:` outputs, checked by
default with a content hash (Task also supports `method: timestamp` for an mtime-based check).
The step-level `inputs.sources` and `artifacts.paths` fields are the direct match, with the same
checksum-by-default/timestamp-as-an-option choice:

```yaml
commands:
  - name: build
    description: Compile the deployable artifact
    steps:
      - type: shell
        command: go build -o bin/handler ./cmd/handler
        inputs:
          sources: ["cmd/**/*.go"]
        artifacts:
          paths: ["bin/handler"]
```

With no explicit `when:`, declaring `inputs`/`artifacts` on a step is enough -- it implicitly
means `when: checksum.changed`, and the step is skipped when the hash of the matched source files
matches the hash recorded after the last successful run. This does not carry over on its own --
add `inputs`/`artifacts` to the migrated step yourself, matching the Taskfile's own
`sources:`/`generates:` lists. The `require`/`assert` step type is a different, older step type --
it only checks that a file, tool, or directory exists, not whether it is fresh, so it does not
replace `inputs`/`artifacts`.

## Common Problems

### `includes:` (multi-file composition)

Task's `includes:` field combines several Taskfiles into one. Atmos has two matching methods.
Pick the one that fits the content being split:

- To split command definitions across files, put them in files such as `atmos.d/commands.yaml`
  or `.atmos.d/commands.yaml`. Atmos auto-discovers `atmos.d/`/`.atmos.d/` in the config
  directory (and, as a lower-priority fallback, at the git/worktree root) -- no `import:` entry
  is needed for this specific location. Use `import:` only when splitting across a directory
  Atmos does not auto-discover. See [Imports](https://atmos.tools/cli/configuration/imports).
- To split multi-step chains, use separate workflow files. Atmos workflows already live one file
  per purpose, under `workflows.base_path`. Unlike `atmos.d`/`.atmos.d`, there is no default for
  `workflows.base_path` -- add it explicitly (for example `workflows.base_path:
  "stacks/workflows"`) the first time the user's migration reaches a workflow, or `atmos
  workflow <name>` fails with `'workflows.base_path' must be configured in 'atmos.yaml'`.

### `sources`/`generates` maps to a different field than `steps`

See [Shape C](#sourcesgenerates-becomes-inputsartifacts) above. It does not carry over
automatically -- the user must add `inputs`/`artifacts` to the migrated step themselves. State
that directly. Do not gloss over it, and do not claim it "just works" without the field.

## What Not To Do

- Do not drop `sources:`/`generates:` without adding the matching `inputs`/`artifacts` fields to
  the migrated step. It is a direct match, not a gap, but it does not carry over on its own.
- Do not turn `deps:` into plain sequential steps, or into a hand-built `parallel` step, without
  first considering command-level `dependencies.commands` -- it is the direct match: concurrent
  by default, and it dedups a dependency shared by more than one command the same way Task's own
  `deps:` graph does.
- Do not describe `require`/`assert` as a freshness or caching check. It only checks that
  something exists.
- Do not turn every `internal: true` task into its own discoverable command by default. If it is
  called from only one task, inline it into that caller's step. If it needs to be called from
  more than one task, or invoked directly for debugging, make it an `internal: true` custom command.
