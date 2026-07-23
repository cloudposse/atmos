# Atmos Cast Demos

This directory contains deterministic fixtures and workflows for regenerating committed website asciicasts.

Curated casts are written to `website/static/casts` so the website serves them from `/casts/...`.
Each committed cast path mirrors the repo-relative source path, followed by an action-oriented cast name:

```text
website/static/casts/<source-path>/<cast-name>.cast
/casts/<source-path>/<cast-name>.cast
```

The matching custom command uses the same source path segments and cast name:

```text
atmos casts generate <source-path-segments> <cast-name>
atmos casts validate <source-path-segments> <cast-name>
```

For example, the quick start simple list-and-plan recording is generated with
`atmos casts generate examples quick-start-simple list-and-plan` and committed as
`website/static/casts/examples/quick-start-simple/list-and-plan.cast`.
These fixtures are intentionally separate from `examples/` and `tests/fixtures/`.

`atmos.yaml` only defines shared setup. Generate and validate commands live
under the default-imported `atmos.d` tree. Per-cast command files mirror the
cast source path and cast name:

```text
atmos.d/<source-path>/<cast-name>.yaml
```

Aggregate commands live next to the demos they group:

```text
atmos.d/<source-path>/all.yaml
```

For example, `atmos.d/examples/all.yaml` defines `casts generate examples`,
and `atmos.d/demo/fixtures/basic/all.yaml` defines
`casts generate demo fixtures basic`. Do not use numbered filenames for cast
commands; the file path should describe the command group or demo it owns.

Run these commands with the process working directory set to `demo/casts`.

Generated `.cast` files use asciicast v3. For `type: cast` recordings in
`mode: steps`, use top-level `title:` for the human-readable cast label. Reserve
top-level `command:` for recordings that execute one real command or session.
Nested shell steps still use `command:` because those commands are actually run.

Regenerate one cast:

```sh
atmos casts generate examples quick-start-simple list-and-plan
atmos casts generate examples sops-secrets secret-lifecycle
atmos casts generate cli commands cast usage
atmos casts generate cli commands cast play
atmos casts generate cli commands cast render
atmos casts generate demo fixtures native-terraform plan
atmos casts generate demo fixtures demo-vendoring pull
atmos casts generate demo fixtures basic list-stacks
```

Regenerate every committed cast:

```sh
atmos casts generate all
atmos casts generate examples
```

The command provisions demo fixtures into `../../.context/cast-fixtures` before recording so
the generated casts are reproducible without mutating the source examples.

`casts generate all` is intentionally the slow path. Prefer the per-demo commands
while tuning recordings.
