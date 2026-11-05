# Fix: Reject non-scalar skill version tags

**Date:** 2026-10-14

## Summary

Reject mappings and sequences tagged `!version` during configuration processing.

## Context

The PR #3352 review identified that `ref: !version {literal: main}` bypassed the
scalar tag handler. Mapstructure then decoded the mapping into `SkillRef.Literal`,
silently replacing deferred version resolution with a literal Git ref. Empty
tagged mappings were also accepted; tagged sequences failed later with generic
decoding errors.

## Changes

Validate the node kind before dispatching YAML processing. Non-scalar `!version`
nodes return the existing unsupported-tag error, including the configuration
path. Scalar processing remains unchanged.

Add configuration-loading regressions for mappings, sequences, and empty
containers. Preserve deferred scalar references, ordinary Git refs, and quoted
literal text that resembles a tag.

## Validation

- The five malformed-tag regression cases fail before the fix and pass afterward.
- Scalar compatibility cases pass.
- Full configuration, schema, skill-source, and skill CLI package tests pass.
- `go build -p 4 ./...`: passes.
- `atmos lint --changed`: zero issues.

## Follow-ups

None.
