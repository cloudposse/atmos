# Atmos Cast Demos

This directory contains deterministic fixtures and workflows for regenerating committed website asciicasts.

Curated casts are written to `website/static/casts` so the website serves them from `/casts/...`.
These fixtures are intentionally separate from `examples/` and `tests/fixtures/`.

Run these commands with the process working directory set to `demo/casts`.

Regenerate one cast:

```sh
atmos casts generate terraform-plan
atmos casts generate vendor-pull
atmos casts generate list-stacks
atmos casts generate emulator-aws-lifecycle
atmos casts generate featured
```

Regenerate every committed cast:

```sh
atmos casts generate all
```

The command provisions demo fixtures into `../../.context/cast-fixtures` before recording so
the generated casts are reproducible without mutating the source examples.

`casts generate all` is intentionally the slow path. Prefer the per-demo commands
while tuning recordings.
