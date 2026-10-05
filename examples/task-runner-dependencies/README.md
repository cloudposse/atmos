---
related_docs:
  - label: "Custom commands"
    url: /cli/configuration/commands
  - label: "Command dependencies"
    url: /cli/configuration/commands/dependencies
  - label: "Step inputs"
    url: /steps/inputs
  - label: "Step preconditions"
    url: /steps/preconditions
---

# Task Dependencies and Incremental Runs

Define [prerequisites for custom commands](https://atmos.tools/cli/configuration/commands/dependencies) and [workflows](https://atmos.tools/workflows/dependencies) so Atmos runs them in dependency order. This example also demonstrates reusing identical dependencies, checking [input freshness](https://atmos.tools/steps/inputs), and skipping steps whose preconditions are not met.

## Try It

From this directory, run:

```shell
atmos verify
```

The `verify` command depends on compilation, linting, and unit-test commands. The commands write execution records under `logs/`, which let you inspect the order and see which dependencies ran. Several other commands deliberately fail to demonstrate failure handling; inspect `atmos.yaml` before running them.
