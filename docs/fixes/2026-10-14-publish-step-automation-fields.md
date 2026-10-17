# Fix: Register publish fields with direct automation calls

**Date:** 2026-10-14

## Summary

Declare the publish step's accepted fields and include it in the automation step catalog.

## Context

The upstream automation validation now requires each built-in handler to declare its fields.
Restacking the publish step onto that change exposed three failing registry and documentation
checks. Without a declaration, direct publish calls could silently accept fields from other steps.

## Changes

Declare source, target, destination, and stack using the existing field-validation mechanism.
Document `steps.publish` alongside the other direct step functions. Extend the direct-call tests
to accept the publish configuration and reject an unrelated command field.

## Validation

The combined step suite reproduced the missing declaration and catalog entry. After the fix,
patch lint and the full step race suite pass. Registry checks, direct-call acceptance and rejection,
and the automation documentation catalog are covered. Final website verification is recorded with
the stack's validation evidence.

## Follow-ups

Complete final published CI and CodeRabbit verification in the active stack.
