- Write the result to file

```
$ atmos describe stacks --file=stacks.yaml
```

- Specify the output format

```
$ atmos describe stacks --format=yaml|json
```

- Filter by a specific stack

```
$ atmos describe stacks -s <stack>
```

- Filter by specific `atmos` components

```
$ atmos describe stacks --components=<component1>,<component2>
```

- Filter by specific component types

```
$ atmos describe stacks --component-types=terraform|helmfile
```

- Output only the specified component sections

```
$ atmos describe stacks --sections=vars,settings
```

- Enable/disable Go template processing in Atmos stack manifests when executing the command

```
$ atmos describe stacks --process-templates=false
```

- Enable/disable YAML functions processing in Atmos stack manifests when executing the command

```
$ atmos describe stacks --process-functions=false
```

- Include stacks with no components in the output

```
$ atmos describe stacks --include-empty-stacks
```

- Skip executing a YAML function in the Atmos stack manifests when executing the command

```
$ atmos describe stacks --skip=terraform.output
```