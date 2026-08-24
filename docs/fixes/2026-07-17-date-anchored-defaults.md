# Fix: Edition Defaults and Auth Table Widths

**Date:** 2026-07-17

## Summary

Corrected three shipped defaults whose configured values did not consistently match their
effective behavior, added date-anchored editions so projects can preserve the defaults that
applied at a chosen point in time while upgrading Atmos, and made auth list tables adapt to the
available terminal width.

## Context

Atmos resolves defaults through several configuration layers. Historical changes to the Helmfile
EKS setting, terminal pager, and log level left those layers out of sync: some projects continued
to receive an older default even after the declared change shipped. Projects also had no supported
way to keep previous defaults when intentionally upgrading the CLI. Separately, auth list tables
used fixed column widths, wasting space on wide terminals and truncating values unnecessarily.

## Changes

- Aligned the effective Helmfile EKS default with its opt-in behavior and made an earlier edition
  restore the prior automatic behavior.
- Aligned the no-config-file terminal pager default with the disabled-pager behavior and made an
  earlier edition restore the prior enabled pager.
- Removed the embedded logging configuration that shadowed the `Warning` log-level default, so
  unpinned projects now receive the declared default and earlier editions retain `Info`.
- Added the top-level `edition` date anchor, `--edition`, and `ATMOS_EDITION` resolution so a pin
  reapplies only defaults that changed after its date; explicit configuration still wins.
- Added a default snapshot and journal invariants to require a matching edition entry whenever a
  previously shipped default changes, while allowing brand-new defaults without rollback gating.
- Made auth provider and identity tables size to terminal content and available width, including the
  `settings.terminal.max_width` ceiling; narrow terminals shrink lower-priority columns first while
  non-TTY output keeps its stable legacy widths.

## Validation

- `go test ./pkg/edition ./pkg/config`
- `go test ./pkg/auth/list`
- `git diff --check`
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh docs/fixes/2026-07-17-date-anchored-defaults.md`

## Follow-ups

None.
