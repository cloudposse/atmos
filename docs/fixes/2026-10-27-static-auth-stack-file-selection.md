# Fix: Select known stack files before static auth loading

**Date:** 2026-10-27

## Summary

Scoped auth validation selects a matching physical stack before loading imports,
so unrelated remote imports cannot prevent validation of that local stack.

## Context

`auth validate --stack dev` loaded every discovered manifest before applying its
stack filter. A remote import in `remote.yaml` caused validation to fail even
when `dev.yaml` and its imports were entirely local.

## Changes

Select exact stack-relative filenames from the already discovered paths before
static loading. Preserve the discovery list and retain full discovery when the
selector needs logical-name resolution. Continue rejecting remote imports in the
selected stack without fetching them. Evaluated stack loading is unchanged.

## Validation

The new command regression reproduces the original failure and passes after the
fix. The selected remote stack still fails, and the HTTP fixture receives no
requests. Race-enabled scoped auth, static stack, conditional import, and file
selection tests pass, including existing logical-name and imported-auth cases.

## Follow-ups

None.
