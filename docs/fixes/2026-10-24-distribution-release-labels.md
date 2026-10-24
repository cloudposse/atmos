# Fix: Classify distribution changes separately from application breaks

**Date:** 2026-10-24

## Summary

Correct PR #3509 from `major` to `no-release` and align CodeRabbit and PR skill
guidance with the maintainer's distribution policy.

## Context

Retiring low-usage prebuilt downloads was incorrectly treated as an incompatible
Atmos application change. The `major` label consequently required a major release
milestone even though the PR does not change the CLI or configuration contract.

## Changes

- Require an identified incompatible public application contract to justify
  `major`; CI, runner pins, release tooling, and distribution-only changes do not
  independently justify a major release.
- Update CodeRabbit's release-label instructions and add review guidance to flag
  unsupported major labels or milestone requirements.
- Update the pull-request skill's decision tree and examples, including Go code
  in build/release tooling and retiring prebuilt targets with source installation.
- Keep the requested platform announcement and installation alternatives, use
  the deprecation blog tag, and remove the major-milestone claim from the PR body.

## Validation

- Validated `.coderabbit.yaml` against CodeRabbit's current configuration schema.
- Confirmed PR #3509 has `no-release` and no `major` label or milestone.
- Reviewed the skill and review instructions together for consistent classification.

## Follow-ups

CodeRabbit instructions guide reviews; they are not a deterministic enforcement
mechanism. Maintainers still review compatibility classification. Existing release
milestone checks remain in place for actual application changes.
