# Migrating from Justfiles

This guide shows how to move Just recipes to Atmos. Find the correct shape for the Justfile
below. Then follow the matching steps. For the full tutorial, see
[atmos.tools/migration/justfile](https://atmos.tools/migration/justfile).

Just recipe bodies do not require tab indentation, unlike Make. Just's named parameters with
default values map closely to Atmos custom command `flags:` and `arguments:`. This is the
closest match of the three task runners this skill covers. If the Justfile also selects a
Terraform environment, also use [from-native-terraform.md](from-native-terraform.md) for the
Terraform-specific steps.

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
    golangci-lint run ./...

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
        command: golangci-lint run ./...
```

## Shape B: Recipe Dependencies

**Before:**
```just
# Run tests (builds first)
test: build
    go test ./...

# Deploy to the given environment (defaults to dev)
deploy env='dev': build test
    cd terraform && terraform apply -var-file=envs/{{env}}.tfvars
```

**Steps:** use the same method as Make's dependency chains. See
[from-makefile.md Shape B](from-makefile.md#shape-b-target-chains-with-dependencies). Do not use
a `type: atmos` step to call another custom command. That step type is only for native Atmos
verbs. Call the dependency with a `type: shell` step and `command: atmos build`.

```yaml
commands:
  - name: test
    description: Run tests (builds first)
    steps:
      - type: shell
        command: atmos build
      - type: shell
        command: go test ./...

  - name: deploy
    description: Deploy to the given environment (defaults to dev)
    flags:
      - name: env
        shorthand: e
        default: "dev"
    steps:
      - type: shell
        command: atmos test
      - type: atmos
        command: terraform apply infra -s {{ .Flags.env }}
```

`infra` is a placeholder Atmos component name, not the `terraform` verb repeated. Move the
recipe's Terraform code to `components/terraform/infra/` (the default
`components.terraform.base_path` is `components/terraform`), then swap `infra` for whatever the
user actually names the component.

## Shape C: Environment and Shell Settings

**Before:**
```just
set dotenv-load := true
set shell := ["bash", "-uc"]

export AWS_REGION := "us-east-1"
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
      AWS_REGION: us-east-1
    steps:
      - type: shell
        command: go build -o bin/handler ./cmd/handler
```

## Common Problems

### `{{ }}` interpolation looks like Atmos templates but is not

Just's `{{ var }}` syntax and Atmos's `{{ .Flags.var }}` syntax both use Go templates, but they
run in different tools at different times. Do not copy Just interpolation syntax into Atmos
YAML. Change each reference to the matching `{{ .Flags.<name> }}` or
`{{ .Arguments.<name> }}` form.

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
