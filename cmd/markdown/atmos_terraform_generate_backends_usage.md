- Backend file template (the file path, file name, and file extension)

```shell
 $ atmos terraform generate backends --file-template {component-path}/{tenant}/{environment}-{stage}.tf.json --format json
```
- Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names

```shell
 $ atmos terraform generate backends --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
```

- Only generate the backend files for the specified `atmos` components (comma-separated values).

```shell
 $ atmos terraform generate backends --file-template <file_template> --components <component1>,<component2>
```

- Specify the format for the output file.

```shell
 $ atmos terraform generate backends --format=hcl|json|backend-config
```
