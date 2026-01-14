- Write the result to file

```shell
 $ atmos describe stacks --file=stacks.yaml
```

- Specify the output format

```shell
 $ atmos describe stacks --format=yaml|json
```

- Filter by a specific stack

```shell
 $ atmos describe stacks -s <stack>
```

- Filter by specific `atmos` components

```shell
 $ atmos describe stacks --components=<component1>,<component2>
```

- Filter by specific component types

```shell
 $ atmos describe stacks --component-types=terraform|helmfile
```

- Output only the specified component sections

```shell
 $ atmos describe stacks --sections=vars,settings
```

- Enable/disable Go template processing in Atmos stack manifests when executing the command

```shell
 $ atmos describe stacks --process-templates=false
```

- Enable/disable YAML functions processing in Atmos stack manifests when executing the command

```shell
 $ atmos describe stacks --process-functions=false
```

- Include stacks with no components in the output

```shell
 $ atmos describe stacks --include-empty-stacks
```

- Skip executing a YAML function in the Atmos stack manifests when executing the command

```shell
 $ atmos describe stacks --skip=terraform.output
```
