---
related_docs:
  - label: Script steps
    url: /steps/type/script
  - label: Include script files
    url: /functions/yaml/include
cast:
  file: /casts/examples/starlark-script/summarize.cast
  title: Custom CLI app with styled Atmos output
---

# A custom CLI app built with Atmos

Build a CLI app that reads a JSON service manifest and reports replica totals.
The program is written in the Atmos Automation Language, a Python-like language
based on Starlark. Atmos supplies the interpreter, file access, JSON decoding,
and terminal output. Run it with
`atmos ./summarize.star services.json`, or use the `#!/usr/bin/env atmos` shebang
to execute it directly. This example needs only Atmos.

From this directory, with Atmos on `PATH`:

```shell
./summarize.star services.json
```

The app reports each service's replica count with `ui.info` and highlights
the total of five replicas with `ui.success`. Its
argument comes from `ctx.args`; `fs.read_file` and `json.decode` are built in.

On Windows, or without the executable bit, run `atmos ./summarize.star services.json`.
Input paths are relative to your working directory.

For a script with a declared command-line interface, run:

```shell
./capacity.star api --replicas 3
./capacity.star --help
```

`capacity.star` declares a required service argument and an integer flag with a
default. Atmos parses the inputs, prints help, and runs validation before calling
`main(args, flags)`. The callback reads the parsed dictionaries directly.

This custom CLI app declares its own interface with `cli.command` inside
the script. To add a project-specific subcommand such as `atmos capacity`, define
it in `atmos.yaml` instead; see the
[custom command example](https://atmos.tools/examples/starlark-commands).

See the [standalone script reference](https://atmos.tools/steps/type/script#standalone-scripts).
