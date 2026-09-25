# Fix: Deferred authentication isolation

**Date:** 2026-09-24

## Summary

Keep cached outputs, store clients, and secret resolvers isolated across effective
identities and concurrent component lookups.

## Context

Review of PR #3212 identified four valid gaps: Terraform output cache keys omitted
authentication scope; shared store binding could interleave with another reader;
disabled secret authentication retained a previous resolver; and multiple default
identities were selected by random map iteration.

## Changes

- Partition all three Terraform output APIs by supplied auth context and manager
  identity/state in `pkg/terraform/output`. Contexts are hashed rather than stored
  as plaintext cache keys; unsupported keys cannot reuse another scope's output.
- Keep store authentication binding and backend operations atomic in
  `pkg/store/deferred`. Locks follow the actual backend instance across aliases
  and copied configurations, and are released/removed after errors and panics.
- Give secret resolution a lookup-local configuration and scoped store registry.
  This covers structured/raw secrets and SOPS age-key stores without moving
  provider-specific logic into the evaluator. Masked and unused values stay lazy.
- Clear prior secret auth before preparing the next lookup. Secret defaults must
  not swallow lazy authentication failures or malformed authentication config.
- Reject multiple default identities with the existing typed error, rather than
  selecting a random principal or silently switching to ambient credentials.
- Verified the component-helper warning is not reachable: Helm and Kubernetes
  execution obtain concrete, component/stack-scoped managers through
  `SetupComponentAuthForCLI`; disabled execution supplies nil. Workflows and custom
  commands also construct ordinary managers, not list/describe deferred handles.

## Validation

- Output-cache regressions failed before the fix for all three APIs and both
  context and manager identity changes. Store concurrency regressions failed for
  Get, GetKey, and template lookups before synchronization.
- Ambiguous-default and enabled-then-disabled secret tests failed before fixes.
- Race tests passed for auth/store/secret deferred packages, secrets, and Terraform
  output. Local package coverage was 99.2%, 100%, 97.7%, 92.7%, and 94.8%, respectively;
  these are package statement percentages, not Codecov's PR-wide patch result.
- Concurrency tests cover aliases, copied configs, same-name identities with
  different credentials, mixed secret/store consumers, and raw secret reads.
- A real local SOPS age-key store round trip passed and preserved caller config.
- Focused Helm/Kubernetes and execution-auth setup tests passed for the reported
  component-helper path; no production change was needed for that finding.
- `go build ./...`, the workspace binary build, and `atmos test` scoped to the
  seven affected packages passed. Full `cmd/list`/`pkg/list` short tests and focused
  `cmd`/`internal/exec` regressions passed; patch lint reported zero issues.
- The workspace binary listed the demo stacks in strict default and provenance
  output, and rejected an invalid explicit identity. The demo's dev emulator was
  already running during this check; stopped-emulator behavior was verified by
  deterministic regression tests, not claimed for this live run.

## Follow-ups

None.
