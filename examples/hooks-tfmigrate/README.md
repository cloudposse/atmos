# `hooks-tfmigrate`

Demonstrates the **`tfmigrate`** hook kind: a Terraform state migration
hook that runs through `atmos terraform migrate` before Terraform plan and
apply operations.

## What this shows

- `kind: tfmigrate` with `mode: dynamic`.
- `before.terraform.plan` previews the migration with `tfmigrate plan`.
- `before.terraform.apply` applies the migration with `tfmigrate apply`.
- `config: .tfmigrate.hcl` enables tfmigrate history mode so reruns are
  safe after the migration has already been applied.
- Local Terraform state keeps the example self-contained and avoids cloud
  credentials. Terraform workspaces are disabled so both components share the
  same local state file.

## Requirements

- `tofu` (OpenTofu) or Terraform on PATH. This example uses `command: tofu`
  in `atmos.yaml`.
- `tfmigrate` on PATH.
- No cloud credentials needed.

Install tfmigrate with Homebrew:

```bash
brew install minamijoyo/tfmigrate/tfmigrate
```

## Run

Start in this example directory:

```bash
cd examples/hooks-tfmigrate
```

Seed old state with the legacy component. This creates a local state file
containing `random_pet.legacy`.

```bash
atmos terraform apply service-legacy -s test -auto-approve
```

Inspect the migration context that Atmos will pass to tfmigrate:

```bash
atmos terraform migrate list service -s test
```

Preview the refactored component. The hook runs `tfmigrate plan` first.
Because `mode: dynamic` only previews during `before.terraform.plan`, the
Terraform plan can still show the old address moving until apply time.

```bash
atmos terraform plan service -s test
```

Apply the refactored component. The hook runs `tfmigrate apply` before
Terraform apply, moving `random_pet.legacy` to `random_pet.service` in
state. The Terraform apply should then converge without replacing the
random pet.

```bash
atmos terraform apply service -s test
```

Run the plan again. History mode records the applied migration, so tfmigrate
does not try to move the address a second time.

```bash
atmos terraform plan service -s test
```

## Files

- `stacks/deploy/test.yaml` configures the legacy and refactored components.
- `components/terraform/service-legacy/` creates the original state address.
- `components/terraform/service/` contains the refactored address and the
  `tfmigrate` history config. The local history file is written under
  `state/tfmigrate-history/`, which is ignored by git.
- `components/terraform/service/migrations/` contains the migration HCL.

## Notes

This example intentionally keeps the migration config beside the refactored
component because Atmos executes `tfmigrate` from the component working
directory. In production, keep migration paths clear and confirm them with
`atmos terraform migrate list`.
