- Lint all stacks with default settings

```
$ atmos lint stacks
```

- Output as JSON for downstream tooling

```
$ atmos lint stacks --format json
```

- Run only specific rules

```
$ atmos lint stacks --rule L-09,L-04
```

- Only report errors (suppress warnings and info)

```
$ atmos lint stacks --severity error
```

- Scope to a specific stack

```
$ atmos lint stacks --stack plat-ue2-prod
```
