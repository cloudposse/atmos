---
title: Parallel Steps
tags: [Automation]
cast:
  file: /casts/examples/parallel-steps/control-steps.cast
  title: atmos parallel workflow steps
---

# Example: Parallel Workflow Steps

This example demonstrates `parallel` and `matrix` workflow control steps with `needs`, configurable output, and failure behavior.

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
