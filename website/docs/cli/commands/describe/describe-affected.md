---
title: atmos describe affected
sidebar_label: affected
sidebar_class_name: command
id: affected
description: This command produces a list of the affected Atmos components and stacks given two Git commits.
---

:::note Purpose
Use this command to show a list of the affected Atmos components and stacks given two Git commits.

For the first commit, the command assumes that the repo root is a Git checkout.

The second commit is specified on the command line using the `--ref` and `--sha` flags.
If the flags are not provided, it will clone the HEAD of the default branch.
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
atmos describe affected
atmos describe affected --ref refs/heads/main
atmos describe affected --ref refs/heads/main --format json
atmos describe affected --ref refs/tags/v1.16.0 --file affected.yaml --format yaml
atmos describe affected --ref refs/heads/my-new-branch --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073
atmos describe affected --ref refs/heads/my-new-branch --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073 --file affected.json
atmos describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073
```

## Flags

| Flag        | Description                                                                                                                                                     | Required |
|:------------|:----------------------------------------------------------------------------------------------------------------------------------------------------------------|:---------|
| `--ref`     | Git reference with which to compare the current branch. Refer to [Git References](https://git-scm.com/book/en/v2/Git-Internals-Git-References) for more details | no       |
| `--sha`     | Git commit SHA with which to compare the current branch                                                                                                         | no       |
| `--file`    | If specified, write the result to the file                                                                                                                      | no       |
| `--format`  | Specify the output format: `json` or `yaml` (`json` is default)                                                                                                 | no       |
| `--verbose` | Print more detailed output when cloning and checking out the Git repository                                                                                     | no       |
