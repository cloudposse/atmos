---
title: Parallel Steps
tags: [Automation]
cast:
  file: /casts/examples/parallel-steps/control-steps.cast
  title: atmos parallel workflow steps
related_docs:
  - label: "Run workflows"
    url: /cli/commands/workflow
  - label: "Parallel steps"
    url: /steps/type/parallel
  - label: "Matrix steps"
    url: /steps/type/matrix
---

# Example: Parallel Workflow Steps

This example demonstrates [`parallel`](https://atmos.tools/steps/type/parallel) and [`matrix`](https://atmos.tools/steps/type/matrix) workflow control steps with `needs`, configurable output, and failure behavior.

## Try It

Requires a POSIX shell, or WSL on Windows, because the example workflows run shell commands.

```shell
cd examples/parallel-steps

atmos workflow checks -f parallel
atmos workflow prefixed -f parallel
atmos workflow matrix -f parallel
```

## What It Shows

- `parallel` runs independent child steps concurrently.
- `needs` makes one child wait for sibling steps.
- `output.mode: grouped` captures each child and prints labeled blocks.
- `output.mode: prefixed` streams child output live with line prefixes.
- `matrix` expands literal axes and runs generated shell steps through the same scheduler.
