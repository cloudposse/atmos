---
title: Atmos Alternatives
sidebar_label: Alternatives
sidebar_position: 3
---

To better understand where Atmos fits in, it may be helpful to understand some of the alternative tooling it seeks to replace. There are lots of great
tools out there and we're going through a bit of a "DevOps Rennisance" when it comes to creativity on how to automate systems.

## General Alternatives

There are many tools in the general category of "task runners" or "workflow automation". Here are some of the alternatives to Atmos, many of which
inspired core functionality in Atmos.

### Make (Makefile) by Free Software Foundation

https://www.gnu.org/software/make/

Many companies (including Cloud Posse) started by leveraging `make` with `Makefile` and targets to call `terraform`. Using `make` is popular method of
orchestrating tools, but it has trouble scaling up to support large projects. We know this because [Cloud Posse](https://cloudposse.com/) used it for
3+ years. The problems we ran into is that `make` targets do not support "natural" parameterization, which leads to a proliferation of environment
variables that are difficult to validate or hacks like overloading make-targets and parsing them (e.g. `make apply/prod`). Makefiles are unintuitive
for newcomers because they are first evaluated as a template, and then executed as a script where each line of a target runs in a separate process
space. Spaces matter too, and it's made worse with inconsistent rules using tabs in some places and spaces in others.

### Mage (Magefile)

https://magefile.org/

Mage is a make/rake-like build tool using native Golang and plain-old `Go` functions. Mage then automatically provides a CLI to call them as
Makefile-like runnable targets.

### Task (Taskfile)

https://github.com/go-task/task

Task is a task runner and build tool that aims to be simpler and easier to use than GNU Make.

:::info
Atmos supports native [workflows](/core-concepts/workflows) that have very similar schema to "Taskfile", only they can be defined together
with [Stacks](/core-concepts/stacks) or as standalone workflow files.
:::

### Variant

https://github.com/mumoshu/variant
https://github.com/mumoshu/variant2 (second generation)

Variant lets you wrap all your scripts and CLIs into a modern CLI and a single-executable that can run anywhere.

:::info
The earliest versions of `atmos` were built on top of [`variant2`](https://github.com/mumoshu/variant2) until we decided to rewrite it from the ground
up in pure Go. Atmos supports native [workflows](/core-concepts/workflows) which provide similar benefits.
:::

### AppBuilder by Choria

https://github.com/choria-io/appbuilder

AppBuilder is a tool built in Golang to create a friendly CLI command that wraps your operational tools.

:::info
Atmos is heavily inspired by the excellent schema provided by AppBuilder and has implemented a similar interface as part of
our [Custom Commands](/core-concepts/custom-commands).
:::

## Terraform Alternatives

There are many tools explicitly designed around how to deploy with Terraform.

The following is a list of tools that only support Terraform.

:::tip Atmos Differentiators
Atmos not only supports Terraform, but can be used to manage any CLI. For example, by combining [Custom commands](/core-concepts/custom-commands)
and [workflows](/core-concepts/workflows), it's possible to support any CLI tool (even the ones listed below) or even reimplement core functionality of
atmos. That's how extensible it is.
:::

### Terragrunt by Gruntwork

https://github.com/gruntwork-io/terragrunt

Terragrunt is a tool built in Golang that is a thin wrapper for Terraform that provides extra tools for working with multiple Terraform modules.

### Terramate by Mineros

https://github.com/mineiros-io/terramate

Terramate is a tool built in Golang for managing multiple Terraform stacks with support for change detection and code generation.

### Terraspace (Terrafile) by Bolt Ops

https://github.com/boltops-tools/terraspace

Terrapsace is a tool built in Ruby that provides an opinionated framework for working with Terraform.

### Terraplate by Verifa

https://github.com/verifa/terraplate

Terraplate is a tool built in Golang that is a lightweight wrapper for terraform the focused on code generation.

### Astro by Uber (abandoned)

https://github.com/uber/astro

Astro is a tool built in Golang that provides a YAML DSL for defining all your terraform projects and then running them.

### Opta by Run X

https://github.com/run-x/opta

Opta is tool built in Python that makes Terraform easier by providing high-level constructs and not getting stuck with low-level cloud configurations.

### pterradactyl by Nike

https://github.com/Nike-Inc/pterradactyl

Pterradactyl is a library developed to abstract Terraform configuration from the Terraform environment setup.

### Leverage by Binbash

The Leverage CLI written in Python and intended to orchestrate Leverage Reference Architecture for AWS

https://github.com/binbashar/leverage

