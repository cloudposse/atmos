# Changelog

## Unreleased

### Breaking Changes

- `pkg/logger.NewAtmosLogger` now requires an explicit `io.Writer` second argument.
  Pass `nil` to use the default `os.Stderr` writer.
  Migration: `NewAtmosLogger(charmLogger)` → `NewAtmosLogger(charmLogger, nil)`.
