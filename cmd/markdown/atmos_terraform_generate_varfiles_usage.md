- Varfile file template (the file path, file name, and file extension)

```bash
 $ atmos terraform generate varfile --file-template {component-path}/{tenant}/{environment}-{stage}.tf.json --format json
```shell

- Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names

```bash
 $ atmos terraform generate varfile --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
```

- Only generate the `.tfvar` files for the specified `atmos` components (comma-separated values).

```bash
 $ atmos terraform generate varfile --file-template <file_template> --components <component1>,<component2>
```shell

- Specify the format for the output file.

```bash
 $ atmos terraform generate varfile --format=hcl|json|backend-config
```
