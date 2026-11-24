# Fix: Native Terragrunt support fixes before 1.235.0

**Date:** 2026-11-23

## Summary

The 1.235.0 regression run found five problems in the native Terragrunt configuration provider (F1 to F5), and
`atmos plugin test plugins/terragrunt --integration` failed 20 of 44 scenarios on Linux. The same 20 failed on
macOS with OpenTofu, Terraform and the scenarios' own Terragrunt stand-in available, so the failures were not
caused by missing tools. This change fixes F1 to F5, fixes the real bugs the integration triage found, and
updates the scenarios whose expectations contradicted documented behavior. After the change all 44
integration scenarios pass on macOS.

## Context

- **F1 (every input masked).** Terragrunt lists the files of a local module source (and `mark_as_read`
  targets) among the files it read without opening them. The plugin's sensitivity scan treated an existing
  file whose evaluated bytes were not captured as unverifiable and returned a root path, so the host masked
  every input, wrote an empty tfvars file and sent all inputs as `TF_VAR_*`. Captured module `.tf.json` files
  were also treated as JSON Terragrunt configuration. On the host, a sensitive value registered its object
  keys and every scalar, and a sensitive generated file (every generated file is marked sensitive, including
  `backend.tf`) registered every line, so `}`, `app`, `3` and similar text was masked everywhere ("known after
  `<MASKED>`ly"). A read_terragrunt_config parent-folder search inside an included file also started from the
  included file instead of the unit, so plain values read from `env.hcl` were treated as sensitive.
- **F2 (dependency outputs and dynamic generate).** A state read of a unit only accepted literal generate
  blocks, so a parent configuration whose generate block interpolates a local made every dependent fail with
  "Cannot verify the backend identity of a generate block".
- **F3 (`terraform generate backend`).** The command wrote a placeholder `local {}` `backend.tf.json` next to
  the unit's own `backend.tf`, and the next plan failed with "Duplicate backend configuration".
- **F4 (one rejected override broke the project).** Stack compilation returned the backend-override error,
  so every command in the project failed, although the docs say only commands that target the unit fail.
- **F5 (no reason for evaluation errors).** The generic "Terragrunt could not evaluate the configuration"
  problem carried no detail at all, so even Debug and Trace output did not show why. The F1 false positives
  also withheld every Terragrunt log event.
- **Integration triage.** CI runs `plugin test` without `--integration`, so these scenarios had never run in
  CI. Besides F1, F2 and F4, the triage found real bugs in saved plans, CI deploy verification, `remote_state`
  without `generate`, and option error messages, and found scenarios whose expectations were stale.

## Changes

- **F1, plugin.** `plugins/terragrunt/internal/engine/sensitivity_logs.go`: the evaluated file view records
  which files the evaluation opened; an existing file that was never opened reads as `errNotRead` (missing),
  except the unit itself, which stays fail-closed. `sensitivity_files.go` (new): Terraform and OpenTofu source
  files (`.tf`, `.tf.json`, `.tofu`, `.tofu.json`) are never scanned as Terragrunt configuration, and
  read_terragrunt_config parent searches start from the unit. `sensitivity.go` uses both.
- **F1, host.** `pkg/io/sensitive_value.go` (new): `RegisterSensitiveValue` and `RegisterSensitiveText` skip
  object keys, booleans, null, values under four characters and punctuation-only text, and skip structural
  lines (`terraform {`) of a sensitive text. `pkg/plugin/adapter/component/objects.go` and
  `pkg/component/configuration/files.go` use them.
- **F2.** `plugins/terragrunt/internal/engine/backend_generation_evaluated.go` (new): when the static check
  cannot prove a generate declaration free of a backend, a state read evaluates the unit once more in the
  read-only discovery environment (inputs projected away, dependency outputs as unknown placeholders) and
  accepts it only when every rendered Terraform file parses and declares no backend. Automatic includes,
  commands, secrets, dependency outputs and unparsable files still fail closed. `resolve.go` calls it and
  `backend_generation.go` marks the static rejection with a sentinel.
- **F3.** `pkg/component/terraform/generate_backend.go` skips a provider-owned component with a warning, and
  `generate_backends.go` skips provider-owned components in bulk generation.
- **F4.** `pkg/component/configuration/conflicts.go` records a rejected component-level backend override in
  the reserved `settings.configuration.atmos_rejected_override` key (`pkg/schema/component_configuration.go`)
  instead of returning an error from stack compilation (`internal/exec/configuration_stacks.go`,
  `stack_processor_merge.go`). `pkg/component/terraform/configuration_overrides.go` rejects commands that
  target the unit, and `pkg/scheduler/adapters/terraform_preflight.go` rejects bulk runs that select it before
  anything runs.
- **F5.** `plugins/terragrunt/internal/engine/problems.go` sends the evaluator's text as unsafe detail;
  `internal/provider/logs.go` redacts it with the call's log sink and drops it when the sink withheld the
  call's events (`engine/logs.go` `RedactDetail`). `pkg/plugin/adapter/component/diagnostics.go` adds the
  masked detail to the error at Debug and Trace (and still logs it), and keeps the debug hint otherwise.
- **Saved plans (integration).** `plugins/terragrunt/bridge/overlay.go` drops Terragrunt's copy manifest at
  every depth, not only the top level; a nested `state/.terragrunt-module-manifest` made the working
  directory differ from the reviewed plan. `pkg/component/terraform/execute_pipeline.go` does not resolve the
  instance owner to clear stale signals before a reviewed rebuild. `configuration_plan.go` forgets the CI
  plan location for deploy's own init, plan and show verification runs, not only for `deploy`.
- **`remote_state` without `generate` (integration).** `pkg/component/terraform/configuration.go` writes the
  backend the provider reports but does not generate, whatever `auto_generate_backend_file` says, so
  Terraform keeps the state at the identity that dependency reads and saved plans use.
- **Option errors (integration).** `pkg/component/provideroptions/flags.go` names the environment variable
  (for example `TG_FEATURE`) when an invalid option value came from the environment.
- **Scenarios.** `imports.star`, `semantics.star` and `modes.star` no longer configure a YAML stacks path
  without a manifest (the documented inventory rule); `modes.star` and `identity_sources.star` compare
  wrapped error text with whitespace collapsed and expect the current kept-backend message;
  `semantics.star` gives the hooks' `marker` variable a default (unit `.tf` files are copied, as Terragrunt
  copies them); `hardening.star` expects the documented rejection of a generate block that writes a backend,
  and checks that `terraform generate backend` skips a unit.
- **Review round (CodeRabbit on PR #3754).**
  - A rejected component-level backend override is dropped before backend normalization
    (`componentBackendLayers` in `internal/exec/configuration_stacks.go`), so a `backend_type` without
    matching options no longer fails the whole stack with a backend type mismatch.
  - When a provider-owned unit's module declares its own `backend` block, the host writes no
    `backend.tf.json`; it passes the `remote_state` settings to `init` as
    `-backend-config=atmos-backend.tfbackend` and removes a host backend file it wrote earlier
    (`pkg/terraform/backend/module_backend.go`, `file.go`).
  - `pkg/io/sensitive_value.go`: a long value made only of symbols, such as a password, is masked again;
    punctuation-only lines of a sensitive text are still skipped. The complete sensitive text is also
    registered in base64 (all four variants), URL-escaped and JSON-escaped forms, without its lines.
  - `plugins/terragrunt/internal/engine/backend_json.go` (new): HCL JSON backend detection reads tokens, so
    duplicate `terraform` properties and top-level body arrays are inspected.
- **Plugin version.** `plugins/terragrunt/plugin.yaml` is bumped from 0.1.1 to 0.1.2.
- **Docs.** `website/docs/migration/terragrunt.mdx`, `website/docs/cli/commands/terraform/generate/backend.mdx`
  and `plugins/terragrunt/README.md` describe the masking, generate, generate backend, `remote_state` without
  `generate`, override scoping and Debug error behavior.

## Validation

- Instrumented the plugin (temporary debug logging, removed) and confirmed the F1 trigger: the root path came
  from `errNotEvaluated` on the unread module `main.tf`; the regression fixture then planned with
  `input = "plainliteral"` and unmasked output.
- New tests were written first and confirmed failing, then passing:
  `TestResolveReportsOnlyDeclaredSensitiveInputsWithModuleFiles`, `TestResolveFindsSensitiveInputsOfIncludedAndReadConfiguration`,
  `TestEvaluatedViewSkipsFilesThatWereNeverOpened`, `TestSensitivitySkipsTerraformSourceFiles`,
  `TestSensitiveInputsResolveParentSearchesFromTheUnit`, `TestBackendPhaseAcceptsEvaluatedGenerateWithoutBackend`,
  `TestBackendPhaseRejectsEvaluatedGenerateThatCannotBeVerified`, `TestFailureDetailIsRedactedLikeTheEvents`,
  `TestCollectOverlayFilesDropsNestedCopyManifests` (plugin module), and `TestTrivialSecret`,
  `TestRegisterSensitiveValueSkipsKeysAndTrivialScalars`, `TestRegisterSensitiveTextSkipsTrivialLines`,
  `TestDecodeInputsSensitiveObjectDoesNotMaskKeysOrTrivialValues`, `TestWriteFilesSensitiveContentDoesNotMaskStructure`,
  `TestUnsafeDetailExplainsTheErrorAtDebug`, `TestExecuteGenerateBackendSkipsProviderOwnedComponents`,
  `TestGenerateComponentBackendSkipsProviderOwnedComponents`, `TestMarkBackendOverridesRecordsTheRejectedSetting`,
  `TestPreflightRejectsARecordedBackendOverride`, `TestInvalidEnvironmentValueNamesTheVariable`, and the new
  cases in `TestCheckStackOverrides`, `TestDropCIPlanLocationKeepsEveryPlanTheUserNamed`,
  `TestPreparedBackendGenerationHonorsTheProjectSetting` and `TestOrdinaryComponentKeepsAFileNamedLikeTheSignalsFile`
  (host).
- `go build ./...`; `go vet` and `gofumpt -l` on every changed package (clean).
- `go test` passed for `pkg/io`, `pkg/component/configuration`, `pkg/component/provideroptions`,
  `pkg/component/terraform`, `pkg/plugin/adapter/component`, `pkg/scheduler/adapters`, `pkg/schema`,
  `internal/exec` (configuration, import and provider tests), and the plugin module (`go test ./...` in
  `plugins/terragrunt` and in `plugins/terragrunt/bridge`).
- Patch coverage on added and changed non-test Go lines, measured from `go test -coverprofile` (host
  packages, the plugin with `-coverpkg=./internal/...`, the bridge module, and `pkg/component/terraform`
  through `-test.gocoverdir`): 209 of 209 statements (100%).
- `atmos plugin test plugins/terragrunt --integration` on macOS (OpenTofu 1.13.2, Terraform 1.5.7,
  `TMPDIR=/tmp/tgf`), before and after:

| Scenario | Classification | Before | After |
|---|---|---|---|
| native-lifecycle | F2 (and F1 masking) | failed | passed |
| imports-and-discovery | stale scenario: YAML stacks path without manifests | failed | passed |
| stack-overrides | F4 | failed | passed |
| saved-plan-lifecycle | real bug: nested copy manifest exported as a unit file | failed | passed |
| generation-lifecycle | stale scenario: a generate block that writes a backend is rejected as documented | failed | passed |
| state-encryption | real bug: `remote_state` without `generate` kept state away from its identity; stale message | failed | passed |
| disable-init | real bug: `remote_state` without `generate` kept state away from its identity | failed | passed |
| semantics-features | real bug: option error did not name `TG_FEATURE` | failed | passed |
| semantics-terraform-binary | stale scenario: YAML stacks path without manifests | failed | passed |
| semantics-lifecycle-hooks | stale scenario: copied `marker.tf` had a required variable (output also hit F1 masking) | failed | passed |
| semantics-tflint-hook | F1 | failed | passed |
| values-exact-and-sensitive | F1 | failed | passed |
| executable-invocation | stale scenario: YAML stacks path without manifests | failed | passed |
| executable-flags-and-config | stale scenario: YAML stacks path without manifests | failed | passed |
| executable-ordering | stale scenario: YAML stacks path without manifests | failed | passed |
| executable-failures | stale scenario: YAML stacks path without manifests | failed | passed |
| executable-requirements | stale scenario: YAML stacks path without manifests; wrapped error text | failed | passed |
| pro-cross-runner-saved-plan | real bug: nested copy manifest exported as a unit file | failed | passed |
| pro-relocated-checkout-saved-plan | real bug: stale signals cleared before a reviewed rebuild | failed | passed |
| pro-deploy-verifies-stored-plan | real bug: CI plan location kept for deploy's verification runs | failed | passed |

  None of the 20 was environmental. The other 24 scenarios passed before and after. Totals: 24 passed and
  20 failed before; 44 of 44 passed after. `executable-real-terragrunt` installs Terragrunt through
  `atmos toolchain install`, so it needs network access.
- `atmos plugin test plugins/terragrunt` (default suite, after the version bump and rebuild): 3 of 3 passed.
- By hand: a unit whose directory holds a nested `.terraform.lock.hcl` shows only the headline and the debug
  hint at the default level, and with `--logs-level=Debug` the error includes the overlay's reason (F5).
- Review round: new tests written first and seen failing, then passing:
  `TestRegisterSensitiveValueMasksSymbolOnlySecrets`, `TestRegisterSensitiveTextMasksEncodedForms`, new
  `TestTrivialSecret` and `TestProvablyWithoutBackend` cases, `TestJSONDeclaresBackend`, the "component type
  without options" case of `TestImportedUnitBackendOptionsBeforeNormalization`,
  `TestGenerateFilePassesTheBackendToInitWhenTheModuleDeclaresOne`,
  `TestGenerateFileWritesTheBackendWhenTheModuleDeclaresNone`, `TestGenerateFileKeepsUnownedComponentsUnchanged`,
  `TestPassBackendToInitEdgeCases` and `TestModuleDeclaresBackendIgnoresWhatIsNotABackendBlock`. By hand: a
  unit whose module declares `backend "local" {}` and whose `remote_state` has no `generate` applied with its
  state at the `remote_state` path, and a second plan reported no changes (before: "Duplicate backend
  configuration"). After the review round: plugin default suite 3 of 3 and integration suite 44 of 44 passed
  again, and patch coverage on added and changed non-test Go lines was 311 of 315 statements (98.7%).
- `golangci-lint` could not be run in this environment (the command was denied). Comment punctuation,
  `perf.Track` on new exported host functions, file length and complexity were checked by hand.
- The website was not rebuilt: `website/node_modules` is not installed in this worktree.

## Follow-ups

- Discovery diagnostics (`list instances` on a broken unit) still drop unsafe detail, so F5's Debug detail
  applies to commands that prepare a unit, not to discovery listings.
