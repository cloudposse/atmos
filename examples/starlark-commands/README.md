---
related_docs:
  - label: Custom commands
    url: /cli/configuration/commands
  - label: Embedded Starlark
    url: /steps/type/script#embedded-starlark
cast:
  file: /casts/examples/starlark-commands/capacity.cast
  title: A custom command with flags and Starlark logic
---

# A custom command with Atmos Automation

Give an Atmos Automation Language program a discoverable CLI with a named flag.
The Python-like language is built on Starlark and embedded in Atmos. Atmos handles flag
parsing and help; Starlark converts the value and checks that the count is positive.
No stacks, components, or external tools are needed.

From this directory:

```shell
atmos capacity --replicas 3
```

The result is `3 replicas x 4 workers = 12 workers`. Run `atmos capacity --help` to
see the generated flag documentation, or omit the flag to use two replicas.

The step passes the parsed flag through `env` rather than interpolating user input
into script source.

See [custom commands](https://atmos.tools/cli/configuration/commands) and the
[Starlark reference](https://atmos.tools/steps/type/script#embedded-starlark).
