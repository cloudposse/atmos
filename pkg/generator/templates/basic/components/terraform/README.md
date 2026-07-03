# Terraform Components

Put your Terraform root modules here, one directory per component.

For example, create `components/terraform/my-component/` with your
`main.tf`, `variables.tf`, and `outputs.tf`, then configure it in a stack
(see `stacks/dev.yaml`) and run:

```shell
atmos terraform plan my-component -s dev
```

Learn more: https://atmos.tools/core-concepts/components/
