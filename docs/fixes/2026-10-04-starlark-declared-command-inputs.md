# Fix: Share native command inputs with Atmos Automation Language

**Date:** 2026-10-04

## Summary

Standalone scripts declare positional arguments, typed flags, and validation with
`cli.command`, `cli.arg`, and `cli.flag`. Embedded script steps read parsed command
inputs directly from `ctx.flags` and `ctx.arguments`.

## Context

Standalone programs previously parsed raw arguments themselves. Custom commands
could only pass flags into scripts through templates or explicit environment
mappings, despite Atmos already having parsed and validated those values.

## Changes

- Reuse native flag definitions, `StandardParser`, its flag registry, and
  `PositionalArgsBuilder` through an isolated host command. Script flags do not
  inherit or modify the global Atmos command namespace.
- Generate help and validate parsed values before invoking `main(args, flags)`.
  An optional validation callback can reject inputs before the main callback.
- Support string, integer, boolean, and string-list flags with defaults, choices,
  shorthand, and optional environment bindings. Required flags need explicit
  command-line or bound environment input. Native conversion rejects malformed
  typed environment values, while explicit CLI values take precedence.
- Separate the Starlark declaration binding into `stdlib/cli`; the runtime owns
  invocation and task restrictions, and the CLI host owns parsing and help.
- Copy parsed inputs into immutable dictionaries. Preserve raw values and types
  through sequential, parallel, matrix, and test children without template
  interpretation or mutation of the parent's context.
- Insert dictionary keys in sorted order so input iteration and printed output
  remain deterministic rather than inheriting Go map iteration order.
- Update the custom-command example and recording to read `ctx.flags` directly.
  Add an executable declared-interface example and language reference.

## Validation

- Native host behavioral tests cover help, typed values, defaults, environment
  precedence, required inputs, choices, separators, and malformed declarations.
- Language binding tests cover declaration metadata, callback failures, help and
  dry-run side-effect suppression, invocation restrictions, and immutable inputs.
  The module reached 100% statement coverage in targeted race-enabled tests.
- A rebuilt CLI exercised parsed inputs through sequential, parallel, matrix,
  and test children, preserving a boolean and literal template syntax.
- The updated custom-command recording passed independent Starlark validation
  and visual review. All 14 focused example CLI cases and schema validation
  passed in 42.278 seconds.
- Full tests for `cmd`, `internal/exec`, `pkg/runner/step`, `pkg/script/...`, and
  `pkg/workflow` passed with coverage instrumentation. Changed-line coverage
  against `origin/osterman/starlark-integration-core` initially reached 95.12%.
  After the dictionary-order regression tests, combined coverage reached
  369/370 changed lines (99.73%).
- Scoped repository lint reported zero issues, and the production website build
  passed with the declaration reference and updated recording.
- Broader Starlark, secret-masking, and instance-list CLI regressions passed
  against the finished implementation in 64.244 seconds.
- `go test -race -count=20 -timeout=30m ./pkg/runner/step` passed in 786.468
  seconds with no race reports or timing failures. This did not reproduce the
  earlier intermittent failure; it does not establish that a race is impossible.
- The dictionary-order regression failed twice before sorting keys. After the
  fix, all Starlark package tests passed three race-enabled repetitions, the
  converter reached 100% statement coverage, and scoped lint reported zero issues.
  A rebuilt binary ran `.context/dictionary-order.star east api --zulu=z
  --alpha=a --middle=m` five times; argument keys were `["app", "zone"]` and
  flag keys were `["alpha", "middle", "zulu"]` every time.

## Follow-ups

None.
