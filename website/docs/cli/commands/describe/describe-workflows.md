---
title: atmos describe workflows
sidebar_label: workflows
sidebar_class_name: command
id: workflows
description: Use this command to show all configured Atmos workflows.
---

:::note Purpose
Use this command to show all configured Atmos workflows.
:::

## Usage

Execute the `describe workflows` command like this:

```shell
atmos describe workflows [options]
```

<br/>

:::tip
Run `atmos describe workflows --help` to see all the available options
:::

## Examples

```shell
atmos describe workflows
atmos describe workflows --output map
atmos describe workflows -o list
atmos describe workflows -o list --format json
atmos describe workflows -o list -f yaml
atmos describe workflows -f json
```

## Flags

| Flag       | Description                                                     | Alias | Required |
|:-----------|:----------------------------------------------------------------|:------|:---------|
| `--format` | Specify the output format: `yaml` or `json` (`yaml` is default) | `-f`  | no       |
| `--output` | Specify the output type: `list` or `map` (`list` is default)    | `-o`  | no       |

<br/>

When `--output list` flag is passed (default), the output of the command is a list of objects. Each object (list item) has the following schema:

- `file` - the workflow manifest file name
- `workflow` - the name of the workflow defined in the workflow manifest file

For example:

```shell
atmos describe workflows
atmos describe workflows -o list
```

```yaml
- file: compliance.yaml
  workflow: deploy/aws-config/global-collector
- file: compliance.yaml
  workflow: deploy/aws-config/superadmin
- file: compliance.yaml
  workflow: destroy/aws-config/global-collector
- file: compliance.yaml
  workflow: destroy/aws-config/superadmin
- file: datadog.yaml
  workflow: deploy/datadog-integration
- file: helpers.yaml
  workflow: save/docker-config-json
- file: networking.yaml
  workflow: apply-all-components
- file: networking.yaml
  workflow: plan-all-vpc-components
- file: networking.yaml
  workflow: plan-all-vpc-flow-logs-bucket-components
- file: vpc.yaml
  workflow: vpc-tgw-eks
```

<br/>

When `--output map` flag is passed, the output of the command is a map of workflow manifest file names to a list of workflows defined in each
manifest. For example:

```shell
atmos describe workflows -o map
```

```yaml
compliance.yaml:
  - deploy/aws-config/global-collector
  - deploy/aws-config/superadmin
  - destroy/aws-config/global-collector
  - destroy/aws-config/superadmin
datadog.yaml:
  - deploy/datadog-integration
helpers.yaml:
  - save/docker-config-json
networking.yaml:
  - apply-all-components
  - plan-all-vpc-components
  - plan-all-vpc-flow-logs-bucket-components
vpc.yaml:
  - vpc-tgw-eks
```

<br/>

:::tip
Use the [atmos workflow](/cli/commands/workflow) CLI command to execute an Atmos workflow
:::
