# Relative source context

This provider-free fixture compares direct execution and an isolated Atmos workdir
for the same nested Terraform component. The component reads a sibling local module
and a template outside its own directory. Both plans should report the markers
from the nested copy, rather than the outer copy.

Create an outer copy and a nested copy with different module and template markers.
The nested copy makes an accidental read from the outer tree observable. Run each
plan from the nested copy and check both values in its output:

```sh
fixture="$(pwd)/tests/fixtures/scenarios/terraform-registry-field-test/relative-context"
run="$(mktemp -d)"
cp -R "$fixture/." "$run/"
mkdir -p "$run/nested"
cp -R "$fixture/." "$run/nested/"
printf 'output "value" { value = "outer-module" }\n' > "$run/modules/marker/main.tf"
printf 'outer-template\n' > "$run/templates/message.txt"
printf 'output "value" { value = "nested-module" }\n' > "$run/nested/modules/marker/main.tf"
printf 'nested-template\n' > "$run/nested/templates/message.txt"
cd "$run/nested"
for component in direct isolated; do
  atmos terraform plan "$component" -s dev --identity=false -no-color > "$run/$component.plan" || exit 1
  grep -F 'nested-module' "$run/$component.plan" >/dev/null || exit 1
  grep -F 'nested-template' "$run/$component.plan" >/dev/null || exit 1
done
```

Choose the engine with `ATMOS_COMPONENTS_TERRAFORM_COMMAND` (Terraform 1.15.6 or
OpenTofu 1.12.5 for the repair comparison). Use a new run directory for each engine.
No remote provider, cloud credentials, apply, or backend provisioning is required.
No symlinks or platform-specific paths are used.

The existing workdir layout flattens the nested component path. Retaining a fixed
workdir depth does not preserve the four parent-directory steps in this fixture.
The desired repair must preserve both output values without rewriting Terraform
source, reading another checkout's sibling files, or requiring symbolic links.

This is a correctness reproducer, not a DAG performance or Windows certification.
