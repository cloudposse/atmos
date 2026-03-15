- Lint all stacks with default settings

```shell
$ atmos lint stacks
```

- Output as JSON for downstream tooling

```shell
$ atmos lint stacks --format json
```

- Run only specific rules

```shell
$ atmos lint stacks --rule L-09,L-04
```

- Only report errors (suppress warnings and info)

```shell
$ atmos lint stacks --severity error
```

- Scope to a specific stack

```shell
$ atmos lint stacks --stack plat-ue2-prod
```
