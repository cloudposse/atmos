# Fix: Homebrew formula bump needs the homebrew/core tap checked out

**Date:** 2026-10-09

## Summary

The release workflow's "Bump Homebrew formula" step still failed on every release with
`No available formula with the name "atmos".`, including on `v1.231.0` after the
`HOMEBREW_NO_INSTALL_FROM_API` change from PR #3240. The real root cause is that
`brew bump-formula-pr` is a developer command that edits the formula's `.rb` file, so it
needs `homebrew/core` checked out as a local tap. A fresh GitHub runner uses the formulae
API with no tap, so the formula file does not exist locally and the command cannot
resolve `atmos`. The fix taps `homebrew/core` explicitly before running the bump.

## Context

This step has failed on every stable release and every Homebrew bump has been done
manually (`v1.228.0`, `v1.229.0`, `v1.230.0`, `v1.230.1`, `v1.231.0`):

- PR #3229 fixed a `brew: command not found` PATH bug (sourcing `brew shellenv`), which
  let the step run `bump-formula-pr` for the first time.
- PR #3240 then removed `HOMEBREW_NO_INSTALL_FROM_API: "1"` on the premise that API mode
  would resolve the formula "with no tap clone needed". That premise was wrong:
  `bump-formula-pr` edits the local formula file, so it needs the tap regardless of
  which resolution mode is active. With or without the env var, a fresh runner has no
  tap, so the bump fails identically.

The `v1.230.1` dry-run that appeared to prove API mode worked was run on a developer
machine that already had `homebrew/core` tapped - which is exactly the condition CI
lacks. On the `v1.231.0` run the automated step failed again, and the bump was done
manually (homebrew-core PR #316660).

## Root cause, verified

With `homebrew/core` tapped locally, the exact CI command resolves `atmos` and runs all
the way to the duplicate-PR check:

```
$ HOMEBREW_NO_AUTO_UPDATE=1 brew bump-formula-pr --dry-run --no-browse --no-audit \
    --version=1.231.0 atmos
Warning: These open pull requests might be duplicates:
atmos 1.231.0 https://github.com/Homebrew/homebrew-core/pull/316660
```

No "No available formula" error. The only difference between a working local run and the
failing CI run is the presence of the tap.

## Changes

- `.github/workflows/build.yml`: added `brew tap homebrew/core` immediately before
  `brew bump-formula-pr` in the "Bump Homebrew formula" step. Corrected the step comment,
  which previously (and incorrectly) claimed no tap clone was needed. `HOMEBREW_NO_AUTO_UPDATE`
  is kept and `HOMEBREW_NO_INSTALL_FROM_API` is left unset (unnecessary once the tap is
  present).

## Validation

- Reproduced the resolution empirically: `brew bump-formula-pr --dry-run ... atmos` fails
  with "No available formula" when `homebrew/core` is not tapped and succeeds (reaching
  the duplicate-PR check) when it is.
- `brew tap homebrew/core` exits 0 on a current Homebrew (7.0.8) in default API mode - it
  is still permitted, just no longer implicit.
- The first real confirmation will be the next release run; this step runs only on
  `release: published`, so it cannot be exercised by a normal PR check.
