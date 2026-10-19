# Migrating from Justfiles

Migrate Just recipes to Atmos as a general-purpose task runner. Preserve the user's build,
test, lint, release, and maintenance commands. Start with `atmos.yaml` and custom commands;
Terraform, stacks, components, and cloud credentials are not prerequisites.

Atmos can call the existing task runner as a shell step while individual tasks are migrated.
Follow the matching shape below and the [end-user guide](https://atmos.tools/migration/justfile).

## Find the Shape of the Justfile

| Shape                                                              | Steps                          |
|-----------------------------------------------------------------------|-----------------------------------|
| Recipes with named parameters and default values                      | [Shape A](#shape-a-recipes-with-named-parameters) |
| Recipe dependencies (`build: test`)                                    | [Shape B](#shape-b-recipe-dependencies) |
| `set dotenv-load`, `export VAR := ...`, `set shell := [...]`           | [Shape C](#shape-c-environment-and-shell-settings) |

## Shape A: Recipes with Named Parameters

**Before:**
```just
# Build the deployable artifact
build:
    go build -o bin/handler ./cmd/handler

# Run static analysis
lint:
    go vet ./...

[private]
_clean:
    rm -rf bin/
```

**Steps:**

1. Turn the `# comment` above a recipe into the command's `description:` field. Atmos shows this
    text in `atmos --help` and `atmos <command> --help`. This replaces `just --list`.
2. Turn a recipe's named parameter with a default value, such as `deploy env='dev':`, into a
    command `flags:` entry with a matching `default:` value. Inside a step, read the value as
    `{{ .Flags.env }}`. Do not use Just's own `{{env}}` syntax. See
    [Common Problems](#--interpolation-looks-like-atmos-templates-but-is-not) below.
3. Set `internal: true` on a command created from a `[private]` recipe. It runs normally
    (`atmos <name> ...`, as a `default:` target, or from another command's steps) but is excluded
    from `atmos --help` listings and completion suggestions. Only inline the recipe's body into a
    caller's step when it is genuinely single-caller logic with no reason to be invoked on its own.

```yaml
commands:
  - name: build
    description: Build the deployable artifact
    steps:
      - type: shell
        command: go build -o bin/handler ./cmd/handler

  - name: lint
    description: Run static analysis
    steps:
      - type: shell
        command: go vet ./...
```

## Shape B: Recipe Dependencies

**Before:**
```just
# Run tests (builds first)
test: build
    go test ./...

# Deploy to the given environment (defaults to dev)
deploy env='dev': build test
    ./scripts/deploy.sh "{{env}}"
```

**Steps:** use the same method as Make's dependency chains. See
[from-makefile.md Shape B](from-makefile.md#shape-b-target-chains-with-dependencies). Use a
`type: atmos` step with `command: build` to call another custom command -- `type: atmos` preserves
step-level stack context and structured output handling, which a `type: shell` step running
`atmos build` does not.

```yaml
commands:
  - name: test
    description: Run tests (builds first)
    steps:
      - type: atmos
        command: build
      - type: shell
        command: go test ./...

  - name: deploy
    description: Deploy to the given environment (defaults to dev)
    flags:
      - name: env
        shorthand: e
        default: "dev"
    steps:
      - type: atmos
        command: test
      - type: shell
        command: ./scripts/deploy.sh "{{ .Flags.env }}"
```

The deployment script receives the application environment as its first argument. Keep the
user's existing script; `--env` is a custom flag and does not select an Atmos stack.

## Shape C: Environment and Shell Settings

**Before:**
```just
set dotenv-load := true
set shell := ["bash", "-uc"]

export APP_LOG_LEVEL := "info"
```

**Steps:**

- Turn `export VAR := value` into a command or step `env:` map.
- `set dotenv-load` maps to `env: !include .env` on the command, workflow, or step. Atmos parses
  the dotenv file natively (including `export VAR=value`, comments, quoting, and `${VAR}`
  expansion) and merges the result into `env:`. If the values are secrets rather than plain
  config, use Atmos's store or secrets integration instead of a plaintext `.env` file.
- `set shell := [...]` changes the shell for every recipe in the Justfile. Atmos has no matching
  command-level setting. Use `type: script` with an explicit `interpreter:` field on the one step
  that needs a different interpreter.

```yaml
commands:
  - name: build
    description: Build the deployable artifact
    env:
      <<: !include .env
      APP_LOG_LEVEL: info
    steps:
      - type: shell
        command: go build -o bin/handler ./cmd/handler
```

## Common Problems

### `{{ }}` interpolation looks like Atmos templates but is not

Just's `{{ var }}` syntax looks like Atmos's `{{ .Flags.var }}` syntax, but the two are not the
same templating tool. Just evaluates `{{ ... }}` with its own built-in expression language
(variables, operators, string and path functions), not Go's `text/template` package. Atmos's
`{{ .Flags.var }}` syntax is a real Go template, rendered by Atmos itself at a different time.
Do not copy Just interpolation syntax into Atmos YAML. Change each reference to the matching
`{{ .Flags.<name> }}` or `{{ .Arguments.<name> }}` form.

### `[private]` recipes map to `internal: true`

The custom command schema has an `internal: true` field. It excludes the command from `atmos --help`
listings and completion suggestions while leaving it fully runnable -- directly, as a `default:`
target, or from another command's steps. This is the direct equivalent of a `[private]` recipe,
and it covers cases plain step-inlining cannot: a helper called from more than one recipe, or one
a user invokes by name for manual debugging.

Only inline a `[private]` recipe's logic into a caller's step when it is genuinely single-caller
and has no reason to be invoked on its own -- in that case a separate `internal` command is just
unnecessary indirection.

If a `[private]` recipe is never called by any public recipe (an orphaned helper, not a
dependency), `internal: true` no longer forces the same discovery you'd get from step-inlining --
it would just as quietly hide dead code as reachable helper code. Confirm with the user whether
the recipe is still needed at all before migrating it; if it is, ask whether it should become a
`internal` command, a step inside whichever command ends up needing it, or a short script the user
maintains separately.

### Command echo differs between `just` and Atmos

By default, Just prints each recipe line before running it (`sh -x`-style), so `just build`'s
visible output includes every command line, not just what those commands print. Atmos `type:
shell` steps run silently by default -- only the command's own stdout/stderr shows. The migrated
command's side effects match the original recipe, but the terminal output will look sparser side
by side. Tell the user this if they compare `just <recipe>` output to `atmos <command>` output
directly; it is a visible difference, not a bug.

### Confirm `set shell` with the user; `dotenv-load` has a direct replacement

`set dotenv-load` maps directly to `env: !include .env` -- no confirmation needed unless the
`.env` file holds secrets, in which case ask whether to use Atmos's store or secrets integration
instead. `set shell` has no command-level equivalent; ask the user if a non-default shell matters
to their workflow, then apply `type: script` with `interpreter:` to the specific steps that need
it.

## What Not To Do

- Do not assume `{{ }}` means the same thing after you move it into Atmos YAML.
- Do not invent a visibility value beyond the documented `internal: true` boolean (no "public"/"private" enum, no partial visibility).
- Do not drop `set shell` behavior without telling the user; `dotenv-load` maps directly to
  `env: !include .env`, so it does not need the same case-by-case confirmation.
