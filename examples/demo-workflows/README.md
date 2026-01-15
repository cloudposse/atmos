# Example: Demo Workflows

Run multiple Atmos commands in sequence with a single command.

Learn more about [Workflows](https://atmos.tools/core-concepts/workflows/).

## What You'll See

- [Workflow definitions](https://atmos.tools/core-concepts/workflows/workflow-manifest/) in stack manifests
- Chaining multiple `atmos terraform` commands
- Parameterized workflows with arguments

## Try It

```shell
cd examples/demo-workflows

# List available workflows
atmos workflow list

# Run a workflow
atmos workflow deploy -s dev
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Configures workflow base path |
| `stacks/workflows/` | Workflow definitions |
