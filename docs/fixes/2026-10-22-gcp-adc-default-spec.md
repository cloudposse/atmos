# Fix: Accept default GCP ADC configuration

**Date:** 2026-10-22

## Summary

A `gcp/adc` provider can omit `spec` or use `spec: {}` when no optional project, region or scope overrides are needed.

## Context

Website example validation exposed that empty mappings become nil during configuration loading. The ADC parser rejected nil even though every ADC setting is optional. This prevented a minimal provider from using existing Application Default Credentials and their defaults.

## Changes

The ADC spec parser returns an empty typed configuration for nil input. Provider kinds with required fields retain their existing validation. The provider configuration page explicitly documents the optional ADC spec.

## Validation

- Added parser and provider-validator regressions; both failed before the fix.
- Full auth types, validation, factory, GCP ADC, GCP WIF and GitHub OIDC test packages passed.
- Live validation compares omitted and empty ADC specs twice on the unchanged baseline and the rebuilt binary; baseline rejects both, rebuilt binary accepts both.
- Negative controls retain rejection of incomplete WIF, GitHub OIDC and IAM Identity Center providers.
- Authentication itself is not invoked by these static checks.

## Follow-ups

Real ADC token acquisition continues to require existing user or workload credentials and is separate from optional configuration parsing.
