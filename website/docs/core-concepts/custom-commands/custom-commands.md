---
title: Atmos Custom Commands
sidebar_label: Custom Commands
id: custom-commands
---

Atmos can be easily extended to support any number of custom CLI commands.

Custom commands are exposed through the `atmos` CLI when you run `atmos help`. It's a great way to centralize
the way operational tools are run in order to improve DX.

For example, one great way to use custom commands is to tie all the miscellaneous scripts into one consistent CLI interface. Then we can kiss those
ugly, inconsistent arguments to bash scripts goodbye! Just wire up the commands in atmos to call the script. Then developers can just run `atmos help`
and discover all available commands.

## Simple Example

Here is an example to play around with to get started.

Adding the following to `atmos.yaml` will introduce a new `hello` command.

```yaml
# Custom CLI commands
commands:
  - name: hello
    description: This command says Hello world
    steps:
      - "echo Hello world!"
```

We can run this example like this:

```shell
atmos hello
```

## Positional Arguments

Atmos also can support positional arguments. Arguments do not support default values and are required if defined.

If we add the following to `atmos.yaml`, will introduce a new `hello` command that accepts one `name` argument.

```yaml
# subcommands
commands:
  - name: hello
    description: This command says hello to the provided name
    arguments:
      - name: name
        description: Name to greet
    steps:
      - "echo Hello {{ .Arguments.name }}!"
```

We can run this example like this:

```shell
atmos hello world
```

## Passing Flags

Passing flags works much like passing positional arguments, except for that they are passed using long or short flags.
Flags can be optional.

```yaml
# subcommands
commands:
  - name: hello
    description: This command says hello to the provided name
    flags:
      - name: name
        shorthand: n
        description: Name to greet
        required: true
    steps:
      - "echo Hello {{ .Flags.name }}!"
```

We can run this example like this, using the long flag:

```shell
atmos hello --name world
```

Or, using the shorthand, we can just write:

```shell
atmos hello -n world
```
