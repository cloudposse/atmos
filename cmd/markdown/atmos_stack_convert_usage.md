- Convert a YAML stack to HCL format (output to stdout)

```shell
atmos stack convert stacks/prod.yaml --to hcl
```

- Convert a YAML stack to HCL format (output to file)

```shell
atmos stack convert stacks/prod.yaml --to hcl --output prod.hcl
```

- Convert an HCL stack to YAML format

```shell
atmos stack convert stacks/prod.hcl --to yaml
```

- Convert a JSON stack to YAML format with output file

```shell
atmos stack convert stacks/prod.json --to yaml --output prod.yaml
```

- Preview conversion without writing (dry-run)

```shell
atmos stack convert stacks/prod.yaml --to hcl --dry-run
```

- Convert a multi-document YAML file to multi-stack HCL

```shell
atmos stack convert stacks/environments.yaml --to hcl
```
