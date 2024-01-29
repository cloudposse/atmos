---
title: atmos help
sidebar_label: help
sidebar_class_name: command
order: 10
---

## Usage

```shell
atmos help
atmos --help
atmos -h
```

<br/>

The `atmos help` starts an interactive help UI in the terminal:

![`atmos help` command](/img/cli/help/atmos-help-command.png)

<br/>

The `atmos --help` and `atmos -h` commands show help for all Atmos CLI commands:

![`atmos --help` command](/img/cli/help/atmos-help-command-2.png)

<br/>

```console
'atmos' is a universal tool for DevOps and cloud automation used for provisioning, 
managing and orchestrating workflows across various toolchains

Usage:
  atmos [flags]
  atmos [command]

Examples:
atmos

Available Commands:
  atlantis        Execute 'atlantis' commands
  aws             Execute 'aws' commands
  completion      Generate completion script for Bash, Zsh, Fish and PowerShell
  describe        Execute 'describe' commands
  helmfile        Execute 'helmfile' commands
  help            Help about any command
  list            Execute 'atmos list' commands
  play            This command plays games
  set-eks-cluster Download 'kubeconfig' and set EKS cluster.

Example usage:
  atmos set-eks-cluster eks/cluster -s tenant1-ue1-dev -r admin
  atmos set-eks-cluster eks/cluster -s tenant2-uw2-prod --role reader

  show            Execute 'show' commands
  terraform       Execute 'terraform' commands
  tf              Execute 'terraform' commands
  validate        Execute 'validate' commands
  vendor          Execute 'vendor' commands
  version         Print the CLI version
  workflow        Execute a workflow

Flags:
  -h, --help   help for atmos

Use "atmos [command] --help" for more information about a command.
```

<br/>

## Examples

```shell
atmos help               # Starts an interactive help UI in the terminal
atmos --help             # Shows help for all Atmos CLI commands
atmos -h                 # Shows help for all Atmos CLI commands
atmos atlantis --help    # Executes 'atlantis' commands
atmos aws --help         # Executes 'aws' commands
atmos completion --help  # Executes 'completion' commands
atmos describe --help    # Executes 'describe' commands
atmos helmfile --help    # Executes 'helmfile' commands
atmos terraform --help   # Executes 'terraform' commands
atmos validate --help    # Executes 'validate' commands
atmos vendor --help      # Executes 'vendor' commands
atmos workflow --help    # Executes 'workflow' commands
```
