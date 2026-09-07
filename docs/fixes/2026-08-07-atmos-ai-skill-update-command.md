# Fix: implement `atmos ai skill update` instead of leaving it deferred

**Date:** 2026-08-07

## Summary

Finding #11 of the `atmos ai` field-test fix pass ("no `skill update`/outdated-detection command")
was originally scoped down to a help-text tweak, with the real command deferred and left
untracked per standing "don't open issues unprompted" guidance. When asked directly, the user
said to implement it instead. `atmos ai skill update [name]` now exists: it reinstalls only the
installed bundled skills that are actually outdated, is a no-op for skills already current, and
reuses the exact `install --force` distribution logic once a reinstall is confirmed necessary.

## Context

The `/fix-log` skill's Follow-ups requirement (link a GitHub issue for any deferred work) surfaced
this gap explicitly: leaving "no update command" as an untracked follow-up wasn't compliant, and
rather than open an issue for it, the user chose to just have it built. This is additive,
user-visible functionality (a new subcommand), so the PR's semver label moved from `patch` to
`minor`, which per this repo's release-doc policy also requires a blog post and a roadmap update
(both included in this change).

## Changes

- `pkg/ai/skills/marketplace/update.go` (new): `Installer.UpdateSkill(ctx, name, opts)` and
  `Installer.UpdateAllBundled(opts)`, plus an exported `SkillVersionOutdated(installed, catalog)`
  helper. Bundled skills only — git-sourced/community skills return
  `ErrSkillUpdateNotSupported`, pointing at `install <source> --force` as the manual path, since
  checking a git source's upstream version cheaply isn't possible without a fetch.
- `cmd/ai/skill/list.go`: refactored to call the new shared `SkillVersionOutdated` helper instead
  of duplicating the comparison inline, so `list --detailed`'s "update available" indicator and
  `update`'s reinstall decision can never disagree.
- `cmd/ai/skill/update.go` (new): the CLI command, mirroring `install`'s flag surface
  (`--client`/`--all-clients`/`--scope`/`--global`/`--path`/`--yes`) since an outdated skill is
  reinstalled the same way `install --force` would install it.
- `cmd/ai/skill/markdown/atmos_ai_skill_update{,_usage}.md` (new), `cmd/ai/markdown/atmos_ai_skill.md`,
  `cmd/ai/skill/install.go` (corrected the now-false "there is no separate update command" help
  text), `agent-skills/skills/atmos-ai/SKILL.md`, `website/docs/cli/commands/ai/skill.mdx`: docs
  updated across all four surfaces.
- `website/static/casts/screengrabs/atmos-ai-skill--help.cast`: regenerated (`--filter "ai skill
  --help"`, scoped to just this one cast) since the new subcommand changes that `--help` output.
- `website/blog/2026-08-07-ai-skill-update.mdx` (new) and `website/src/data/roadmap.js`: blog post
  and roadmap milestone, required by the `minor` label per the `pull-request`/`changelog`/`roadmap`
  skills.

## Validation

- `go build ./...`, `go vet ./...` clean.
- `go test ./cmd/ai/... ./pkg/ai/skills/marketplace/... -count=1` — all new and existing tests
  pass, including `TestUpdateSkill_{NotInstalled,AlreadyUpToDate,ReinstallsWhenOutdated,
  NonBundledSkillUnsupported}` and `TestUpdateAllBundled_*`.
- `atmos fix lint` (patch-scoped) — 0 issues after one round of fixes (gocritic hugeParam,
  nolintlint).
- Manual end-to-end smoke test against isolated `HOME`/project fixtures under `/tmp`: installed
  `atmos-terraform`, confirmed `update` no-ops when current, manually downgraded the recorded
  registry version and confirmed `update` reinstalls and corrects it, confirmed `update` on a
  never-installed skill errors, confirmed `update` on a simulated git-sourced installed skill
  returns the `ErrSkillUpdateNotSupported` error with the correct manual-refresh suggestion, and
  confirmed `update` with no args correctly reports "already up to date" when nothing is outdated.
- `cd website && npm run build` — succeeds with both the blog post and roadmap changes present.
- `./build/atmos --chdir=demo/casts casts validate screengrabs cli` — passes.

## Follow-ups

None.
