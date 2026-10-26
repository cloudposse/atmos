# Fix: `atmos ai` field-test findings — 16 DX and correctness gaps

**Date:** 2026-08-07

## Summary

A hands-on field test of the `atmos ai` command surface (`skill install/list/uninstall`,
`ask`/`exec`/`sessions`, MCP routing) surfaced 16 ranked findings — silent flag no-ops, a version
gate skipped on two of three install paths, broken session export/import, an unreachable exit
code, and more. All 16 are fixed on this PR.

## Context

`/field-test our atmos ai commands` was run to catch "vibe-coded slop" in the `atmos ai` command
tree — behavior that looks fine in code review but breaks or misleads a real user. Three parallel
agents executed real commands against isolated `HOME`/project fixtures (never the operator's real
`~/.atmos` or this repo's own `.claude/`) and produced a ranked report. The user then asked for a
plan to fix everything found, made four explicit calls on ambiguous items (implement `--session`
persistence fully rather than just erroring; make exit code 2 reachable for real rather than
doc-only; make `--older-than 0d` genuinely destructive rather than doc-only; add local-path source
support), and the rest were implemented as scoped.

## Changes

**`pkg/ai/skills/marketplace` + `cmd/ai/skill`:**
- `installer.go`: the `compatibility.atmos` version gate now runs for bundled and multi-skill Git
  package installs (previously only single-skill Git clones were validated at all).
- `pkg/flags`: `WithValidValues` now supports `StringSliceFlag`, and a new
  `StandardParser.ValidateFlagValues(cmd)` closes a latent gap where `ValidValues` was silently
  dead for any command using `BindFlagsToViper` without the full `Parse()` pipeline. Wired into
  `skill install`/`uninstall`'s `--client` flag.
- `install.go`: warns when `--path` is combined with `--client`/`--scope`/`--global`/`--all-clients`
  (those flags are otherwise silently ignored).
- `local_registry.go`: registry-corruption error now uses the `errUtils.Build().WithHint()` pattern.
- `InstalledSkill` gained `MinAtmosVersion`; `skill list --detailed` now shows it and flags update
  availability.
- `source.go`/`downloader.go`: added local-path/`file://` source support, reusing the existing
  `copyDir` helper.
- Doc drift fixed in `agent-skills/skills/atmos-ai/SKILL.md` (never documented `skill install`) and
  the embedded `--help` markdown (dropped a phantom `info` subcommand).

**`cmd/ai` + `pkg/ai/{executor,session,agent}`:**
- New `cmd/ai/session_helpers.go` + `pkg/ai/executor` `Options.History`: `exec`/`ask --session` now
  actually load, thread, and persist conversation history — previously a complete no-op.
- `chat.go`: session `Model` is now resolved from the constructed client (`client.GetModel()`)
  instead of an independent config lookup, fixing `sessions export`/`import` for the default
  zero-config `claude-code` path.
- `init.go`: `--mcp` server filtering now applies to CLI providers too (was unconditionally
  ignored for `claude-code`/`codex-cli`/`copilot-cli`/`gemini-cli`).
- `executor.go`: infrastructure-level tool failures (e.g. an unregistered tool) now set
  `Success=false`/`Type: "tool_error"` immediately instead of only after 25 tool-call iterations;
  application-level failures still get fed back to the model to retry, unchanged.
- `exec.go`: `--format` now validated against `text`/`json`/`markdown` at the flag layer.
- `pkg/ai/session/manager.go` + `cmd/ai/sessions.go`: `sessions clean --older-than 0d` now deletes
  everything immediately, distinguished via a sentinel from "flag not provided" (still 30-day
  default); negative durations are now a hard parse error.

## Validation

- `go build ./...` and `go vet ./...` clean.
- `go test ./cmd/ai/... ./pkg/ai/... ./pkg/flags/... ./errors/...` — all pass, both locally and
  with `CI=true` set.
- `gofumpt -l` clean on every changed Go file.
- `atmos fix lint` (patch-scoped, matches CI's real gate) — 0 issues after one round of fixes
  (godot, gocritic, nolintlint, revive, unparam).
- `cd website && npm run build` — succeeds for the doc changes.
- Manual smoke test against isolated `HOME`/project fixtures under `/tmp` confirmed `--client`
  rejection, the `--path` warning, and the registry-corruption hint against the real CLI, not just
  unit tests.
- Full CI run on PR #2903: all jobs pass, including `Acceptance Tests (linux/macos/windows)`.

## Follow-ups

None. The one deferred item from the original field-test report (`atmos ai skill update`, a real
update command instead of a `--force`-only refresh) was implemented in the same PR after the user
asked — see `2026-08-07-atmos-ai-skill-update-command.md`.
