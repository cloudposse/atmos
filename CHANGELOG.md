# Changelog

## Unreleased

### Breaking Changes

- `pkg/logger.NewAtmosLogger` now requires an explicit `io.Writer` second argument.
  Pass `nil` to use the default `os.Stderr` writer.
  Migration: `NewAtmosLogger(charmLogger)` → `NewAtmosLogger(charmLogger, nil)`.

### Links

- Blog: [Warn when vendoring from an archived GitHub repository](/blog/warn-vendor-archived-repo)
  (`website/blog/2026-03-12-warn-vendor-archived-repo.mdx`)
- Docs: [`atmos vendor pull` — ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT](/cli/commands/vendor/pull)
  (`website/docs/cli/commands/vendor/vendor-pull.mdx`)
- Docs: [Environment Variables — ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT](/cli/environment-variables)
  (`website/docs/cli/environment-variables.mdx`)
