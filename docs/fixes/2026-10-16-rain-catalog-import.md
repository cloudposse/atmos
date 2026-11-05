# Fix: Import the Rain migration catalog component

**Date:** 2026-10-16

## Summary

Make the deployment example resolve the abstract component it inherits.

## Context

Stack discovery includes deploy files, so catalog defaults must be imported explicitly.

## Changes

Added catalog/vpc-defaults to the deploy stack import list.

## Validation

Extracted the documented configuration and stack snippets into an isolated fixture.
`atmos describe stacks` resolved `vpc-defaults` and inherited `CAPABILITY_IAM`.
Website prebuild tests, navigation tests, and applicable pre-commit hooks passed.
The production website build passed. Its rendered inheritance example includes
the explicit catalog import.

## Follow-ups

None.
