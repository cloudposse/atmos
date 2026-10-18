# Fix: Preserve chat terminal ownership when merging interactive AI questions

**Date:** 2026-10-18

## Summary

Merge the interactive AI changes from `main` while preserving declarative skills
and the field-test fixes. Give model questions the terminal handoff already used
by chat's tool approval prompts.

## Context

Main added `AskUserQuestion` handling outside the permission prompter. This branch
attaches approvals to chat and releases its terminal inside that prompter, so the
new question path would otherwise compete with the chat renderer for input.

## Changes

- Wrap model questions with terminal release and restoration, forwarding answers
  unchanged and returning release/restore failures. Ordinary tool approvals retain
  their existing cached-decision handling.
- Keep the branch's physical-terminal checks, explicit remembered scope labels,
  per-turn stack context, and provider descendant cleanup.
- Retain main's question choices, shared abort keys, response-style system prompts,
  error hints, protocol validation, rendering improvements, and additional tests.
- Combine the documentation for both permission boundaries and question prompts.

## Validation

- AI command and package short tests, spinner tests, and configuration tests pass.
  The first run overlapped creation of the chat wrapper; the affected TUI,
  approval, and permission packages passed after all files were present.
- New chat question tests verify release before prompting, restoration on success
  and cancellation, terminal failures, answer forwarding, and ordinary-tool bypass.
- Race tests pass for chat, approval, and Claude Code packages.
- `go build -p 4 ./...` passes, and `atmos lint --changed` reports zero issues.
- Website build passes after `pnpm install --frozen-lockfile` installs main's new
  font dependency. Docusaurus reports nonblocking anchor and normalization warnings.

## Follow-ups

None.
