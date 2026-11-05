# Fix: `atmos stack get|set|delete|format` deprecation notice written to stdout

**Date:** 2026-11-05

## Summary

The legacy `atmos stack get`, `set`, `delete`, and `format` commands, deprecated in favor of
`atmos stack config <command>`, printed `Command "get" is deprecated, use ...` on stdout before their output.
That broke `value=$(atmos stack get ...)` and `--format json` parsing. The notice is now a UI warning on stderr.

## Context

The v1.233.2 → v1.234.0-rc.1 regression test showed v1.233.2 printing only `alpha` for
`atmos stack get vars.a -c app -s dev`, while the release candidate printed the deprecation line first. #3442
marked the commands deprecated with Cobra's `Deprecated` field. Cobra prints that notice with `c.Printf` to the
command's output stream before any hooks run, and `cmd/root.go` sets the root output stream to stdout for help
templates.

## Changes

- `cmd/stack/stack.go`: `deprecateLegacyCommand` hides each legacy command, records its replacement in an
  annotation, and wraps `RunE` to emit the notice with `ui.Warningf`. Cobra's `Deprecated` field is no longer set
  for these commands.
- `cmd/stack/create_test.go`: asserts the replacement annotation instead of the Cobra field.

## Validation

- New `cmd/stack/deprecation_test.go`: with separate stdout and stderr streams, a deprecated command writes only
  its data to stdout, the notice to stderr, and nothing to Cobra's output stream; the four legacy commands are
  hidden, carry their replacement, and do not set Cobra's `Deprecated` field.
- `go test ./cmd/stack/` passed. `gofumpt -l` reported no changes. `golangci-lint` was not run (the command was
  not permitted in that session).

## Follow-ups

Other commands that use Cobra's `Deprecated` field have the same stdout leak, which predates this release:
`atmos terraform varfile` and `atmos terraform write varfile`, which also print their own stderr warning. A
central fix should route Cobra command deprecation notices to stderr without changing help visibility.
