# Relative source context

This provider-free fixture compares direct execution and an isolated Atmos workdir
for the same nested Terraform component. The component reads a sibling local module
and a template outside its own directory. Both plans should report `sibling-module`
and `sibling-template`.

Copy the fixture to a fresh disposable directory before running. Initialization and
planning write generated files; do not run them in this reusable source fixture.
Choose the engine with `ATMOS_COMPONENTS_TERRAFORM_COMMAND` (Terraform 1.15.6 or
OpenTofu 1.12.5 for the repair comparison), then run:

```text
atmos terraform plan direct -s dev --identity=false -no-color
atmos terraform plan isolated -s dev --identity=false -no-color
```

Use separate fresh copies for each command when comparing cold execution. No remote
provider, cloud credentials, apply, or backend provisioning is required. No symlinks
or platform-specific paths are used.

The existing workdir layout flattens the nested component path. Retaining a fixed
workdir depth does not preserve the four parent-directory steps in this fixture.
The desired repair must preserve both output values without rewriting Terraform
source, reading another checkout's sibling files, or requiring symbolic links.

This is a correctness reproducer, not a DAG performance or Windows certification.
