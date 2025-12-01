- The output format

```shell
 $ atmos describe component <component> -s <stack> --format=yaml|json
```

- Write the result to the file

```shell
 $ atmos describe component <component> -s <stack> --file component.yaml
```

- Enable/disable Go template processing in Atmos stack manifests when executing the command

```shell
 $ atmos describe component <component> -s <stack> --process-templates=false
```

- Enable/disable YAML functions processing in Atmos stack manifests when executing the command

```shell
 $ atmos describe component <component> -s <stack> --process-functions=false
```

- Skip executing a YAML function in the Atmos stack manifests when executing the command

```shell
 $ atmos describe component <component> -s <stack> --skip=terraform.output
```
