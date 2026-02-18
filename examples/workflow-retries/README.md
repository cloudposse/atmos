# Workflow Retries Example

Demonstrates automatic retry configuration for workflow steps.

## Run

```shell
atmos workflow retry-demo -f retries
```

You'll see "Attempting deployment..." printed 3 times as Atmos retries,
then fails with "max attempts (3) exceeded".

## Learn More

See [Workflow Retries documentation](https://atmos.tools/workflows/).
