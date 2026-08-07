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
3. Set `hidden: true` on a command created from an `internal: true` task. It runs normally
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

This is the most important problem in this migration. Task runs `deps:` at the same time by
default. Atmos custom-command and workflow steps run one after another by default. This is the
opposite default. If you turn `deps: [test, lint]` into two plain steps that run one after
another, the command becomes slower. It also changes what happens when one task fails. To keep
Task's default behavior, put the dependency tasks inside a `parallel` step:

```yaml
commands:
  - name: deploy
    description: Plan and apply the given environment
    steps:
      - name: checks
        type: parallel
        fail:
          mode: wait_all
        steps:
          - name: test
            type: shell
            command: atmos test
          - name: lint
            type: shell
            command: atmos lint
      - type: atmos
        command: terraform apply terraform -s dev
```

If the Taskfile's `deps:` list needs its own internal order, add `needs:` to the steps inside the
`parallel` block. Do not assume the tasks should run one after another just because that is
Atmos's default for steps outside a `parallel` or `matrix` block.

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

### The `sources`/`generates` gap

Task skips a task's `cmds:` when its `sources:` files match its `generates:` outputs. It checks
this with a file hash. Atmos steps always run. There is no built-in check for whether a file is
up to date. The `require`/`assert` step type does not fix this. It only checks that a file, tool,
or directory exists. It does not compare hashes or timestamps.

If the user depends on `sources:`/`generates:` to skip a slow step, such as code generation, tell
them plainly that this behavior does not carry over. Then offer two honest choices:

1. Accept that the step always runs. This is correct for most fast build steps.
2. Add a hash or timestamp check inside the shell step itself. This is a script the user
    maintains. It is not a built-in Atmos feature.

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

### `sources`/`generates` has no built-in match

See [Shape C](#the-sourcesgenerates-gap) above. This is the largest real gap in this migration.
State it directly. Do not gloss over it.

## What Not To Do

- Do not drop `sources:`/`generates:` caching without comment. State the change directly. Let
  the user decide how, or whether, to replace it.
- Do not turn `deps:` into plain sequential steps without warning the user about the change in
  default concurrency.
- Do not describe `require`/`assert` as a freshness or caching check. It only checks that
  something exists.
- Do not turn every `internal: true` task into its own discoverable command by default. If it is
  called from only one task, inline it into that caller's step. If it needs to be called from
  more than one task, or invoked directly for debugging, make it a `hidden: true` custom command.
