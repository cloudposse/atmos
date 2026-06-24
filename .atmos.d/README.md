# Atmos Custom Commands

This directory contains custom Atmos commands that are automatically loaded when you run `atmos` from the repository root.

## Development Commands

The `dev.yaml` file defines development workflow commands that help maintain code quality:

- `atmos dev setup` - Set up local development environment
- `atmos dev check` - Run pre-commit hooks on staged files
- `atmos dev check-all` - Run pre-commit hooks on all files
- `atmos dev lint` - Run golangci-lint

## Why Custom Commands?

We use Atmos custom commands for our development workflow as a form of "dogfooding" - using our own tool to manage our development process. This ensures we experience the same workflows our users do and helps us identify areas for improvement.

For more information on custom commands, see the [Atmos Custom Commands documentation](https://atmos.tools/cli/configuration/commands).
