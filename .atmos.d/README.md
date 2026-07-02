# Atmos Custom Commands

This directory contains custom Atmos commands that are automatically loaded when you run `atmos` from the repository root.

## Command Groups

- `dev.yaml` - local setup, `atmos dev shell`, and quick validation workflow commands
- `build.yaml` - `atmos build deps`, `atmos build`, `atmos build version`, and README generation
- `check.yaml` - `atmos check staged`, `atmos check pr`, `atmos check all`, and Codecov validation
- `format.yaml` - `atmos format staged`, `atmos format pr`, and `atmos format all`
- `cache.yaml` - `atmos cache list` and `atmos cache clear`
- `lint.yaml` - `atmos lint changed`, lintroller, gomodcheck, custom golangci-lint, and link checks
- `test.yaml` - `atmos test`, mode flags, legacy test subcommands, race, and mock generation commands
- `screengrabs.yaml` - `atmos screengrabs ...` commands for generating website screengrabs
- `toolchain.yaml` - toolchain aliases and registries

## Why Custom Commands?

We use Atmos custom commands for our development workflow as a form of "dogfooding" - using our own tool to manage our development process. This ensures we experience the same workflows our users do and helps us identify areas for improvement.

For more information on custom commands, see the [Atmos Custom Commands documentation](https://atmos.tools/cli/configuration/commands).
