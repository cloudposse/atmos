# Fix: Version Tracker error hints, apply blast-radius visibility, and stale manager docs

**Date:** 2026-09-08

## Summary

A `/field-test` pass on branch `osterman/json-manager-format-field` (json manager `format` field +
new `yaml` file manager) found four issues in the Version Tracker's file-manager subsystem: every
`WithHint()` attached to a file-manager error was silently dropped before reaching users; a single
broken `version.files` rule discarded every other rule's successfully planned changes with no
indication anything was withheld; `managed-versions.mdx`, `cli/commands/version/track/apply.mdx`,
and the `atmos-version` skill omitted the `json`/`yaml` managers entirely (and one doc stated a
now-false claim); and a plausible copy-paste misconfiguration (porting a json manager's gjson
wildcard path into a yaml manager rule) produced a correct-but-leaky error exposing a raw Go
`strconv.ParseInt` detail. All four are fixed.

## Context

The `/field-test` skill built a disposable fixture (a copy of `examples/version-tracker`) and ran
`atmos version track apply` live against it to verify the PR's own claims and probe adjacent
behavior no automated test exercised. Two of the four findings (the hint-swallowing bug and the
JSON-manager-doc staleness) predate this PR — they already affected the existing `json` manager —
but the PR's new `yaml` manager copies the same broken hint pattern and adds new failure modes
(multi-document rejection, anchor rejection, a different path dialect) that make both pre-existing
bugs far more likely to be hit in practice, so fixing them here closes the loop on the whole
subsystem rather than just the new code.

## Changes

- **`errors/join.go`** (new): added `errUtils.JoinPreservingHints`, a drop-in replacement for
  `errors.Join` that re-attaches every sub-error's hints onto the joined result.
  `cockroachdb/errors`' `GetAllHints` walks a single-cause `Unwrap() error` chain and treats a
  multi-cause `Unwrap() []error` error — exactly what `errors.Join` produces — as a leaf node, so
  any hint attached below a `Join` boundary was completely unreachable by the CLI's error
  formatter, in any verbosity, regardless of which manager set it.
- **`pkg/version/managers/managers.go`**: `Plan()` now calls `withheldChangesError` instead of
  `errors.Join` directly. Apply/check stay atomic (a documented, intentional design — no partial
  writes), but when other rules planned real changes, the returned error now carries a hint naming
  every withheld path and manager, e.g. `1 other planned change(s) were withheld because of the
  error(s) above: values.yaml (yaml)`.
- **`pkg/version/managers/yaml/yaml.go`**: `applySet` now routes both `atmosyaml.Get`/`Set` error
  paths through `wrapSetFailed`, which detects gjson-wildcard characters (`#*?@`) in the configured
  path and attaches a hint explaining the dialect mismatch instead of surfacing the raw
  `strconv.ParseInt` detail from the underlying `yq` evaluator unexplained. The bare-numeric-path
  decode hint (previously copy-pasted from the json manager, describing json's dialect) now
  correctly describes yaml's own bracket-index syntax (`sources[0].version`).
- **Docs**: added `json`/`yaml` manager sections to
  `website/docs/cli/configuration/version/managed-versions.mdx` (and removed its now-false "use
  `template` instead of JSON" claim), added matching `<dt>`/`<dd>` entries to
  `website/docs/cli/commands/version/track/apply.mdx`, and updated
  `agent-skills/skills/atmos-version/SKILL.md`'s file-manager list and example.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/version/... ./errors/... ./cmd/version/...` — all pass.
- New/changed code coverage: `errors/join.go` `JoinPreservingHints` 100%; `pkg/version/managers/managers.go`
  `withheldChangesError` 100%; `pkg/version/managers/yaml/yaml.go` `wrapSetFailed` 100% (measured via
  `go tool cover -func`). Package totals: `errors` and `pkg/version/managers/yaml` (98.7%) well above
  the 85% floor.
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml --allow-serial-runners
  --new-from-rev=origin/main` — clean on every file this fix touched (one `godot` finding on the new
  `errors/join.go` comment was fixed during this pass; the run's other 3 remaining findings are in
  files this branch never touches — `pkg/store/providers/azure_keyvault_store.go`,
  `cmd/terraform/utils.go`, `pkg/component/helm/client.go` — confirmed via `git diff
  origin/main...HEAD --stat` against each, so left alone as pre-existing and out of scope).
- `gofumpt -l` on every changed/new Go file — no output (clean).
- `cd website && npm run build` — succeeds; the only broken-anchor warnings reported are on
  unrelated pre-existing pages (`/changelog/mcp-for-ai-coding-assistants`,
  `/functions/yaml/terraform.output`, `/functions/yaml/terraform.state`), not any file this fix
  touched.
- Live end-to-end verification against a disposable fixture (a copy of `examples/version-tracker`,
  built from this branch's `./build/atmos`) for all four fixes: the withheld-changes hint appears
  with the 💡 icon when one yaml rule is multi-document and another has a real pending change; the
  bare-numeric-path hint appears (previously silent) and correctly names bracket-index syntax; the
  gjson-wildcard-mistake hint appears and correctly names the dialect mismatch; and — as a side
  effect of the `JoinPreservingHints` fix — the pre-existing json manager's own numeric-path hint,
  never visible before, now also appears correctly.

## Follow-ups

None.
