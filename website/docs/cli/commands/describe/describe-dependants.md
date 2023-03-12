---
title: atmos describe dependants
sidebar_label: dependants
sidebar_class_name: command
id: dependants
description: This command produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component.
---

:::note Purpose
Use this command to show a list of Atmos components in Atmos stacks that depend on the provided Atmos component.
:::

## Description

The command uses two different Git commits to produce a list of affected Atmos components and stacks.

For the first commit, the command assumes that the current repo root is a Git checkout. An error will be thrown if the current repo is not a Git
repository (the `.git` folder does not exist or is configured incorrectly).

The second commit can be specified on the command line by using
the `--ref` ([Git References](https://git-scm.com/book/en/v2/Git-Internals-Git-References)) or `--sha` (commit SHA) flags.

Either `--ref` or `--sha` should be used. If both flags are provided at the same time, the command will first clone the remote branch pointed to by
the `--ref` flag and then checkout the Git commit pointed to by the `--sha` flag (`--sha` flag overrides `--ref` flag).

__NOTE:__ If the flags are not provided, the `ref` will be set automatically to the reference to the default branch (e.g. `main`) and the commit SHA
will point to the `HEAD` of the branch.

If you specify the `--repo-path` flag with the path to the already cloned repository, the command will not clone the target
repository, but instead will use the already cloned one to compare the current branch with. In this case, the `--ref`, `--sha`, `--ssh-key`
and `--ssh-key-password` flags are not used, and an error will be thrown if the `--repo-path` flag and any of the `--ref`, `--sha`, `--ssh-key`
or `--ssh-key-password` flags are provided at the same time.

The command works by:

- Cloning the target branch (`--ref`) or checking out the commit (`--sha`) of the remote target branch, or using the already cloned target repository
  specified by the `--repo-path` flag

- Deep merging all stack configurations for both the current working branch and the remote target branch

- Looking for changes in the component directories

- Comparing each section of the stack configuration looking for differences

- Outputting a JSON or YAML document consisting of a list of the affected components and stacks and what caused it to be affected

Since Atmos first checks the component folders for changes, if it finds any affected files, it will mark all related components and stacks as
affected. Atmos will then skip evaluating those stacks for differences since we already know that they are affected.


<br/>

## Usage

```shell
atmos describe dependants [options]
```

<br/>

:::tip
Run `atmos describe dependants --help` to see all the available options
:::

## Examples

```shell
atmos describe dependants test/test-component -s tenant1-ue2-test-1
atmos describe dependants test/test-component -s tenant1-ue2-dev --format yaml
atmos describe dependants test/test-component -s tenant1-ue2-test-1 -f yaml
atmos describe dependants test/test-component -s tenant1-ue2-test-1 --file dependants.json
atmos describe dependants test/test-component -s tenant1-ue2-test-1 --format yaml --file dependants.yaml
```

## Arguments

| Argument    | Description     | Required |
|:------------|:----------------|:---------|
| `component` | Atmos component | yes      |

## Flags

| Flag       | Description                                         | Alias | Required |
|:-----------|:----------------------------------------------------|:------|:---------|
| `--stack`  | Atmos stack                                         | `-s`  | yes      |
| `--format` | Output format: `json` or `yaml` (`json` is default) | `-f`  | no       |
| `--file`   | If specified, write the result to the file          |       | no       |

## Output

The command outputs a list of objects (in JSON or YAML format).

Each object has the following schema:

```json
{
  "component": "....",
  "component_type": "....",
  "component_path": "....",
  "namespace": "....",
  "tenant": "....",
  "environment": "....",
  "stage": "....",
  "stack": "....",
  "spacelift_stack": ".....",
  "atlantis_project": "....."
}
```

where:

- `component` - the dependant Atmos component in the stack

- `component_type` - the type of the dependant component (`terraform` or `helmfile`)

- `component_path` - the filesystem path to the `terraform` or `helmfile` component

- `namespace` - the `namespace` where the dependant Atmos component is provisioned

- `tenant` - the `tenant` where the dependant Atmos component is provisioned

- `environment` - the `environment` where the dependant Atmos component is provisioned

- `stage` - the `stage` where the dependant Atmos component is provisioned

- `stack` - the Atmos stack where the dependant Atmos component is provisioned

- `spacelift_stack` - the dependant Spacelift stack. It will be included only if the Spacelift workspace is enabled for the dependant Atmos component
  in the Atmos stack in the `settings.spacelift.workspace_enabled` section (either directly in the component's `settings.spacelift.workspace_enabled`
  section or via inheritance)

- `atlantis_project` - the dependant Atlantis project name. It will be included only if the Atlantis integration is configured in
  the `settings.atlantis` section in the stack config. Refer to [Atlantis Integration](/integrations/atlantis.md) for more details

<br/>

:::note

Abstract Atmos components (`metadata.type` is set to `abstract`) are not included in the output since they serve as blueprints for other
Atmos components and are not meant to be provisioned.

:::

## Output Example

```shell
atmos describe dependants test/test-component -s tenant1-ue2-test-1
```

```json
[
  {
    "component": "top-level-component2",
    "component_type": "terraform",
    "component_path": "examples/complete/components/terraform/top-level-component1",
    "namespace": "cp",
    "tenant": "tenant1",
    "environment": "ue2",
    "stage": "test-1",
    "stack": "tenant1-ue2-test-1",
    "atlantis_project": "tenant1-ue2-test-1-top-level-component2"
  },
  {
    "component": "top-level-component1",
    "component_type": "terraform",
    "component_path": "examples/complete/components/terraform/top-level-component1",
    "namespace": "cp",
    "tenant": "tenant1",
    "environment": "ue2",
    "stage": "dev",
    "stack": "tenant1-ue2-dev",
    "spacelift_stack": "tenant1-ue2-dev-top-level-component1",
    "atlantis_project": "tenant1-ue2-dev-top-level-component1"
  }
]
```
