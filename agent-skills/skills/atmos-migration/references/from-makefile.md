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
```makefile
ENV ?= dev

deploy: build test ## Plan and apply the given ENV (default: dev)
	cd terraform && terraform apply -var-file=envs/$(ENV).tfvars
```

**Steps:**

1. Turn `ENV ?= dev` into a command `flags:` entry with `default: "dev"`.
2. Turn the target order (`deploy: build test`) into steps that run in the same order. In this
   example, the steps call the Shape A commands, one after the other.
3. Check if the prerequisites are truly independent. In this example, `build` must finish before
   `test` runs, but nothing else depends on their order relative to each other. When two
   prerequisites do not depend on each other, use a `parallel` step with `needs:` instead of
   listing them one after the other. See [Shape C](#shape-c-recursive-or-parallel-make) for the
   general `parallel`/`matrix` pattern.
4. Move the Terraform-specific line, `terraform apply -var-file=envs/$(ENV).tfvars`, to
   [from-native-terraform.md Shape B](from-native-terraform.md#shape-b-single-dir-with--var-file-from-a-makefile).
   That guide shows how the Terraform side maps to stacks. Here, the line becomes a single
   `type: atmos` step, because `terraform apply` is a native Atmos verb.
5. Turn `ifeq ($(ENV),prod)` conditionals into a Go template conditional inside a custom command:
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
    steps:
      - type: shell
        command: atmos build
      - type: shell
        command: atmos test
      - type: atmos
        command: terraform apply infra -s {{ .Flags.env }}
```

## Shape C: Recursive or Parallel Make

**Before:**
```makefile
SERVICES := vpc eks rds

build-all:
	for dir in $(SERVICES); do $(MAKE) -C services/$$dir build; done

build-parallel:
	$(MAKE) -j4 build-all
```

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
changed, that caching behavior has no equivalent in Atmos. Atmos steps always run. Tell the user
this directly. Do not imply that the behavior carries over. Task's `sources:`/`generates:`
feature has the same problem. See
[from-taskfile.md](from-taskfile.md#the-sourcesgenerates-gap) for more detail.

### Silent recipes and command echo

`@command` suppresses the echo of one command line. It maps to the step field `output: none` on
that one step. It does not mean you should add `output: none` to every step.

### Split commands across files

When a Makefile uses `include foo.mk` to split its content across files, split the Atmos config
the same way. Put the extra commands in a file such as `atmos.d/commands.yaml`. Then add that
file to the root config with `import:` in `atmos.yaml`. See
[Imports](https://atmos.tools/cli/configuration/imports).

### Complex `$(eval)` and `$(call)` macros

Do not try to convert deeply macro-driven Makefiles into flags and arguments one by one. Put
genuinely dynamic logic in a `shell` or `script` step, or in a script that the step calls. Atmos
custom commands replace task orchestration. They do not replace a general-purpose macro
language.

## What Not To Do

- Do not build file-timestamp or `.PHONY` caching as an Atmos feature. It does not exist. State
  this directly instead of dropping the behavior without comment.
- Do not turn every private or helper recipe into its own discoverable command. There is no
  `hidden` or `private` field on custom commands. Put the helper logic in a step inside the
  command or workflow that needs it.
- Do not treat "wrap `atmos` commands in the Makefile" as the final state. It is a valid bridge
  during early migration, as shown in Shape B, step 4. Leaf targets should become custom
  commands. Target chains should become workflows.
- Do not invent `when:` conditions that check flag values on workflow steps. The `when:` field
  checks CEL context values, such as `stack`, `ci`, and `local`. Flag-based conditionals belong
  in the custom command's own Go templates.
