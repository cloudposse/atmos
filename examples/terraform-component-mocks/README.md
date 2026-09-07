# Terraform component mocks

This example lets `app` consume `vpc`'s output either from real Terraform state or from a component-owned mock.

Create the producer's local state and use the normal lookup path:

```shell
atmos terraform apply vpc -s dev
atmos terraform plan app -s dev
```

That path passes the real `vpc-real` Terraform output into `app`.

Skip state entirely and use the explicit mock path:

```shell
atmos describe component app -s dev --use-mocks
atmos terraform plan app -s dev --use-mocks
```

The mocked commands pass `vpc-local` from `vpc.mocks.vpc_id` into `app`. `--use-mocks` affects only Terraform state/output YAML functions; it does not mock Terraform resources or providers.
