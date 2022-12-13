---
title: atmos describe affected
sidebar_label: affected
sidebar_class_name: command
id: affected
description: This command produces a list of the affected Atmos components and stacks given two Git commits.
---

:::note Purpose
Use this command to show a list of the affected Atmos components and stacks given two Git commits.
For the first commit, the command assumes that the Atmos root is a Git checkout.
The second commit SHA is specified on the command line.
:::

## Usage

```shell
atmos describe affected [options]
```

<br/>

:::tip
Run `atmos describe affected --help` to see all the available options
:::

## Examples

```shell
atmos describe affected --base origin/main
atmos describe affected --base origin/main --format json
atmos describe affected --base origin/main --file affected.json
atmos describe affected --base origin/main --file affected.yaml --format yaml
```

## Flags

| Flag       | Description                                                     | Required |
|:-----------|:----------------------------------------------------------------|:---------|
| `--base`   | The SHA of a Git commit to compare the current Git checkout to  | yes      |
| `--file`   | If specified, write the result to the file                      | no       |
| `--format` | Specify the output format: `json` or `yaml` (`json` is default) | no       |
