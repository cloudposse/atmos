---
related_docs:
  - label: Script steps
    url: /steps/type/script
  - label: Include script files
    url: /functions/yaml/include
cast:
  file: /casts/examples/starlark-script/summarize.cast
  title: One executable file, no extra interpreter
---

# An executable Atmos Automation script

Use Atmos Automation Language, a Python-like language built on Starlark, to
summarize a JSON service manifest with one executable file. The `#!/usr/bin/env atmos`
shebang uses Atmos as the interpreter; no Python or jq installation is needed.

From this directory, with Atmos on `PATH`:

```shell
./summarize.star services.json
```

The script prints each service's replica count and a total of five replicas. Its
argument comes from `ctx.args`; `fs.read_file` and `json.decode` are built in.

On Windows, or without the executable bit, run `atmos ./summarize.star services.json`.
Input paths are relative to your working directory.

See the [standalone script reference](https://atmos.tools/steps/type/script#standalone-script-spike).
