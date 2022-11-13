---
title: atmos describe config
sidebar_label: describe config
---

Execute `describe config` command.

```shell
atmos describe config [options]
```

This command shows the final (deep-merged) CLI configuration (from `atmos.yaml` file(s)).

## Examples

```shell
atmos describe config
atmos describe config -f yaml
atmos describe config --format yaml
atmos describe config -f json
```

## Flags

| Flag        | Description                                | Alias | Required |
|:------------|:-------------------------------------------|:------|:---------|
| `--format`  | Output format: `json` (default) or `yaml`) | `-f`  | no       |
