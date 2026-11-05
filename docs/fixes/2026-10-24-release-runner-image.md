# Fix: Pin GitHub Actions runner images

**Date:** 2026-10-24

## Summary

Pin floating GitHub-hosted runner labels throughout the workflows, including
the tagged release pipeline and its cache warmup.

## Context

GitHub is migrating `ubuntu-latest` to Ubuntu 26.04. Release preparation and
build jobs reported migration warnings, and the floating label permits an OS
change without a workflow change that maintainers can review.

## Changes

- Pin all five jobs in `release-pipeline.yml` to Ubuntu 24.04.
- Pin `release-cache-warmup.yml` to the same image as the release build.
- Replace remaining floating labels in 38 workflows: Ubuntu 24.04, Windows 2025,
  and macOS 26. Preserve existing explicit version and architecture pins.
- Use `runner.os` for installer smoke-test conditions so a future runner version
  update does not silently skip the Windows wait or Linux package checks.
- Add a Renovate regex manager for OS values in the four matrix-based workflows.
  Renovate's built-in GitHub Actions manager handles direct `runs-on` values but
  does not resolve matrix expressions. Preserve the existing 14-day release age.
- The separate Node.js 20 warning is addressed by this PR's existing setup-go
  v7.0.0 upgrade, which uses Node.js 24; changing the runner image does not fix
  an action's declared JavaScript runtime.

## Validation

- Passed actionlint for the release workflows and all 38 subsequently changed
  workflows with ShellCheck and Pyflakes disabled.
- Passed the existing release cache, pipeline, and composite-action contract tests.
- Passed Renovate 44.181.3 configuration validation and current JSON Schema validation.
- Checked matrix extraction against the parsed workflow matrices.
- Reviewed the workflow diff; passed the fix-record validator and `git diff --check`.

## Follow-ups

Future runner OS upgrades come through reviewed Renovate changes. Dependabot
does not currently update `runs-on` labels. Explicit OS labels still receive
GitHub's image updates within the selected OS version. Hosted CI must confirm
the pinned images; local validation does not run the hosted jobs.
