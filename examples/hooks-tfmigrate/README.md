# `hooks-tfmigrate`

Demonstrates the **`tfmigrate`** hook kind: a Terraform state migration
hook that runs through `atmos terraform migrate` before Terraform plan and
apply operations.

Watch this example as a recorded demo in the
[Terraform state migrations with tfmigrate](https://atmos.tools/blog/terraform-tfmigrate)
changelog post, or in the
[`atmos terraform migrate`](https://atmos.tools/cli/commands/terraform/migrate)
command docs.

## What this shows

- `kind: tfmigrate` with `mode: dynamic`.
- `before.terraform.plan` previews the migration with `tfmigrate plan`.
- `before.terraform.apply` applies the migration with `tfmigrate apply`.
- Zero tfmigrate configuration: with no `.tfmigrate.hcl`, Atmos generates one
  that reuses the component's Terraform backend for tfmigrate history storage,
  so reruns are safe after the migration has already been applied. Here the
  backend is local, so history lands next to the state file; with an S3 or GCS
  backend the history goes to the same bucket under a namespaced key. Provide
  your own `.tfmigrate.hcl` (or the hook's `config:` field) to override.
- Local Terraform state keeps the example self-contained and avoids cloud
  credentials. Terraform workspaces are disabled so both components share the
  same local state file.

## Requirements

Nothing needs to be pre-installed. The components declare `opentofu` and
`tfmigrate` in `dependencies.tools`, so the Atmos toolchain downloads both
automatically on first run (into the git-ignored `.tools/` directory). No
cloud credentials are needed either — the example uses local Terraform state.

If you prefer managing the tools yourself, install them on PATH instead, for
example with Homebrew:

```bash
brew install opentofu minamijoyo/tfmigrate/tfmigrate
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
- `components/terraform/service/` contains the refactored address.
- `components/terraform/service/migrations/` contains the migration HCL.
  Atmos points the generated tfmigrate config at this directory automatically.
  The local history file is written under `state/tfmigrate/`, which is ignored
  by git.

## Notes

Atmos executes `tfmigrate` from the component working directory, so migration
files live beside the component. In production, confirm the migration context
with `atmos terraform migrate list`.
