# Fix: Harden-runner DNS repair no longer races the StepSecurity DNS listener

**Date:** 2026-10-26

## Summary

The `harden-runner` composite action's DNS repair checked once whether the StepSecurity agent was
listening on `127.0.0.1:53`. The agent reports `active` slightly before its DNS proxy binds that port,
so on a slow-starting runner the single check failed and the job died in the "Harden Runner" step
before checkout. The repair now polls for the listener for up to 15 seconds.

## Context

`Acceptance Tests (linux, shard 5/10)` failed in the Harden Runner step with
`DNS repair failed: StepSecurity is not listening on 127.0.0.1:53`. No tests ran. The agent log shows
the listener check (`ss -lnup ( sport = :53 )`) ran about 2.4 seconds after the agent started, while
the agent was still resolving its own API endpoints. The one-shot check came from the runner-hardening
unification in #3493 and affects any job that uses the action, not only this branch.

## Changes

- `.github/actions/harden-runner/runtime/index.cjs`: the listener check retries every 500 ms for up to
  30 attempts (15 seconds) before failing, with an injectable synchronous `sleep`. Every other safety
  check is unchanged: the agent must be active and in block mode, and DNS is never replaced unless the
  listener is confirmed. The failure message now states how long it waited.
- `.github/actions/harden-runner/runtime/index.test.cjs`: tests for a listener that binds on the third
  check, for giving up after 30 checks without touching DNS, and for not waiting when the listener is
  already up.
- `.github/actions/harden-runner/README.md`: documents the wait.

## Validation

- `node --test runtime/*.test.cjs diagnostics/*.test.cjs` in `.github/actions/harden-runner`: 90 passed,
  0 failed.
- Confirmed the real synchronous sleep (`Atomics.wait`) blocks for the requested time on Node's main
  thread.
- The fix itself only proves out on a GitHub-hosted runner; the failed job was also re-run.

## Follow-ups

None.
