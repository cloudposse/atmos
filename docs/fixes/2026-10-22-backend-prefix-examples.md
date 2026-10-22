# Fix: Correct dynamic S3 state prefix examples

**Date:** 2026-10-22

## Summary

Place `workspace_key_prefix` under `backend.s3` and state the Go-template setting required to render it.

## Context

The field test generated backend JSON from the published example. A prefix directly under `backend` was ignored and the component-derived default was used. The nested S3 control rendered the intended value.

## Changes

- Correct the Terraform skill backend reference and both workspace command pages.
- Distinguish an S3 state key prefix from the Terraform workspace name.
- State `templates.settings.enabled: true` explicitly.

## Validation

Ran real `atmos terraform generate backend demo -s dev` with a disposable stack and inspected `backend.tf.json`. With templates enabled, `backend.s3.workspace_key_prefix: "{{.vars.stage}}-demo"` produced `dev-demo`; the original top-level placement produced the default `demo`.

## Follow-ups

Backend list and describe failures remain separate open runtime findings in the field-test report.
