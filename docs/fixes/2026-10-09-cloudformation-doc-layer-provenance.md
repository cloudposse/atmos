# Fix: Clarify CloudFormation documentation layer provenance and prerequisites

**Date:** 2026-10-09

## Summary

CloudFormation docs now distinguish the #3136 documentation snapshot from its #3137 companion
implementation and declare Python 3 for the example's lifecycle hooks.

## Context

Five historical fix reports described implementation and validation absent from the documentation
layer itself. The termination-protection guidance described the companion's safer behavior, while
the older snapshot still reconciled a false setting by disabling protection. The example also
claimed a container runtime was its only prerequisite despite invoking `python3` hooks.

## Changes

- Mark the five historical reports' changes as pending at #3136 and scope their validation to
  the companion implementation.
- Match both reference pages to #3136: a successful stack apply with protection set to false
  disables it, with an explicit caution about removing the deletion safeguard.
- Supply a separate #3137 documentation patch restoring the enable-only guidance alongside that
  implementation. Keep stack-history explanations in fix records, not user-facing reference pages.
- Require Docker or Podman plus Python 3 for the example lifecycle.

## Validation

- Inspected the owning head `179920b9fd6bddcc0dd462c36a382b7c1ab44c75`, companion implementation,
  and final stack head `48b88699b0b89e6c76398ade858c2f4bb73dd50f`.
- Focused termination-protection and apply tests passed on the owning head.
- The same focused test command passed on the final stack head, whose false-setting regression
  asserts that protection is left unchanged.
- The three Python hook-marker tests passed.
- Fix-log format validation and `git diff --check` passed.
- The combined website build is handled on the owning stack branch after the documentation
  patches are combined.

## Follow-ups

None.
