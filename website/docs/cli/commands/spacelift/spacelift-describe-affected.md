---
title: atmos spacelift describe affected
sidebar_label: describe affected
sidebar_class_name: command
id: describe-affected
description: Use this command to generate a list of the affected Spacelift stacks given two Git commits.
---

:::note Purpose
Use this command to show a list of the affected Spacelift stacks given two Git commits.
:::

## Description

The command uses two different Git commits to produce a list of affected Spacelift stacks.

For the first commit, the command assumes that the current repo root is a Git checkout. An error will be thrown if the current repo is not a Git
repository (`.git` folder does not exist or is configured incorrectly).

The second commit is specified on the command line by using
the `--ref` ([Git References](https://git-scm.com/book/en/v2/Git-Internals-Git-References)) or `--sha` (commit SHA) flags.

Either `--ref` or `--sha` should be used. If both flags are provided at the same time, the command will first clone the remote branch pointed to by
the `--ref` flag and then checkout the Git commit pointed to by the `--sha` flag (`--sha` flag overrides `--ref` flag).

If the flags are not provided, the `ref` will be set automatically to the reference to the default branch (e.g. `main`) and the commit SHA will point
to the `HEAD` of the branch.

The command works by:

- Cloning the branch (`--ref`) or checking out the commit (`--sha`) of the remote target branch
- Deep merging all stack configurations for both the current working branch and the target branch
- Looking for changes in the component directories
- Comparing each section of the stack configuration looking for differences
- Output a JSON or YAML document consisting of a list of affected Spacelift stacks

Since Atmos first checks the component folders for changes, if it finds any affected files, it will mark all Spacelift stacks as
affected. Atmos will then skip evaluating those stacks for differences since we already know that they are affected.

<br/>

```shell
> atmos spacelift describe affected --verbose=true

Cloning repo 'https://github.com/cloudposse/atmos' into the temp dir '/var/folders/g5/lbvzy_ld2hx4mgrgyp19bvb00000gn/T/16710736261366892599'

Checking out the HEAD of the default branch ...

Enumerating objects: 4215, done.
Counting objects: 100% (1157/1157), done.
Compressing objects: 100% (576/576), done.
Total 4215 (delta 658), reused 911 (delta 511), pack-reused 3058

Checked out Git ref 'refs/heads/master'

Current working repo HEAD: 7d37c1e890514479fae404d13841a2754be70cbf refs/heads/describe-affected
Remote repo HEAD: 40210e8d365d3d88ac13c0778c0867b679bbba69 refs/heads/master

Changed files:

examples/complete/components/terraform/infra/vpc/main.tf
internal/exec/spacelift_describe_affected.go
website/docs/cli/commands/spacelift/spacelift_describe_affected.md

Affected Spacelift stacks:

[
  "tenant1-ue2-dev-infra-vpc",
  "tenant1-ue2-prod-infra-vpc",
  "tenant1-ue2-staging-infra-vpc"
]
```

## Usage

```shell
atmos spacelift describe affected [options]
```

<br/>

:::tip
Run `atmos spacelift describe affected --help` to see all the available options
:::

## Examples

```shell
atmos spacelift describe affected
atmos spacelift describe affected --verbose=true
atmos spacelift describe affected --ref refs/heads/main
atmos spacelift describe affected --ref refs/heads/my-new-branch --verbose=true
atmos spacelift describe affected --ref refs/heads/main --format json
atmos spacelift describe affected --ref refs/tags/v1.16.0 --file affected.yaml --format yaml
atmos spacelift describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073 --file affected.json
atmos spacelift describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073
atmos spacelift describe affected --ssh-key <path_to_ssh_key>
atmos spacelift describe affected --ssh-key <path_to_ssh_key> --ssh-key-password <password>
```

## Flags

| Flag                 | Description                                                                                                                   | Required |
|:---------------------|:------------------------------------------------------------------------------------------------------------------------------|:---------|
| `--ref`              | [Git Reference](https://git-scm.com/book/en/v2/Git-Internals-Git-References) with which to compare the current working branch | no       |
| `--sha`              | Git commit SHA with which to compare the current working branch                                                               | no       |
| `--file`             | If specified, write the result to the file                                                                                    | no       |
| `--format`           | Specify the output format: `json` or `yaml` (`json` is default)                                                               | no       |
| `--verbose`          | Print more detailed output when cloning and checking out the Git repository<br/>and processing the result                     | no       |
| `--ssh-key`          | Path to PEM-encoded private key to clone private repos using SSH                                                              | no       |
| `--ssh-key-password` | Encryption password for the PEM-encoded private key if the key contains<br/>a password-encrypted PEM block                    | no       |
