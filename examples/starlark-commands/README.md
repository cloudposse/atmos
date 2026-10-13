---
related_docs:
  - label: Custom commands
    url: /cli/configuration/commands
  - label: Embedded Starlark
    url: /steps/type/script#embedded-starlark
cast:
  file: /casts/examples/starlark-commands/capacity.cast
  title: Add your own Atmos subcommand
---

# Add your own Atmos subcommand

This example defines `atmos capacity` in `atmos.yaml`. The command calculates
how many workers a service can run and provides a `--replicas` flag and generated help.

Atmos parses the command's inputs and passes them to a script written in the
Atmos Automation Language, a Python-like language based on Starlark. The interpreter
ships with Atmos. The script calculates worker capacity and rejects
replica counts below one. Install Atmos and put it on your `PATH`; this example
needs no stacks, components, or external tools.

From this directory:

```shell
atmos capacity --replicas 3
```

```text
✓ 3 replicas x 4 workers = 12 workers
```

Omit the flag to use the default of two replicas:

```shell
atmos capacity
```

```text
✓ 2 replicas x 4 workers = 8 workers
```

Run `atmos capacity --help` to see the command description and its flag:

```text
--replicas int  Number of service replicas (default 2)
```

The flag is declared with `type: int`, so Atmos rejects a value such as `--replicas=many`
with a usage error before the script runs, and the step reads the parsed number directly
from `ctx.flags["replicas"]`. Named custom
command arguments are available in `ctx.arguments`. Both mappings are read-only;
values stay data, including strings that happen to contain template syntax.

For a standalone executable such as `./capacity.star`, with its interface declared
in the script using `cli.command`, see the
[executable script example](https://atmos.tools/examples/starlark-script).

See [custom commands](https://atmos.tools/cli/configuration/commands) and the
[Starlark reference](https://atmos.tools/steps/type/script#embedded-starlark).
