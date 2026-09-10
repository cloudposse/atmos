# Fix: Preserve Required Dependency Sources in Scoped Reverse Lookups

**Date:** 2026-09-10

## Summary

`atmos describe dependents` now reports an unavailable required typed target instead of silently returning no dependents.

## Context

Reverse scoped evaluation omitted modern required dependency sources when their unavailable targets did not create structural graph edges.

## Changes

Direct dependent lookup now asks scoped reverse evaluation to include sources with required modern dependencies on the selected component.

## Validation

- Focused scoped reverse-resolution regression.
- Focused resolved-stack dependent lookup regression.
- Fresh CLI reproduction for a disabled `packer:image` required by `app`.

## Follow-ups

None.
