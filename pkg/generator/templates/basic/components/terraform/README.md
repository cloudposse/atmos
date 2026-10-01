# Terraform Components

Put your Terraform root modules here, one directory per component.

`greeting/` is a real example already wired up in `stacks/dev.yaml` — a
local-only component (no cloud account or emulator needed) showing the shape
a component takes: `main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`.
Try it:

```shell
atmos terraform apply greeting -s dev
```

Add your own components the same way — create
`components/terraform/my-component/`, configure it in a stack (see
`stacks/dev.yaml`), then run:

```shell
atmos terraform plan my-component -s dev
```

Learn more: https://atmos.tools/core-concepts/components/
