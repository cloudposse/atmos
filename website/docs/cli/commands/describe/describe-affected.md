---
title: atmos describe affected
sidebar_label: affected
sidebar_class_name: command
id: affected
description: This command produces a list of the affected Atmos components and stacks given two Git commits.
---

:::note Purpose
Use this command to show a list of the affected Atmos components and stacks given two Git commits.
:::

<br/>

:::info
For the first commit, the command assumes that the repo root is a Git checkout.

The second commit is specified on the command line by using the `--ref` or `--sha` flags.

If the flags are not provided, the `ref` will be the default branch (e.g. `main`) and the `sha` will point to the `HEAD` of the branch.
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
atmos describe affected --verbose=true
atmos describe affected --ref refs/heads/main
atmos describe affected --ref refs/heads/my-new-branch --verbose=true
atmos describe affected --ref refs/heads/main --format json
atmos describe affected --ref refs/tags/v1.16.0 --file affected.yaml --format yaml
atmos describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073 --file affected.json
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

## Output

The command outputs a list of objects (in JSON or YAML formats).

Each object has the following schema:

```json
{
  "stack": "....",
  "component_type": "....",
  "component": "....",
  "affected": "....."
}
```

where:

- `stack` is the affected Atmos stack
- `component` is the affected Atmos component in the stack
- `component_type` is the type of the affected Atmos component (`terraform` or `helmfile`)
- `affected` shows what was changed for the component. The possible values are:

  - `vars` - the `vars` component section in the stack config has been modified
  - `env` - the `env` component section in the stack config has been modified
  - `settings` - the `settings` component section in the stack config has been modified
  - `metadata` - the `metadata` component section in the stack config has been modified
  - `terraform` - the Terraform component (Terraform files) that the affected Atmos component provisions has been changed
  - `helmfile` - the Helmfile component (Helmfile files) that the affected Atmos component provisions has been changed

<br/>

For example:

```json
[
  {
    "stack": "tenant2-ue2-staging",
    "component_type": "terraform",
    "component": "infra/vpc",
    "affected": "terraform"
  },
  {
    "stack": "tenant1-ue2-prod",
    "component_type": "terraform",
    "component": "test/test-component-override-3",
    "affected": "env"
  },
  {
    "stack": "tenant1-ue2-dev",
    "component_type": "terraform",
    "component": "test/test-component-override-3",
    "affected": "vars"
  }
]
```
