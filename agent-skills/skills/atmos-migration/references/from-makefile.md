# Migrating from Makefiles

This guide shows how to move Make targets to Atmos. Find the correct shape for the Makefile
below. Then follow the matching steps. For the full tutorial, see
[atmos.tools/migration/makefile](https://atmos.tools/migration/makefile).

This guide covers the `make` orchestration layer only: targets, target dependencies, variables,
and conditionals. If the Makefile also selects a Terraform environment through `-var-file` or
per-environment directories, also use
[from-native-terraform.md](from-native-terraform.md). That guide covers the Terraform-specific
steps: backend generation, `.tfvars` files, and workspace mapping.

## Find the Shape of the Makefile

| Shape                                                        | Steps                            |
|-----------------------------------------------------------------|-------------------------------------|
| Independent leaf targets (`build`, `test`, `lint`, `clean`)      | [Shape A](#shape-a-independent-leaf-targets) |
| Target chains with dependencies (`deploy: build test`)           | [Shape B](#shape-b-target-chains-with-dependencies) |
| Recursive or parallel Make (`$(MAKE) -C dir`, `make -j`)         | [Shape C](#shape-c-recursive-or-parallel-make) |

Most Makefiles mix all three shapes. Treat each target on its own. Then combine the results.

## Shape A: Independent Leaf Targets

**Before:**
<!-- editorconfig-checker-disable -->
```makefile
.PHONY: build test lint clean help

help: ## Show available targets
	@awk 'BEGIN {FS=":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Compile the deployable artifact
	go build -o bin/handler ./cmd/handler

test: build ## Run unit tests
	go test ./...

lint: ## Run static analysis
	golangci-lint run ./...

clean: ## Remove build artifacts
	@rm -rf bin/
```
<!-- editorconfig-checker-enable -->

**Steps:**

1. Turn each leaf target into a custom command in `atmos.yaml`. If the Makefile is large, put
    the commands in a separate file. See
    [Split commands across files](#split-commands-across-files) below.
2. Turn the silent-recipe `@` prefix into the step field `output: none`.
3. Delete the `help` target. Atmos generates the same information from each command's
    `description:` field. Run `atmos help` or `atmos <command> --help` to see it.
4. When one target depends on another leaf target, such as `test: build`, add a step that runs
    the dependency's command. Do not use a `type: atmos` step to call a custom command. That step
    type is only for native Atmos verbs, such as `terraform plan`. Use a `type: shell` step with
    `command: atmos build` instead.

```yaml
commands:
  - name: build
    description: Compile the deployable artifact
    steps:
      - type: shell
        command: go build -o bin/handler ./cmd/handler

  - name: test
    description: Run unit tests
    steps:
      - type: shell
        command: atmos build
      - type: shell
        command: go test ./...

  - name: lint
    description: Run static analysis
    steps:
      - type: shell
        command: golangci-lint run ./...

  - name: clean
    description: Remove build artifacts
    steps:
      - type: shell
        command: rm -rf bin/
        output: none
```

## Shape B: Target Chains with Dependencies

**Before:**
<!-- editorconfig-checker-disable -->
```makefile
ENV ?= dev

deploy: build test ## Plan and apply the given ENV (default: dev)
	cd terraform && terraform apply -var-file=envs/$(ENV).tfvars
```
<!-- editorconfig-checker-enable -->

**Steps:**

1. Turn `ENV ?= dev` into a command `flags:` entry with `default: "dev"`.
2. Turn the target list (`deploy: build test`) into command-level `dependencies.commands: [build,
    test]`. This is the direct match, not a workaround: it resolves through the same DAG scheduler
    as `parallel`/`matrix` `needs:`, runs concurrently by default -- `make` itself does not
    guarantee prerequisite order without `-j` either -- and dedups a dependency shared by more
    than one target to a single run, the same guarantee `make` already gives for free. Do not turn
    this into plain sequential steps unless one prerequisite genuinely must finish before another
    starts; if so, declare that dependency directly on the later one's own `dependencies.commands`
    instead of ordering a flat list.
3. Move the Terraform-specific line, `terraform apply -var-file=envs/$(ENV).tfvars`, to
    [from-native-terraform.md Shape B](from-native-terraform.md#shape-b-single-dir-with--var-file-from-a-makefile).
    That guide shows how the Terraform side maps to stacks. Here, the line becomes a single
    `type: atmos` step, because `terraform apply` is a native Atmos verb.
4. Turn `ifeq ($(ENV),prod)` conditionals into a Go template conditional inside a custom command:
    `{{ if eq .Flags.env "prod" }}...{{ end }}`. This is the same pattern used for `--verbose` and
    other boolean flags. Inside a workflow, use `when: !cel 'stack == "prod"'` on the step
    instead.

```yaml
commands:
  - name: deploy
    description: Plan and apply the given environment (default dev)
    flags:
      - name: env
        shorthand: e
        default: "dev"
    dependencies:
      commands: [build, test]
    steps:
      - type: atmos
        command: terraform apply infra -s {{ .Flags.env }}
```

`infra` is a placeholder Atmos component name, not the `terraform` verb repeated. Move the
target's Terraform code to `components/terraform/infra/` (the default
`components.terraform.base_path` is `components/terraform`), then swap `infra` for whatever the
user actually names the component.

## Shape C: Recursive or Parallel Make

**Before:**
<!-- editorconfig-checker-disable -->
```makefile
SERVICES := vpc eks rds

build-all:
	for dir in $(SERVICES); do $(MAKE) -C services/$$dir build; done

build-parallel:
	$(MAKE) -j4 build-all
```
<!-- editorconfig-checker-enable -->

**Steps:**

1. Turn `$(MAKE) -j` into a `parallel` step. Use `max_concurrency` to set the fan-out width.
2. Turn `$(MAKE) -C dir target` recursion over a fixed set of directories into a `matrix` step.
    Define a `service` axis, and call the per-service command once for each value.

```yaml
commands:
  - name: build-all
    description: Build every service
    steps:
      - name: build-services
        type: matrix
        matrix:
          service: [vpc, eks, rds]
        max_concurrency: 4
        steps:
          - type: shell
            command: atmos build --service {{ .matrix.service }}
```

## Common Problems

### Tabs, `.PHONY`, and file-timestamp caching

Atmos steps have no tab requirement. Do not confuse `.PHONY` with a caching feature. If a
Makefile target is not `.PHONY` and uses file timestamps to skip work when inputs have not
changed, turn it into step `inputs.sources`/`artifacts.paths` -- with no explicit `when:`, that
implicitly means `when: checksum.changed`, and the step is skipped when nothing has changed since
its last successful run. It does not carry over automatically; tell the user to add
`inputs`/`artifacts` to the migrated step themselves. Content hashing (the default) is a
deliberate upgrade over Make's own mtime comparison -- a fresh `git clone`/CI checkout resets
every file's mtime, which makes Make think everything changed even when it didn't; use
`when: timestamp.changed` instead for Make's exact mtime semantics. Task's `sources:`/`generates:`
feature maps to the same fields. See
[from-taskfile.md](from-taskfile.md#sourcesgenerates-becomes-inputsartifacts) for more detail.

### Silent recipes and command echo

`@command` suppresses the echo of one command line. It maps to the step field `output: none` on
that one step. It does not mean you should add `output: none` to every step.

### Split commands across files

When a Makefile uses `include foo.mk` to split its content across files, split the Atmos config
the same way. Put the extra commands in a file such as `atmos.d/commands.yaml` or
`.atmos.d/commands.yaml`. Atmos auto-discovers `atmos.d/`/`.atmos.d/` in the config directory
(and, as a lower-priority fallback, at the git/worktree root) -- no `import:` entry is needed for
this specific location. Use `import:` only when splitting across a directory Atmos does not
auto-discover. See [Imports](https://atmos.tools/cli/configuration/imports).

### Complex `$(eval)` and `$(call)` macros

Do not try to convert deeply macro-driven Makefiles into flags and arguments one by one. Put
genuinely dynamic logic in a `shell` or `script` step, or in a script that the step calls. Atmos
custom commands replace task orchestration. They do not replace a general-purpose macro
language.

## What Not To Do

- Do not confuse `.PHONY` with a caching feature -- it is not one. Do not drop file-timestamp
  caching without adding the matching `inputs`/`artifacts` fields to the migrated step; it is a
  direct match, not a gap, but it does not carry over on its own.
- Do not turn `target: dep1 dep2` into plain sequential steps, or into a hand-built `parallel`
  step, without first considering command-level `dependencies.commands` -- it runs concurrently
  by default and dedups a dependency shared by more than one target, the way `make` already does.
- Do not turn every private or helper target into its own discoverable command by default. If the
  helper is called from only one recipe, put its logic in a step inside the command or workflow
  that needs it. If it needs to be called from more than one recipe, or invoked directly for
  debugging, make it a custom command with `internal: true` instead -- it stays runnable but is
  excluded from `atmos --help` listings and completion suggestions.
- Do not treat "wrap `atmos` commands in the Makefile" as the final state. It is a valid bridge
  during early migration. Leaf targets should become custom commands. Target chains should become
  workflows.
- Do not invent `when:` conditions that check flag values on workflow steps. The `when:` field
  checks CEL context values, such as `stack`, `ci`, and `local`. Flag-based conditionals belong
  in the custom command's own Go templates.
