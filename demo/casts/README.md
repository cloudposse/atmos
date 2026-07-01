# Atmos Cast Demos

This directory contains deterministic fixtures and workflows for regenerating committed website asciicasts.

Curated casts are written to `website/static/casts` so the website serves them from `/casts/...`.
These fixtures are intentionally separate from `examples/` and `tests/fixtures/`.

Regenerate one cast from the repository root:

```sh
ATMOS_CLI_CONFIG_PATH=demo/casts atmos casts generate terraform-plan
ATMOS_CLI_CONFIG_PATH=demo/casts atmos casts generate vendor-pull
ATMOS_CLI_CONFIG_PATH=demo/casts atmos casts generate list-stacks
ATMOS_CLI_CONFIG_PATH=demo/casts atmos casts generate emulator-aws-lifecycle
```

Regenerate every committed cast:

```sh
ATMOS_CLI_CONFIG_PATH=demo/casts atmos casts generate all
```

The command copies demo fixtures into `.context/cast-fixtures` before recording so
the generated casts are reproducible without mutating the source examples.

`casts generate all` is intentionally the slow path. Prefer the per-demo commands
while tuning recordings.
