# Fix: Correct bundled skill command examples

**Date:** 2026-10-22

## Summary

Correct the command and configuration examples reported in #3479 and add regression checks tied to the published examples.

## Context

The skills advertised commands and flags that the CLI does not implement. Workflow examples also used `retry.delay`, although the retry schema has always used `initial_delay`. Earlier permissive decoding ignored `delay`; strict decoding now reports it as an unknown field. Removing it does not remove a supported retry feature.

## Changes

- Use `atmos aws ecr login`, including the supported public-registry flag.
- Configure tool aliases under `toolchain.aliases` and inspect the effective mapping with `atmos toolchain aliases`.
- Use `retry.initial_delay`, retaining a single key when an example already declares it.
- Pin vendored versions in manifests instead of passing an unsupported `vendor pull --version` flag, including the website vendoring workflow examples.
- Remove unsupported Ansible `describe component --type` and `theme browse` examples; link to the existing web theme gallery.
- Remove unsupported component/workflow tree-format claims while retaining tree examples for stacks and instances.
- Align corresponding command documentation and add curated tests that read the actual skill examples, validate command routing/flags, load the complete workflow and nested HTTP test examples, and decode toolchain aliases.

## Validation

- Reproduced the invalid commands and `retry.delay` failure with a binary built from unchanged revision `0bed64ed1c`.
- `go test ./cmd -run '^TestAgentSkill' -count=1` passed.
- Live local Ansible playbook, component description, theme list/show, toolchain installation/execution through configured aliases, local vendoring, and the corrected HTTP test example against a disposable local server passed. ECR authentication against a real account remains blocked by the deliberate local-only test scope.
- All steps from `.github/workflows/validate-agent-skills.yml` passed locally.
- The website production build passed.

## Follow-ups

Additional pre-existing field-test findings are recorded separately for prioritization. They are not fixed or filed as issues as part of this change.
