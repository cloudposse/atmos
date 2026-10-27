# Fix: Preserve inherited unsets and output ownership in VHS tapes

**Date:** 2026-10-26

## Summary

Tape recordings now honor inherited environment removal, protect hand-written render targets when
mixed with tape outputs, and tolerate repeated validation through the automation step library.

## Context

Review of PR #2889 found three gaps. A lifted `unset` only deleted a tracked override, so variables
inherited from the process or workflow remained present. One tape `Output` directive enabled
replacement for every render target, including hand-written paths. Automation validated the same
cast step twice, mistaking generated tape children for conflicting hand-written steps on the second
pass.

## Changes

- Session tapes replay `unset` in the live shell and keep subsequent hidden setup commands in order.
  Earlier lifted exports remain in the initial shell environment. This does not depend on which
  environment variables happen to exist during validation.
- Steps tapes track removed names and apply a safely quoted `unset` before each subsequent shell
  command. Setup expansion sees the names as absent; a later export restores them. Simulated text
  stays unchanged, generated commands remain literal, and the author's command supplies the exit
  status.
- Tape outputs carry overwrite permission per render format, with `.jpg` and `.jpeg` normalized to
  the same format. Rendering still preflights every target before changing any artifact.
- A nonserialized marker records successful tape expansion. Repeated validation reuses generated
  children; failed expansion and explicit source/steps conflicts still fail normally. The marker and
  output ownership metadata never enter JSON or YAML declarations.
- Updated tape guidance and the hero interpretation regression to describe live unset behavior.

## Validation

- Focused tape, setup, rendering, and new review regressions passed in `pkg/runner/step` and
  `pkg/asciicast`, including a race-detector run.
- New environment tests confirm both process and workflow values are visible to a shell before
  unsetting, then absent afterward in both tape modes. They use the Go shell interpreter and avoid
  platform-specific test binaries.
- Mixed HTML/ASCII output tests verify that a protected existing artifact prevents replacement of
  either target; tape-owned targets can still be regenerated. Repeated-validation tests exercise
  inline tape, `tape_file`, and actual automation-library generation.
- `go build ./pkg/runner/step ./pkg/asciicast ./cmd/cast` passed.
- JPEG-alias rendering and JSON/YAML nonserialization tests passed under the race detector.
- `go test ./cmd/cast -run TestRender -count=1` and `go build ./cmd/cast` passed.
- The fix-document validator and `git diff --check` passed.
- `./custom-gcl run --config=.golangci.yml --allow-serial-runners --new-from-rev=origin/main`
  passed with zero issues. The affected unset tests passed under the race detector again after
  the test-only lint fixes.

## Follow-ups

None.
