# Task Dependencies and Incremental Runs

Define prerequisites for custom commands and workflows so Atmos runs them in dependency order. This example also demonstrates reusing identical dependencies, checking input freshness, and skipping steps whose preconditions are not met.

## Try It

From this directory, run:

```shell
atmos verify
```

The `verify` command depends on compilation, linting, and unit-test commands. The commands write execution records under `logs/`, which let you inspect the order and see which dependencies ran. Several other commands deliberately fail to demonstrate failure handling; inspect `atmos.yaml` before running them.

## Related Documentation

- [Custom commands](https://atmos.tools/cli/configuration/commands)
- [Command dependencies](https://atmos.tools/cli/configuration/commands/dependencies)
- [Step inputs](https://atmos.tools/steps/inputs)
- [Step preconditions](https://atmos.tools/steps/preconditions)
