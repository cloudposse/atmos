# Standalone Scripts and `cli.command`

A standalone script is a `.star` file (or an executable with an Atmos shebang) that Atmos
runs directly as its own command-line tool. This is "interpreter mode". See also
[Custom CLI apps](https://atmos.tools/automation/standalone-cli-apps) and the
[language overview](https://atmos.tools/automation/language).

## Running a script

```shell
atmos ./deploy.star vpc --stack=dev    # Explicit path.
atmos deploy.star vpc --stack=dev      # A .star file needs no ./ prefix.
./deploy vpc --stack=dev               # Executable with an Atmos shebang.
```

Rules:

- `.star` files: any first argument ending in `.star` selects script mode. The file must
  exist and be a regular file. Only `.star` names can produce a script error (a `.star`
  directory fails with "must be a regular file").
- Extensionless files: the path must contain a separator (`./deploy`, `/opt/tools/deploy`)
  and the first line must be an Atmos shebang. A bare name such as `atmos deploy` is still a
  normal Atmos command lookup and fails as an unknown command. A path that is not a script (an
  existing directory such as `./stacks`, a missing file, a file without the shebang) falls
  through to normal command handling.
- The `.star` extension is optional when the script runs by path or through its shebang. It
  is what lets editors and GitHub recognize the file as Starlark. For extensionless files, put
  the shebang on line 1 and a file-type hint on line 2, for example
  `# vim: set filetype=bzl: -*- mode: bazel-starlark -*-` (Vim has no `starlark` filetype;
  Emacs needs the bazel-mode package for `bazel-starlark`). GitHub Linguist only maps
  `ft=`/`mode:` values of `starlark` or `bazel`, so use `.gitattributes`
  (`bin/tool linguist-language=Starlark`) for extensionless files there. The hint line does not
  affect shebang detection.
- Accepted shebangs: `#!/usr/bin/env atmos`, `#!/usr/bin/env -S atmos`, and an absolute path
  whose last element is `atmos` (`#!/usr/local/bin/atmos`). Other forms are not detected.
  `atmos` must be on `PATH` for the `env` forms.
- Symlinks are resolved before running. `ctx.script.path` is the real file, and `load()`
  resolves relative to the target file's directory, so the target file can load its
  sibling modules.
- Atmos global flags go between `atmos` and the script path, as `--flag=value` or
  `--flag value`: `atmos --chdir=x deploy.star`, `atmos --logs-level Debug ./deploy.star`. The
  first word that is not a global flag starts the script; everything after it belongs to the
  script, including flags that look like Atmos flags. A leading `--` ends the global flags.
  A relative script path resolves after `--chdir` is applied. Flags with an optional value
  (`--identity`) must use `--flag=value`. An unknown flag before the script is not a script
  invocation and fails like any other unknown flag.
- With `ATMOS_USE_VERSION` or `version.use`, the re-exec forwards the script path and its
  arguments unchanged.
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
  that value. A `None` result means no output: nothing prints for `--help` or when `run`
  returns nothing. For ordinary output, call `print` inside `run`.
- `name` (default: the script file name) must start with a letter and contain only letters,
  digits, `_`, and `-`. `description` appears in help.
- `validate(args, flags)` is optional. Call `fail("message")` or return `False` to reject
  input. Returning `None` or `True` accepts it; any other return value is an error.
- Call `cli.command` once, from the main thread. A second call, or a call inside a
  `steps.parallel` task, fails. It is unavailable in script steps (workflows, custom
  commands, hooks); use `ctx.flags` and `ctx.arguments` there.

`cli.arg(name, description=, required=True)`:

- Names must start with a letter and contain only letters, digits, `_`, and `-`.
- Argument names must be unique ignoring case, and required arguments must come before
  optional ones.
- A missing optional argument is `None` in the `args` dict.

`cli.flag(name, type="string", default=, shorthand=, description=, required=False, choices=, env=)`:

- `type` is `"string"`, `"int"`, `"bool"`, or `"string_list"`.
- `required=True` cannot have a `default`. A `bool` flag cannot be required; use a default.
- `choices` works only for `string` and `string_list`.
- Flag names must be unique ignoring case (`Stage` and `stage` collide and are rejected).
- `shorthand` is one letter and cannot be `h`. Shorthands must be unique.
- `help` is reserved (and `-h`). Every script gets `--help` automatically.
- `env` binds an environment variable used when the flag is not given on the command line.

## Parsing behavior to know

- `--help` or `-h` prints the help and skips both `validate` and `run`. Help marks required
  flags `(required)`, lists choices as `(one of: dev, prod)`, and shows env bindings as
  `[env: NAME]`. Top-level code
  still runs before help, so keep side effects (network calls, deployments, file writes)
  inside `run`, never at the top level.
- User input mistakes (unknown flag, missing or extra positional, invalid value, missing
  required flag, failed `choices`) are usage errors: no Starlark traceback, a hint
  `Run <script> --help for usage.`, the usage line, and exit status 2. A missing required
  positional names the argument. They fail before `validate` and `run`. Errors from the
  script itself (`fail()`, runtime errors, `validate` returning `False`) keep the Starlark
  traceback presentation.
- `--` ends flag parsing. Tokens after it are still validated as declared positional
  arguments; there is no variadic or trailing-argument capture. For
  free-form arguments, read `ctx.args`.
- A `bool` flag takes no value token. `--dry-run false` is rejected with a hint to write
  `--dry-run=false`, because `false` would otherwise become a positional argument. Put `--`
  before a positional that must be literally `true` or `false`.
- `int` values (command line and env) are base 10 only: `--n 010` is 10, and `0x10` is an
  error.
- `string_list` flags accumulate repeated flags and split a single value on commas with CSV
  quoting (`--tag a,b --tag c` gives `["a", "b", "c"]`). An `env` binding parses exactly the
  same way (`DEPLOY_TAGS=a,b`); whitespace is not a separator. With `choices`, every element is
  validated.
- Command-line values take precedence over `env`, which takes precedence over `default`.
