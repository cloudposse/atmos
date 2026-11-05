# Fix: Harden upload validation and HTTP expectation retries

**Date:** 2026-10-09

## Summary

Upload sources are checked against the opened file, HTTP expectation polling is limited to safe request methods, and ANSI trimming preserves closing sequences without introducing trailing styles. The examples design preview tolerates a missing preferred example.

## Context

Checking an upload source only by pathname left a gap before opening it. HTTP response expectation failures could repeat a successful write request. Trimming whitespace retained style-opening escapes from discarded padding, and the examples design preview assumed a particular example always existed.

## Changes

- Validate the opened upload source against the inventoried file while retaining symlink checks.
- Restrict automatic expectation polling to supported safe HTTP methods and preserve explicit retry rules.
- Discard style-opening sequences that apply only to trimmed padding while retaining needed closing sequences.
- Fall back when the preferred design-preview example is absent, including an empty example collection.
- Remove duplicate expanded-player CSS declarations while preserving the final fullscreen styles.

## Validation

- Deterministic regressions reproduced all four findings before the fixes.
- Go 1.26.9 race tests and builds pass for the affected packages.
- Changed executable Go lines in the focused fixes are fully covered; final Codecov remains required.
- The combined stack passes CloudFormation command/component race tests and changed-code lint with zero issues.
- All 57 navigation tests, 40 website prebuild tests, and the combined website build pass.
- The combined Floci lifecycle passes initial deployment, unchanged redeployment, changed release, archive verification, HTTP 200 and response assertions, and cleanup.

- PostCSS parsing and computed declaration comparisons verify the CSS cleanup preserves the expanded, fullscreen, player, and preformatted styles.

## Follow-ups

Publish the signed fixes on the S3 upload layer, cascade to the Rain migration layer, and obtain fresh CI, coverage, and CodeRabbit approvals for both resulting heads.
