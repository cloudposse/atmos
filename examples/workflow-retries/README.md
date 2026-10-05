---
title: Workflow Retries
tags: [Automation]
cast:
  file: /casts/examples/workflow-retries/retry-failure.cast
  title: atmos workflow retries
related_docs:
  - label: "Run workflows"
    url: /cli/commands/workflow
  - label: "Workflow configuration"
    url: /workflows
---

# Workflow Retries Example

Demonstrates automatic [retry configuration](https://atmos.tools/steps/retry) for workflow steps.

## Run

```shell
atmos workflow retry-demo -f retries
```

You'll see "Attempting deployment..." printed 3 times as Atmos retries,
then fails with "max attempts (3) exceeded".

## Learn More

See [Workflow Retries documentation](https://atmos.tools/workflows/).
