# Standalone Scripts and `cli.command`

A standalone script is a `.star` file (or an executable with an Atmos shebang) that Atmos
runs directly as its own command-line tool. This is "interpreter mode". See also
[Custom CLI apps](https://atmos.tools/automation/standalone-cli-apps) and the
[function reference](https://atmos.tools/functions/automation/cli.command).

## Running a script

```shell
atmos ./deploy.star vpc --stack=dev    # Explicit path.
atmos deploy.star vpc --stack=dev      # A .star file needs no ./ prefix.
./deploy vpc --stack=dev               # Executable with an Atmos shebang.
```

Rules:

- `.star` files: any first argument ending in `.star` selects script mode. The file must
  exist and be a regular file.
- Extensionless files: the path must contain a separator (`./deploy`, `/opt/tools/deploy`)
  and the first line must be an Atmos shebang. A bare name such as `atmos deploy` is still a
  normal Atmos command lookup and fails as an unknown command.
- Accepted shebangs: `#!/usr/bin/env atmos`, `#!/usr/bin/env -S atmos`, and an absolute path
  whose last element is `atmos` (`#!/usr/local/bin/atmos`). Other forms are not detected.
  `atmos` must be on `PATH` for the `env` forms.
- Symlinks are resolved before running. `ctx.script.path` is the real file, and `load()`
  resolves relative to the real file's directory, so a symlink on `PATH` can load its
  sibling modules.
- Atmos global flags must not precede the script path: `atmos --chdir=x deploy.star` fails
  as an unknown command. Everything after the script path belongs to the script. Change
  directory first, or use environment variables (for example `ATMOS_LOGS_LEVEL=Debug`).
- `atmos` with no arguments keeps the normal Atmos screen.
- Atmos configuration (`atmos.yaml`) is discovered from the current working directory with
  the usual search rules, not from the script's directory. A script that does not touch
  stacks or components runs with no `atmos.yaml` at all. `components.get`, `atmos.terraform`,
  and other project-aware calls need an Atmos project in or above the working directory.
- Scripts are not rendered as Go templates; `{{` in a standalone script is ordinary text.
- Standalone scripts do not print the Atmos resource-usage summary. Set
  `settings.metrics.enabled: true` in `atmos.yaml` to opt in.
- `ctx.script.path` and `ctx.script.directory` identify the physical file. `load()` paths are
  relative to that file. Process working directory stays with the caller.
- Top-level `output` is written to stdout (a string raw, anything else as compact JSON).
  `print` also writes to stdout; `ui.*` writes to stderr.

## Inputs

Standalone scripts get their inputs from `cli.command` callbacks or from raw `ctx.args`:

- `ctx.args`: immutable list of all script arguments exactly as typed, including flags.
- `main(args, flags)` callback: parsed `args` and `flags` dicts (see below).
- `ctx.flags`, `ctx.arguments`, and `env` are empty in standalone scripts. Those are for
  script steps in workflows and custom commands.

With no `cli.command`, handle `ctx.args` yourself. Prefer `cli.command`: it gives native
`--help`, typed flags, validation, and error messages.

## Declaring the interface

```python
def main(args, flags):
    print(args["service"], flags["replicas"], flags["dry-run"], flags["tag"])

def validate(args, flags):
    if flags["replicas"] < 1:
        fail("--replicas must be at least 1")

cli.command(
    run = main,
    name = "deploy",
    description = "Deploy a service",
    args = [cli.arg("service", description="Service name")],
    flags = [
        cli.flag("replicas", type="int", default=2, shorthand="r", description="Replica count"),
        cli.flag("dry-run", type="bool", shorthand="d"),
        cli.flag("tag", type="string_list", env="DEPLOY_TAGS"),
        cli.flag("env", choices=["dev", "prod"], default="dev"),
    ],
    validate = validate,
)
```

`cli.command(run, name=, description=, args=, flags=, validate=)`:

- `run` is required and called as `run(args, flags)` with immutable dicts. `cli.command`
  returns what `run` returns (and `None` for `--help`), so `output = cli.command(...)` emits
  that value; a `None` return would print `null`. For ordinary output, call `print` inside `run`.
- `name` (default: the script file name) must start with a letter and contain only letters,
  digits, `_`, and `-`. `description` appears in help.
- `validate(args, flags)` is optional. Call `fail("message")` or return `False` to reject
  input. Returning `None` or `True` accepts it; any other return value is an error.
- Call `cli.command` once, from the main thread. A second call, or a call inside a
  `steps.parallel` task, fails. It is unavailable in script steps (workflows, custom
  commands, hooks); use `ctx.flags` and `ctx.arguments` there.

`cli.arg(name, description=, required=True)`:

- Names must start with a letter and contain only letters, digits, `_`, and `-`.
- Argument names must be unique, and required arguments must come before optional ones.
- A missing optional argument is `None` in the `args` dict.

`cli.flag(name, type="string", default=, shorthand=, description=, required=False, choices=, env=)`:

- `type` is `"string"`, `"int"`, `"bool"`, or `"string_list"`.
- `required=True` cannot have a `default`. A `bool` flag cannot be required; use a default.
- `choices` works only for `string` and `string_list`.
- `shorthand` is one letter and cannot be `h`. Shorthands must be unique.
- `help` is reserved (and `-h`). Every script gets `--help` automatically.
- `env` binds an environment variable used when the flag is not given on the command line.

## Parsing behavior to know

- `--help` or `-h` prints the native help and skips both `validate` and `run`. Top-level code
  still runs before help, so keep side effects (network calls, deployments, file writes)
  inside `run`, never at the top level.
- Unknown flags and extra positional arguments fail with an error. Missing required flags
  and invalid `choices` values fail before `run`.
- `--` ends flag parsing. Tokens after it are still validated as declared positional
  arguments; there is no variadic or trailing-argument capture. For
  free-form arguments, read `ctx.args`.
- A `bool` flag takes no value token. `--dry-run false` parses `false` as a positional
  argument (an error if no positional is left). Write `--dry-run=false`.
- `int` values accept base prefixes, so `--n 010` is octal 8. Use plain decimal and
  document that to users.
- `string_list` flags accumulate repeated flags and split a single CLI value on commas
  (`--tag a,b --tag c` gives `["a", "b", "c"]`). An `env` binding is split on whitespace
  instead (`DEPLOY_TAGS="a b c"`). Recommend repeated flags or comma lists on the CLI.
- Command-line values take precedence over `env`, which takes precedence over `default`.
