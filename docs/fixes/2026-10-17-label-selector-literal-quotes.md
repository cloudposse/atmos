# Fix: Preserve literal quotes in repeated label selectors

**Date:** 2026-10-17

## Summary

Existing selectors such as `--labels 'owner=O"Reilly'` work again while retaining repeated flags,
comma-separated pairs, and CLI-over-environment-over-configuration precedence. This is a regression
fix for every edition; no edition transition is required.

## Context

The release comparison against v1.232.0 found that replacing the single string `--labels` flag with
pflag's `StringSlice` added strict CSV parsing before the label parser ran. A literal double quote
in a previously accepted label value then caused a `bare " in non-quoted-field` error. The failure
was reproduced in `list instances`, `list components`, and `list stacks`, including old edition pins.
The intended change was to accumulate repeated selectors, not to introduce CSV quoting rules.

## Changes

Use the existing `flags.StringArrayFlag`/`WithStringArrayFlag` support for repeatable label selectors.
Each occurrence reaches the existing label parser unchanged, and that parser continues to split
comma-separated `key=value` or `key:value` pairs. Updated registrations and direct Cobra reads in
list, Terraform, workflows, vendor, Helm, Kubernetes, containers, and CloudFormation. Viper keys and
environment bindings are unchanged. The existing namespace guard now also recognizes string-array
registrations.

Added tests through real command parsers for literal quotes, repeated/comma-separated selectors,
last-value precedence, scalar/list configuration, environment overrides, explicit empty flags, and
workflow forwarding. Existing direct flag test fixtures now use the same flag type as production.

## Validation

- Added `TestListLabelsPreserveLiteralQuotes` first: all three command cases failed with the original
  CSV parse error before the production change, then passed after it.
- Targeted label, selector, command-config, and string-array tests passed in `cmd/list`,
  `cmd/terraform`, `cmd/workflow`, `cmd/kubernetes`, `cmd/helm`, `cmd/container`,
  `cmd/aws/cloudformation`, `cmd/vendor`, `pkg/tags`, and `pkg/flags`.
- Focused root command selector namespace guards and internal workflow/vendor label callers passed.
- Changed Go files formatted with `gofumpt`; `git diff --check` passed.

## Follow-ups

None.
