---
title: Atmos Alternatives
sidebar_label: Alternatives
---


# General Alternatives

There are many tools in the general category of "task runners" or "workflow automation". Here are some of the alternatives to Atmos, many of which inspired core functionality in Atmos.

## Make / Magefile by Free Software Foundation

https://www.gnu.org/software/make/

Many companies (including Cloud Posse) started by leveraging `make` with `Makefile` and targets to call terraform. Using `make` is tried and true method of orchestrating tools, but it has trouble scaling up to support large projects. We know this because we tried it for ~3 years. Make targets do not support parameterization, which leads to a proliferation of environment variables that are difficult to validate. Makefiles are unintuitive for newcomers because they are first evaluated as a template, and then executed as a script where each line of a target runs in a separate process space.

## Mage / Magefile

https://magefile.org/

Mage is a make/rake-like build tool using native Golang and plain-old `Go` functions. Mage then automatically provides a CLI to call them as Makefile-like runnable targets.

## Gotask / Taskfile

https://github.com/go-task/task


## Variant

https://github.com/mumoshu/variant
https://github.com/mumoshu/variant2 (second generation)

Variant lets you wrap all your scripts and CLIs into a modern CLI and a single-executable that can run anywhere.

:::info
The earliest versions of `atmos` were built on top of [`variant2`](https://github.com/mumoshu/variant2) until we decided to rewrite it from the ground up in pure Go.
:::

## appbuilder by Choria

https://github.com/choria-io/appbuilder


AppBuilder is a tool built in Golang to create a friendly CLI command that wraps your operational tools.


# Terraform Alternatives

There are many tools explicitly designed around how to deploy with Terraform.  

The following is a list of tools that only support Terraform.

:::info
Atmos not only supports Terraform, but can be used to manage any CLI. For example, by combinging custom [subcommands](/core-concepts/subcommands) and [workflows](/core-concepts/workflows) it's possible support any CLI tool (even the ones listed below) or even reimplement core functionality of atmos. That's how extensible it is.
:::

## Terragrunt by Gruntwork

https://github.com/gruntwork-io/terragrunt

Terragrunt is a tool built in Golang that is a thin wrapper for Terraform that provides extra tools for working with multiple Terraform modules.


## Terramate by Mineros

https://github.com/mineiros-io/terramate

Terramate is a tool built in Golang for managing multiple Terraform stacks with support for change detection and code generation.


## Terraspace by Bolt Ops

https://github.com/boltops-tools/terraspace

Terrapsace is a tool built in ruby that provides an opinionated framework for working with Terraform.

## Terraplate by Verifa

https://github.com/verifa/terraplate

Terraplate is a tool built in Golang that is a lightweight wrapper for terraform the focused on code generation.

## Astro by Uber (abandoned)

https://github.com/uber/astro

Astro is a tool built in Golang that provides a YAML DSL for defining all your terraform projects and then running them.

## Opta by Run X

https://github.com/run-x/opta

Opta is tool built in Python that makes Terraform easier by providing high-level constructs getting stuck with low-level cloud configurations.

## pterradactyl by Nike

https://github.com/Nike-Inc/pterradactyl

Pterradactyl is a library developed to abstract Terraform configuration from the Terraform environment setup.


## Leverage by Binbash

The Leverage CLI written in Python and intended to orchestrate Leverage Reference Architecture for AWS

https://github.com/binbashar/leverage



