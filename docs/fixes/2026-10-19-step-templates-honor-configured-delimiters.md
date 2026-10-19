# Fix: Step templates honor `templates.settings.delimiters`

**Date:** 2026-10-19

## Summary

`templates.settings.delimiters` is documented as the delimiters used to process templates, but only stack manifests honored it. Workflow, custom command, and lifecycle hook steps always parsed `{{ }}`, so a project that configured `["[[", "]]"]` still had to escape or mark `!literal` every step field that held Go-template-looking text, and could write two template syntaxes depending on where a value lived. Step templates now use the configured pair. The change is selected by the edition behavior transition `templates.step_delimiters` (`2026-10-19`); projects pinned to an earlier edition keep `{{ }}` for steps.

## Context

Step fields render through `Variables.Resolve` (`pkg/runner/step/variables.go`) with a renderer that workflows (`internal/exec/workflow_utils.go`) and custom commands (`cmd/cmd_utils.go`) install on top of `ProcessTmpl`. `processTmpl` parsed with the default delimiters; only the stack-manifest paths applied `resolveTemplateDelimiters`. Template detection, the multi-pass loop, and flag-value protection also hardcoded `{{`, as did the `output:` detection in both hosts, the control-step child renderer and its default `{{ .step.name }}` prefix, the embedded-interpreter validation, the `!literal` error hint, and the hook renderer. Fixing only the parser would have left every one of those sites treating `[[ ]]` fields as plain text, or treating `{{` in a flag value as a template to protect.

This is the editions framework's `KindBehavior` case: an existing configuration value gets a new effect, without its stored value changing. Both code paths are kept and selected by `edition.BehaviorChanged`.

## Changes

- `pkg/edition/journal.go`: one `KindBehavior` entry, `templates.step_delimiters`, dated `2026-10-19`. Its `Ref` is https://github.com/cloudposse/atmos/pull/3436.
- `pkg/template/delimiters` (new): the `Delimiters{Left, Right}` value type (zero value is the default pair), `Parse` (the validation that `resolveTemplateDelimiters` carried, which now delegates to it), and `ForSteps(atmosConfig)`, which returns the configured pair only when `BehaviorChanged(atmosConfig.Edition, "templates.step_delimiters")` is true. Invalid pins and invalid delimiter lists are returned as errors. Helpers: `Contains`, `ContainsAny`, `Wrap`, `Protect`/`Unprotect`, `Apply`.
- `pkg/runner/step`: `Variables` carries the pair (`SetTemplateDelimiters`, `ApplyTemplateDelimiters`, `TemplateDelimiters`, copied by `Clone`). Detection, the multi-pass loop, protection and restoration of protected roots, the built-in fallback renderer, and `output:` detection use it. `Variables.TemplateFieldError` names the configured opening in the `!literal` hint.
- `internal/exec`: `ProcessStepTmpl` parses with a supplied pair (`tmplInput.delims` is applied before parsing). `workflow_utils.go` and the two control adapters are inline call sites that resolve the pair once and pass it on.
- `cmd/cmd_utils.go`: resolves the pair once per custom command run, installs it with the renderer, and applies it to the legacy `component_config.component`/`stack` templates, the `output:` check, and the interpreter checks. `cmd/standalone_script.go` and `pkg/git/hooks/steps.go` install it on their step variables.
- `pkg/workflow`: `ControlExecutionOptions.Delimiters` drives child field detection and parsing and builds the default output prefix from the pair. `ValidateEmbeddedInterpreters`, `ValidateEmbeddedInterpreterSteps`, and `RenderScriptInterpreter` accept `WithEmbeddedDelimiters`; the validate-time check is conservative and treats a value holding the default `{{` or the configured opening as templated, so it never rejects what the runtime would render.
- `pkg/hooks`: a `hookRenderer` carries the pair for the hook envelope and every step payload. `ExecContext` resolves it once per invocation, and both step engines fail the hook on an invalid pin or delimiter list.
- Not changed: stack-manifest templating, `IsGolangTemplate`, and `pkg/generator/scaffoldhooks` (scaffold templates have no `atmos.yaml` to read, so they keep `{{ }}`).
- Docs: the `templates.settings.delimiters` entry in `website/docs/templates/templates.mdx`, the behavior-changes section of `website/docs/cli/configuration/edition.mdx`, `docs/prd/editions.md`, a changelog post, and a roadmap milestone.

Old and new paths:

- Old (pinned before `2026-10-19`): step templates use `{{ }}` and ignore `templates.settings.delimiters`.
- New (unpinned, or pinned on or after `2026-10-19`): step templates use `templates.settings.delimiters`. With no setting, or the default pair, behavior is unchanged.
- To keep the old behavior, pin an earlier edition in `atmos.yaml`: `edition: "2026-10-18"`.

## Validation

- `go build ./...` passed.
- `go test -count=1 ./pkg/edition/... ./pkg/config/... ./pkg/runner/step/... ./pkg/workflow/... ./pkg/hooks/... ./pkg/template/... ./pkg/git/hooks/...` passed, except seven `TestCast*` session-mode tests in `pkg/runner/step` that time out waiting for a pseudo-terminal response. They fail identically on a clean `origin/main` checkout, so they are an environment limit and not related to this change.
- `go test -count=1 ./internal/exec -run 'Workflow|Tmpl|Template'` and `go test -count=1 ./cmd -run 'Custom|Command|Template'` passed.
- New table-driven tests cover resolution (unpinned, pinned before, on, and after the date, no delimiters, explicit default pair, invalid pin, invalid list), `command`/`script`/`env`/`output` rendering under both paths, protected flag roots under custom delimiters, control children and the default prefix, the hook envelope and step engines, a real workflow run, and a real custom command run under each pin.
- `./custom-gcl run --new-from-rev=origin/main ./pkg/... ./cmd/... ./internal/exec/...` reported 0 issues; `gofumpt -l` on the changed Go files was clean.
- End to end with the built binary: a custom command and a workflow with `delimiters: ["[[", "]]"]` print `got first and {{ literal }}` when unpinned and when pinned to `2026-10-19`. Pinned to `2026-10-18`, `[[ ]]` stays literal and `{{ literal }}` is parsed as a template, which fails with `function "literal" not defined`, the pre-change behavior.

## Follow-ups

None.
