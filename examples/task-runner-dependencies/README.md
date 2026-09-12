---
title: Task Runner Dependencies
tags: [Automation]
---

# Task Runner Dependencies

Demonstrates the custom-command and workflow task-runner's dependency system: `dependencies.commands` / `dependencies.workflows` (DAG execution with dedup), `fail_fast` vs `best_effort` sibling-dependency failure modes, step-level `inputs`/`artifacts` freshness checks (checksum, timestamp, and explicit CEL conditions), `preconditions`, the `continue: always` step field, native command aliases/`internal` commands, and constrained flag `values`.

## Run

```shell
# Dependency graph: verify depends on compile (deduped), trd-lint, and unit-test
atmos verify

# fail_fast aborts the slow sibling once step-b-fails fails
atmos release-failfast

# best_effort lets the slow sibling finish despite a failing sibling
atmos release-besteffort

# Command depending on a workflow
atmos deploy

# Freshness: skips its own step when src/**/*.txt is unchanged
atmos freshbuild
atmos freshbuild        # second run is skipped -- inputs unchanged
```

## Learn More

See [Command Dependencies](https://atmos.tools/cli/configuration/commands/dependencies) documentation.
