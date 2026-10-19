---
title: CLI-Config Imports
tags: [Configuration]
related_docs:
  - label: "CLI-config imports"
    url: /cli/configuration/imports
  - label: "MCP server configuration"
    url: /cli/configuration/mcp
  - label: "CLI Configuration"
    url: /cli/configuration
---

# CLI-Config Imports Example

This example demonstrates the string and object forms of the top-level [`import:`](https://atmos.tools/cli/configuration/imports)
list in `atmos.yaml`, bringing CLI-config imports toward parity with stack-manifest imports.

## What it demonstrates

- **String imports are optional (fail open).** A failed string import is warned and skipped - the
  historical behavior, unchanged.
- **Object imports are required by default (fail closed).** A failed `{ path: ... }` import aborts
  configuration loading. Use `skip_if_missing: true` to opt back into warn-and-skip.
- **Conditional imports with `when`.** `when: local` resolves an import on workstations and skips
  it in CI (and `when: ci` does the reverse), so an automated `apply` never resolves
  developer-tooling config. For dynamic conditions, `when` also accepts a quoted `"!cel ..."` CEL
  expression that can read the process environment through the `env` map - for example
  `when: '!cel env.DEPLOY_ENV == "prod"'`.
- **Optional MCP servers.** A server marked `optional: true` whose `identity` cannot be resolved is
  skipped with a warning instead of failing - useful for a shared `mcp.servers` config consumed
  across repositories.

## Directory structure

```
examples/cli-config-imports/
├── README.md
├── atmos.yaml                        # import list (string + object + when) and an optional MCP server
└── configs.d/
    ├── base-settings.yaml            # imported as a string entry (optional)
    ├── required-logging.yaml         # imported as a required object entry (fail closed)
    ├── local-tooling.yaml            # imported only off-CI (when: local)
    ├── optional-overrides.yaml       # imported as an optional object entry (skip_if_missing: true)
    └── prod-overrides.yaml           # imported only when a CEL condition matches (DEPLOY_ENV=prod)
```

## Try it

Show the merged configuration (the imported settings are deep-merged in order):

```shell
atmos --chdir examples/cli-config-imports describe config --format yaml
```

See conditional loading in action - `settings.terminal.syntax_highlighting.enabled` is `true` on a
workstation (the `when: local` import loads) and `false` in CI (it is skipped):

```shell
# Workstation
atmos --chdir examples/cli-config-imports describe config --format yaml | grep -A1 syntax_highlighting

# CI (simulated)
GITHUB_ACTIONS=true GITHUB_RUN_ID=1 atmos --chdir examples/cli-config-imports describe config --format yaml | grep -A1 syntax_highlighting
```

See the dynamic CEL condition - `settings.terminal.max_width` is `120` by default and `160` only
when `DEPLOY_ENV=prod` (the `when: '!cel env.DEPLOY_ENV == "prod"'` import loads):

```shell
atmos --chdir examples/cli-config-imports describe config --format yaml | grep max_width
DEPLOY_ENV=prod atmos --chdir examples/cli-config-imports describe config --format yaml | grep max_width
```

See the fail-closed behavior - remove the required import's target and the load aborts:

```shell
# temporarily remove a required import target
mv examples/cli-config-imports/configs.d/required-logging.yaml /tmp/
atmos --chdir examples/cli-config-imports describe config   # aborts: "required config import failed to load"
# restore
mv /tmp/required-logging.yaml examples/cli-config-imports/configs.d/
```
