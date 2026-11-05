# Fix: Preserve step endpoints with component credentials

**Date:** 2026-10-16

## Summary

Preserve step endpoints with component credentials.

## Context

A non-nil component authentication context with no endpoint discarded workflow AWS_ENDPOINT_URL_S3 and AWS_ENDPOINT_URL overrides.

## Changes

Prefer a nonempty component endpoint, then the service-specific step endpoint, then the global step endpoint. Cover endpoint precedence and real local HTTP requests signed with component credentials.

## Validation

Regression reproduced against the original code. Focused race tests, the full Go build, and all applicable pre-commit checks passed with Go 1.26.9.

## Follow-ups

None.
