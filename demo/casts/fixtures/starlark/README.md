# Starlark custom command

Run from this directory with an Atmos build that supports embedded Starlark:

```console
atmos release-plan api -s dev
```

The command loads `scripts/plan.star` with `!include`. That program loads an
ordinary function from `scripts/components.star` and invokes it for three custom
application components, with at most two tasks running at once. Each task has a
three-attempt retry policy and a ten-second timeout. This successful recording
does not exercise the retry path.

The command reads inherited versions and environment values, applies each
component's replica override, and prints the joined results in input order. It
performs no deployment and requires no cloud credentials or external interpreter.

Regenerate and validate the recording from the repository root:

```console
atmos --chdir=demo/casts casts generate demo fixtures starlark release-plan
atmos --chdir=demo/casts casts validate demo fixtures starlark release-plan
```
