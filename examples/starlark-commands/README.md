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

Give your team a project-specific command with flags, defaults, and generated help.
This example adds `atmos capacity` to calculate how many workers a service can run.
`capacity` is defined by this project's `atmos.yaml`; it is not a built-in Atmos command.

The YAML defines the command's name, flag, default, and help text. An embedded
Atmos Automation Language script implements the calculation and rejects counts
below one. Atmos registers the subcommand, parses its inputs, and passes them to
the script. The language is Python-like, built on Starlark, and runs inside Atmos.
No stacks, components, or external tools are needed.

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
--replicas string  Number of service replicas (default 2)
```

The step reads the parsed flag directly from `ctx.flags["replicas"]`. Named custom
command arguments are available in `ctx.arguments`. Both mappings are read-only;
values stay data, including strings that happen to contain template syntax.

For a standalone executable such as `./capacity.star`, with its interface declared
in the script using `cli.command`, see the
[executable script example](https://atmos.tools/examples/starlark-script).

See [custom commands](https://atmos.tools/cli/configuration/commands) and the
[Starlark reference](https://atmos.tools/steps/type/script#embedded-starlark).
