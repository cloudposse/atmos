# Fix: Suppress JIT provisioning output during concurrent Terraform runs

**Date:** 2026-08-06

## Summary

Concurrent Terraform runs now suppress transient provisioner and step-hook UI, preventing spinner and line-clear control sequences from corrupting prefixed component output.

## Context

PR #2860 normalized child-process carriage returns and suppressed provisioner UI for Terraform output lookups. Normal scheduled Terraform execution marked its scheduler context as output-suppressed, but initial component resolution, `prepareInitExecution`, and post-init provisioners could lose that context. Backend provisioning and step hooks also retained global spinner and terminal-line UI paths while component output was streaming concurrently.

## Changes

- `pkg/scheduler/adapters/terraform.go`: mark the scheduler context as output-suppressed whenever Terraform concurrency exceeds one.
- `internal/exec/`: preserve the process context through JIT component resolution and Terraform init preparation so source and workdir provisioners receive the suppression marker.
- `pkg/provisioner/`: share the suppression marker across backend, source, and workdir provisioners; backend creation now runs without spinner/warning UI when concurrent.
- `pkg/hooks/` and `pkg/runner/step/`: suppress `clear` and `spin` terminal UI for hooks that receive scheduler node writers.
- `internal/exec/terraform_execute_helpers.go`: forward process context and writers to post-init provider-lock provisioners.
- `pkg/scheduler/adapters/terraform_test.go`: verify concurrent nodes receive suppression and sequential nodes do not.
- `internal/exec/terraform_execute_helpers_test.go`: verify JIT and pre-init provisioners receive output suppression.
- `pkg/hooks/step_engine_test.go`: verify step hooks suppress transient UI when node writers are active.

## Validation

- Focused scheduler, hooks, runner-step, provisioner, and source/workdir provisioner tests passed.
- Focused explicit-init dispatch tests passed; the full `internal/exec` suite exceeded five minutes.
- `go build ./...` passed.
- `atmos lint --changed` passed.

## Follow-ups

None.
