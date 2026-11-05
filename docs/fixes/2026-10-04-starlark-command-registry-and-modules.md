# Fix: Integrate Starlark command bindings with the CLI registry

**Date:** 2026-10-04

## Summary

Starlark's named Atmos helpers now come from the actual host command tree. Native
flag spellings come from registered flag and compatibility metadata. Independent
Atmos, logging, and regular-expression bindings live in `stdlib` subpackages.

## Context

The runtime maintained a hard-coded list of CLI commands and special-cased
Terraform's detailed-exitcode spelling. The list could drift from the registered
CLI and could not expose custom commands or aliases. Session state, language
bindings, and host command definitions were mixed together.

## Changes

- Added an immutable `pkg/flags.CommandCatalog` snapshot, populated through the
  existing command registry after built-ins, custom commands, and aliases register.
- Included late-registering commands and Cobra's help command by configuring the
  engine during invocation setup rather than the middle of package initialization.
- Resolved shorthand/native compatibility names through nested command metadata,
  including subcommands supplied in `args=` and registered flags before subcommands.
- Kept actual parsing, environment binding, and execution in the invoked CLI.
- Extracted `stdlib/atmos`, `stdlib/log`, and `stdlib/regex`. The engine retains
  process execution, cancellation, session state, and parallel-task lifecycle.
- Shared sequence conversion and argument diagnostics through an internal package,
  retaining error identities and readable diagnostics without repeated prefixes.
- Added `WithAtmosCommands` for embedding hosts. Existing explicit invocation
  helpers remain available without a catalog; named helpers reflect host commands.

## Validation

- Race-enabled Starlark integration tests passed after the module extraction.
- Direct binding tests passed, including argv, policy, structured logging, masking,
  Unicode regex handling, and invalid-argument behavior.
- Real CLI registry test passed for provider commands, late registrations, and
  Terraform compatibility flag spellings.
- Catalog tests passed for aliases, immutable snapshots, inherited flags,
  interspersed flag values, unknown arguments, and explicit separators.

## Follow-ups

None.
