# Terraform registry source and workdir field fixture

Derived from the existing `source-provisioner-workdir-nested`, `workdir-global-default`,
and `terraform-output-jit-workdir` scenarios. Uses real `terraform_data` resources,
local state, generated files, and local module sources; no cloud credentials or
external providers are needed.

Copy this directory into a disposable Git repository before running.
`terraform apply source-local -s dev` exercises source provisioning;
`terraform apply local-alias -s dev` exercises local workdir provisioning.
Run each with Terraform and OpenTofu, then repeat to verify warm reuse.

The field harness expands copies with nested/colliding names, component-level
workdir opt-outs, local Git tags/subdirectories, zero-TTL sources, output/state
consumers, and linked Git worktrees. Keep state outside runtime workdirs so source
refresh can be observed without discarding the real local state. Evidence and
the runnable harness live in the investigation run directory.

```shell
atmos terraform apply source-local -s dev -auto-approve --identity=false
atmos terraform plan source-local -s dev -detailed-exitcode --identity=false
atmos terraform apply local-alias -s dev -auto-approve --identity=false
atmos terraform output local-alias -s dev -json --identity=false
atmos terraform source pull source-local -s dev --force
```

For the output command, capture stdout separately and parse the entire stream;
do not strip initialization messages before deciding whether the output is valid
JSON. State lives in `state/` outside the runtime workdir in this fixture. The
investigation also tests the separate case of state inside a refreshed source
workdir; that case must use an independently disposable copy.

The separate vendor interface is exercised with `atmos vendor pull`, followed by
`atmos terraform apply vendor-alias -s dev -auto-approve --identity=false`.
After modifying the module, a normal pull preserves the materialized dependency;
use `atmos vendor pull --refresh-lock` to fetch the modified local source and
apply again to verify its output. The `vendor.yaml` excludes README files.

Run `atmos terraform shell source-local -s dev` in a terminal, inspect `pwd`,
`ATMOS_COMPONENT`, `ATMOS_STACK`, and `TF_CLI_ARGS_plan`, then execute the engine's
native output command and exit. Repeat with `--skip-init`. The field run also
tests explicit `--identity=false`, which currently fails in an auth-free project
on both compared revisions, and tagged Git sources whose runtime is a subdirectory.
