# Fix: Preserve custom command arguments and inherited flag shorthands

**Date:** 2026-10-22

## Summary

Custom commands preserve defaults when native arguments follow `--`, and can extend command families whose inherited flags already use `-i`.

## Context

The documented `ansible run` custom command crashed during registration because inventory and identity both claimed `-i`. After resolving registration, native flags incorrectly filled the omitted playbook argument. The same argument bug exposed trailing tokens as optional Starlark command arguments.

## Changes

Resolve positional defaults using Cobra's separator boundary. Register the identity long flag without its shorthand when an inherited flag already owns that shorthand, copying the definition so unrelated commands retain `-i`. Explain both behaviors in the command argument and identity configuration pages. Correct the skill introduction to use the documented `say.content` field.

## Validation

Both defects reproduced twice before their fixes. Focused command tests cover inherited flags from parents and ancestors, identity selection, preserved default arguments and the actual skill introduction. Fifteen live cases pass, including the introductory commands, typed Starlark inputs, a real Ansible playbook receiving native flags, standalone script invocation and interactive Ansible prompts.

## Follow-ups

None.
