# Fix: Skill reconciliation and AI approval field-test defects

**Date:** 2026-10-09

## Summary

Fix skill reconciliation, configuration editing, chat approvals, and follow-up interaction issues reproduced while field-testing declarative skills and AI permissions.

## Context

The PR field test exercised real CLI commands, isolated project/user installations, loopback Git repositories, PTYs, and authenticated Claude Code requests. Legacy update preview flags could mutate installations; named uninstall bypassed confirmation and scope selection; several selectors and source-edit operations did not match their documented behavior. Chat did not attach the permission checker used by ask and exec, and approval labels overstated the specificity of remembered permissions.

Claude-native and provider-preapproved requests intentionally follow Claude's own policy. Atmos handles requests delegated to it; this boundary is now explicit in the documentation.

## Changes

- Reject unsupported legacy dry-run/check/frozen updates before mutation. Restore named installation/update/uninstall confirmation and project-default removal scope, honor environment selectors, register update force recovery, and disambiguate same-owner source/skill names.
- Keep manual-path updates at existing destinations. Apply explicit uninstall track filters, report completed reconciliation statuses, and reject unsupported set/remove `--name` flags.
- Exclude project-generated skill state and owned destinations from local source snapshots, keeping `source: .` stable without ignoring authored hidden files. Validate layered edits against effective configuration and preserve YAML file permissions.
- Attach Claude approval and timeout handling to chat, including provider switches. Release and restore the chat terminal around real prompts, retaining cached decisions.
- Show the actual command/path/tool scope in remembered approval choices. Preserve existing cache behavior, including tool-wide MCP decisions.
- Carry initial stack context into every ask follow-up without duplicating it in persisted question history. Make Escape end follow-ups and require actual terminal input even when output TTY styling is forced.
- Terminate Claude provider descendants on timeout, caller cancellation, and finish-grace expiry using Unix process groups and Windows job objects. Normal successful completion retains existing detached-service behavior.
- Update command, configuration, changelog, and consumer agent-skill documentation for these contracts.

## Validation

- New behavioral regressions reproduced failures before implementation.
- `go test ./cmd/ai/skill -short -count=1` and shuffled execution passed.
- `go test ./pkg/ai/skills/source -count=1` passed.
- `go test ./pkg/ai/tui ./cmd/ai ./pkg/ai/interactive ./pkg/ai/tools/permission -short -count=1` passed.
- `go test ./cmd/ai/... ./pkg/ai/... -short -count=1` passed across the complete AI command/package tree.
- `atmos build` and `go build ./...` passed.
- `atmos lint --changed` passed with zero issues using the repository custom lint plugins.
- Linux amd64 and Windows amd64 provider tests cross-compiled; runtime checks ran on macOS.
- The website build, MDX reference compilation, agent-skill structure checks, and fix-document validation passed.
- Fresh CLI command replay passed 88 assertions across 70 commands, repeating legacy no-mutation guards, PTY confirmations, scope/client selection, environment handling, force recovery, and label/status checks from independent baselines.
- Fresh CLI source-engine replay passed 93 assertions across 53 commands, including real Git resolution/pins, frozen replay, version tracks, offline checks, manual destinations, and repeated source-root/layered-edit checks.
- PTY replays confirmed Escape, forced-TTY nonterminal behavior, context on every follow-up, approval allow/deny/cancel and caching, timeout pausing, and narrow/wide rendering.
- Six fresh CLI descendant cases passed: timeout stopped child work in about 2.02 seconds, and result/finish-grace cases in about 5.04 seconds. Every child was dead when Atmos exited and no marker appeared after its scheduled write time. Focused provider race tests also passed.
- Real Claude chat requests confirmed delegated blocked MCP denial and interactive approval with a real marker write and restored chat UI. A first interactive fixture contained terminal-query response text; the corrected replay sent clean input and completed successfully.
- Evidence and reproducible scripts are preserved under `.context/field-test-3352/`; the original investigation remains intact.

## Follow-ups

None.
