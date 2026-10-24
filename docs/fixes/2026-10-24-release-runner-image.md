# Fix: Pin the release pipeline runner image

**Date:** 2026-10-24

## Summary

Pin the tagged release pipeline and its cache warmup to `ubuntu-24.04`.

## Context

GitHub is migrating `ubuntu-latest` to Ubuntu 26.04. Release preparation and
build jobs reported migration warnings, and the floating label permits an OS
change without a workflow change that maintainers can review.

## Changes

- Pin all five jobs in `release-pipeline.yml` to Ubuntu 24.04.
- Pin `release-cache-warmup.yml` to the same image as the release build.
- The separate Node.js 20 warning is addressed by this PR's existing setup-go
  v7.0.0 upgrade, which uses Node.js 24; changing the runner image does not fix
  an action's declared JavaScript runtime.

## Validation

- Passed actionlint for both changed workflows with ShellCheck and Pyflakes disabled.
- Passed the existing release cache, pipeline, and composite-action contract tests.
- Reviewed the workflow diff; passed the fix-record validator and `git diff --check`.

## Follow-ups

Future runner OS upgrades require a reviewed workflow change. Dependabot does
not currently update `runs-on` labels; Renovate supports runner version updates.
The explicit Ubuntu label still receives GitHub's image updates within 24.04.
