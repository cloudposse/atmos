# stack-lifecycle-field-test

Fixture for `atmos stack create`, `atmos stack deploy`, and `atmos stack destroy`.

- `dev`: a Terraform chain (`network` → `database`/`cache` → `app`) plus a custom
  `script` component that depends on `app`. Terraform components use only the
  built-in `terraform_data` resource with a local backend, so real runs need no
  cloud credentials or provider downloads.
- `prod`: an `app` that depends on the `dev` network across stacks, which
  exercises the destroy surviving-consumer check.
- `platform`: Helmfile and Packer components for previews. Helmfile needs a
  Kubernetes context for a real run, and Packer has no lifecycle handler.
- `scaffolds/`: recipes for `stack create` that use scaffold hooks, ignore the
  reserved stack inputs, or render extra stack files from a matrix.

Set `ORDER_LOG` to an absolute file path before a real run. Terraform
provisioners (a POSIX shell `local-exec`), the `script` custom component, and
step hooks append one line per operation, which makes execution order and hook
firing observable. Run real operations only in a disposable copy of this
directory, because Terraform writes local state next to the component.

```shell
export ORDER_LOG="$PWD/order.log"
atmos stack deploy --all --dry-run
atmos stack deploy --all --stack=dev
atmos stack destroy --all --stack=dev --include-dependents --auto-approve
```
